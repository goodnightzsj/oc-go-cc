"""Run the deployed binary on loopback without changing production routing.

Invoke on the verified project host. Enter 'stop' to stop the child and remove
the temporary credential-bearing config; the isolated usage DB is retained.
"""

import json
import os
from pathlib import Path
import signal
import socket
import subprocess
import tempfile
import time
import urllib.error
import urllib.request


root = Path('/root/oc-go-cc')
binary = (root / '.tmp/prod/current/routatic-proxy').resolve(strict=True)
assert Path.cwd() == root
os.umask(0o077)
directory = Path(tempfile.mkdtemp(prefix='commandcode-live-', dir=root / '.tmp'))
with Path('/root/.config/oc-go-cc/config.json').open() as source:
    commandcode = json.load(source)['commandcode']
# Omitted URLs use the application's verified official defaults.
assert commandcode.get('base_url') in (None, '', 'https://api.commandcode.ai/provider/v1/chat/completions')
assert commandcode.get('api_key') or commandcode.get('api_keys'), 'CommandCode key is not configured'

with socket.socket() as probe:
    probe.bind(('127.0.0.1', 0))
    port = probe.getsockname()[1]
model_id = 'deepseek/deepseek-v4.1-flash'
model = {'provider': 'commandcode', 'model_id': model_id, 'max_tokens': 2048}
configuration = {
    'host': '127.0.0.1', 'port': port, 'hot_reload': False,
    'respect_requested_model': False, 'enable_streaming_scenario_routing': True,
    'models': {'default': model}, 'model_overrides': {model_id: model},
    'fallbacks': {}, 'catalog': {'enabled': False}, 'commandcode': commandcode,
    'logging': {'level': 'info', 'requests': True},
    'storage': {'database_path': str(directory / 'requests.db'), 'retention_days': -1},
}
config_path = directory / 'config.json'
with config_path.open('x') as target:
    json.dump(configuration, target)
home = directory / 'home'
home.mkdir()
environment = {
    'PATH': os.environ['PATH'], 'HOME': str(home),
    'TIKTOKEN_CACHE_DIR': '/root/.cache/routatic-proxy/tiktoken',
}
process = None


def stop_on_signal(signum, _frame):
    raise SystemExit(128 + signum)


for stop_signal in (signal.SIGINT, signal.SIGTERM, signal.SIGHUP):
    signal.signal(stop_signal, stop_on_signal)

try:
    with (directory / 'proxy.log').open('x') as log:
        process = subprocess.Popen([str(binary), 'serve', '--config', str(config_path)],
                                   env=environment, stdout=log, stderr=log)
        for _ in range(120):
            if process.poll() is not None:
                raise RuntimeError('Isolated proxy exited; inspect only sanitized diagnostics')
            try:
                with urllib.request.urlopen(f'http://127.0.0.1:{port}/health', timeout=1) as response:
                    health = json.load(response)
                assert health['status'] == 'ok' and health['binary'] == str(binary)
                break
            except (urllib.error.URLError, TimeoutError):
                time.sleep(0.25)
        else:
            raise TimeoutError('Isolated proxy did not become healthy')
        print(json.dumps({'ready': True, 'port': port, 'pid': process.pid,
                          'binary': str(binary), 'model': model_id, 'directory': str(directory)}), flush=True)
        for line in iter(input, 'stop'):
            print(json.dumps({'error': 'Only the stop command is accepted'}), flush=True)
finally:
    if process is not None and process.poll() is None:
        process.send_signal(signal.SIGTERM)
        try:
            process.wait(timeout=15)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait(timeout=5)
    config_path.unlink(missing_ok=True)
    print(json.dumps({'stopped': True, 'credential_copy_removed': not config_path.exists(),
                      'evidence_directory': str(directory)}), flush=True)

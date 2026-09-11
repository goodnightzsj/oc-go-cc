"""Run via SSH on the verified host; only backups and a rehearsal DB are written."""

import json
import os
from pathlib import Path
import shutil
import sqlite3
import subprocess
import tempfile
from contextlib import closing


root = Path('/root/oc-go-cc')
live = Path('/root/.local/share/routatic-proxy/data.db')
config = Path('/root/.config/oc-go-cc/config.json')
binary = root / '.tmp/prod/current/routatic-proxy'
assert live.is_file() and config.is_file() and binary.is_file()
os.umask(0o077)
directory = Path(tempfile.mkdtemp(prefix='history-restore-20260912-', dir=root / '.tmp'))
shutil.copy2(config, directory / 'config.before.json')
(directory / 'config.before.json').chmod(0o600)


def connect(path, mode='ro'):
    return sqlite3.connect(path.as_uri() + '?mode=' + mode, uri=True)


def totals(db):
    return dict(zip(('requests', 'input', 'output', 'cache_read', 'cache_creation', 'cost_units'),
                    db.execute('SELECT COUNT(*),COALESCE(SUM(input_tokens),0),'
                               'COALESCE(SUM(output_tokens),0),COALESCE(SUM(cache_read_tokens),0),'
                               'COALESCE(SUM(cache_creation_tokens),0),'
                               'COALESCE(SUM(CAST(ROUND(cost_usd*1e8) AS INTEGER)),0) '
                               'FROM requests').fetchone()))


def sync(apply=False):
    command = [str(binary), 'costs', 'sync-requests', '--config', str(directory / 'rehearsal-config.json')]
    if apply:
        command.append('--apply')
    result = subprocess.run(command, text=True, capture_output=True, timeout=60)
    if result.returncode:
        raise RuntimeError('Rehearsal sync failed: ' + result.stdout + result.stderr)
    return json.loads(result.stdout)


with closing(connect(live)) as source:
    assert source.execute('PRAGMA quick_check').fetchone()[0] == 'ok'
    before = totals(source)
    assert before['requests'] == 0, 'Live requests changed; re-plan without removing any rows'
    expected = dict(zip(('requests', 'input', 'output', 'cache_read', 'cache_creation', 'cost_units'),
                        source.execute('SELECT COUNT(*),SUM(input_tokens),SUM(output_tokens),'
                                       'SUM(cache_read_tokens),SUM(cache_write_5m_tokens+cache_write_1h_tokens),'
                                       'SUM(cost_units) FROM provider_usage').fetchone()))
    for name in ('data.before.db', 'rehearsal.db'):
        with closing(sqlite3.connect(directory / name)) as target:
            source.backup(target)
        (directory / name).chmod(0o600)

with (directory / 'rehearsal-config.json').open('x') as output:
    json.dump({'storage': {'database_path': str(directory / 'rehearsal.db'), 'retention_days': -1}}, output)

dry = sync()
assert dry['ambiguous'] == 0 and dry['conflicting'] == 0, dry
assert dry['would_insert'] == expected['requests'] and dry['would_remove'] == 0, dry
applied = sync(True)
repeat = sync(True)
assert repeat['inserted'] == 0 and repeat['updated'] == 0 and repeat['removed'] == 0, repeat

with closing(connect(directory / 'rehearsal.db')) as db:
    assert db.execute('PRAGMA quick_check').fetchone()[0] == 'ok'
    after = totals(db)
    assert after == expected, (after, expected)
    assert db.execute("SELECT COUNT(*) FROM requests WHERE provider <> 'opencode-go' OR cost_source <> 'provider' OR details_known <> 0 OR usage_trusted <> 1").fetchone()[0] == 0
    assert db.execute('SELECT COUNT(*) FROM provider_usage').fetchone()[0] == expected['requests']

result = dict(backup_dir=str(directory), live_changed=False, before=before, expected=expected,
              after=after, dry_run=dry, applied=applied, repeated=repeat,
              limitation='Only the existing 1390-row account snapshot; not all current official history.')
with (directory / 'rehearsal-report.json').open('x') as output:
    json.dump(result, output, indent=2)
print(json.dumps(result, indent=2))

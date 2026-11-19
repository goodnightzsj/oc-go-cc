package gui

import "testing"

func TestRuntimeSettingsSaveInvalidatesOlderRead(t *testing.T) {
	runPlatformBehavior(t, navigationBehaviorDOMScript+`
async function checks() {
  await bootView();
  const original = fetch;
  let releaseRead, releaseSave, signal, reads = 0;
  fetch = (url, options) => {
    if (url !== '/api/config') return original(url, options);
    if (options?.method === 'POST') return new Promise(resolve => { releaseSave = resolve; });
    reads++;
    signal = options?.signal;
    return new Promise(resolve => { releaseRead = resolve; });
  };
  const older = refreshConfig();
  const toggle = document.getElementById('toggle-autostart');
  toggle.checked = true;
  const saving = toggleAutostart(toggle);
  releaseSave({ok:true});
  await saving;
  releaseRead({ok:true, json:async()=>({autostart:false, notify:false})});
  await older;
  assert.equal(toggle.checked, true, 'a GET started before the save cannot revert a confirmed change');
  assert.equal(signal.aborted, true, 'saving must cancel the old read');

  toggle.checked = false;
  const nextSave = toggleAutostart(toggle);
  await refreshConfig();
  assert.equal(reads, 1, 'polling during a write could capture pre-save settings');
  releaseSave({ok:true});
  await nextSave;
}
`+platformBehaviorRunScript)
}

func TestRuntimeSettingsReadLifecycle(t *testing.T) {
	runPlatformBehavior(t, navigationBehaviorDOMScript+`context.windowListeners=windowListeners;
async function checks() {
  await bootView();
  activeTab = 'settings';
  const pending = [], timers = new Map(); let id = 0;
  setTimeout = (callback, ms) => { assert.equal(ms, 15000); timers.set(++id, callback); return id; };
  clearTimeout = id => timers.delete(id);
  fetch = (_url, options) => new Promise((resolve, reject) => {
    pending.push({resolve, options});
    options?.signal.addEventListener('abort', () => reject(new Error('aborted')));
  });
  const first = refreshConfig();
  for (let i = 0; i < 6; i++) await refreshConfig();
  assert.equal(pending.length, 1, 'slow settings reads must not accumulate every poll');
  for (const expire of timers.values()) expire();
  await first;
  assert.equal(pending[0].options.signal.aborted, true);
  assert.equal(timers.size, 0);

  const retry = refreshConfig();
  assert.equal(pending.length, 2);
  cancelTabRequests('settings');
  await retry;
  assert.equal(pending[1].options.signal.aborted, true);
  const resumed = refreshConfig();
  for (const handler of windowListeners.pagehide) handler(new Event('pagehide', {persisted:true}));
  await resumed;
  assert.equal(pending[2].options.signal.aborted, true);
}
`+platformBehaviorRunScript)
}

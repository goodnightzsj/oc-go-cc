package gui

import "testing"

func TestNavigationRestartsDiagnosticsOutsideSettings(t *testing.T) {
	runPlatformBehavior(t, navigationBehaviorDOMScript+`context.windowListeners=windowListeners;
async function checks() {
  await bootView();
  assert.equal(activeTab, 'overview');
  const original = fetch;
  let requests = 0, signal;
  fetch = (url, options) => url === '/api/cline-pass/channels'
    ? new Promise((resolve, reject) => {
      requests++;
      signal = options.signal;
      signal.addEventListener('abort', () => reject(new Error('aborted')));
    }) : original(url, options);
  const old = loadChannelCatalog();
  for (const handler of windowListeners.pagehide) handler(new Event('pagehide', {persisted:true}));
  await old;
  assert.equal(signal.aborted, true);
  fetch = async (url, options) => {
    if (url !== '/api/cline-pass/channels') return original(url, options);
    requests++;
    return {ok:true, json:async()=>({channels:{}, capture_enabled:false})};
  };
  for (const handler of windowListeners.pageshow) handler(new Event('pageshow', {persisted:true}));
  await settleView();
  document.getElementById('nav-settings').click();
  await settleView();
  assert.equal(requests, 2, 'document restore must restart the canceled global diagnostics read');
  assert.notEqual(document.getElementById('cline-pass-channel-diagnostics').textContent, t('clinepass.diagnosticsLoading'));
}
`+platformBehaviorRunScript)
}

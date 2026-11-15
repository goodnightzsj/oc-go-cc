package gui

import "testing"

func TestAnalyticsRequestCancellation(t *testing.T) {
	runPlatformBehavior(t, platformBehaviorDOMScript+`
async function checks() {
  const get = id => document.getElementById(id);
  get('analytics-start').value = '2026-11-01';
  get('analytics-end').value = '2026-11-01';
  const pending = [];
  fetch = (url, options) => new Promise((resolve, reject) => {
    pending.push({url, signal:options?.signal, resolve:data=>resolve({ok:true,json:async()=>data}), reject});
  });
  get('analytics-provider').value = 'workbuddy';
  const old = AnalyticsModule.load(true);
  assert.equal(pending.length,3);
  get('analytics-provider').value = 'cline-pass';
  const middle = AnalyticsModule.load(true);
  get('analytics-provider').value = 'openrouter';
  const latest = AnalyticsModule.load(true);
  assert.equal(pending.length,9);
  assert.equal(pending.filter(p=>!p.signal?.aborted).length,3,'obsolete queries remain in flight');
  await AnalyticsModule.load(true);
  assert.equal(pending.length,9,'same query must reuse in-flight requests');
  assert.ok(AnalyticsModule.loading);
  // Delayed completion is possible even after abort; sequence still owns state.
  for (const p of pending.slice(0,6)) p.reject(Object.assign(new Error('aborted'),{name:'AbortError'}));
  await Promise.all([old,middle]);
  assert.ok(AnalyticsModule.loading,'stale finally cleared latest loading');
  assert.equal(get('analytics-error').hidden,true,'superseded abort surfaced as error');
  pending[6].resolve({provider:'openrouter',summary:{total_requests:0},models:[]});
  pending[7].resolve({trend:[]});
  pending[8].resolve(null);
  await latest;
  assert.equal(AnalyticsModule.currentView.provider,'openrouter');
  assert.equal(AnalyticsModule.loading,false);
  assert.equal(AnalyticsModule.request,null);
  const failed = AnalyticsModule.load(true);
  pending[9].reject(new Error('synthetic query failure'));
  await failed;
  assert.ok(pending[10].signal.aborted && pending[11].signal.aborted,'failed load left siblings running');
  assert.equal(get('analytics-error').hidden,false);
  assert.ok(get('analytics-error').textContent.includes('synthetic query failure'));
  // A retry with unchanged filters must start after failure, not reuse dead work.
  const retry = AnalyticsModule.load(true);
  assert.equal(pending.length,15);
  for (const p of pending.slice(12)) p.reject(new Error('cleanup'));
  await retry;
}
`+platformBehaviorRunScript)
}

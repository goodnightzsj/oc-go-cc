package gui

import "testing"

func TestAnalyticsManualPriceRefreshDuringStatistics(t *testing.T) {
	runPlatformBehavior(t, platformBehaviorDOMScript+`
async function checks() {
  activeTab='analytics';
  const get=id=>document.getElementById(id);
  get('analytics-start').value=get('analytics-end').value='2026-11-01';
  let priceCalls=0;
  const reply=url=>({ok:true,json:async()=>url==='/api/prices' ? {tables:{workbuddy:{rules:1}}}
    : url.includes('/summary?') ? {summary:{total_requests:1},models:[]} : {trend:[]}});
  fetch=async url=>{if(url==='/api/prices') priceCalls++; return reply(url)};
  await AnalyticsModule.load(true);
  const pending=[];
  fetch=url=>{
    if(url==='/api/prices') {priceCalls++; return Promise.resolve(reply(url))}
    return new Promise(resolve=>pending.push(()=>resolve(reply(url))));
  };
  const running=AnalyticsModule.load(true);
  assert.equal(pending.length,2);
  assert.equal(priceCalls,1);
  await get('btn-refresh-analytics').emit('click');
  assert.equal(pending.length,2,'manual price refresh duplicated statistics');
  for(const resolve of pending) resolve();
  await running;
  assert.equal(priceCalls,2,'manual refresh during statistics did not refresh prices');
}
`+platformBehaviorRunScript)
}

func TestAnalyticsPriceCacheLifecycle(t *testing.T) {
	runPlatformBehavior(t, platformBehaviorDOMScript+`
async function checks() {
  activeTab='analytics';
  const get=id=>document.getElementById(id);
  get('analytics-start').value=get('analytics-end').value='2026-11-01';
  const calls=[];
  let now=Date.now(), priceFailure=false;
  Date.now=()=>now;
  fetch=async url=>{
    calls.push(url);
    if(url==='/api/prices' && priceFailure) throw new Error('price network failure');
    const data=url==='/api/prices' ? {tables:{workbuddy:{rules:1}}}
      : url.includes('/summary?') ? {summary:{total_requests:3},models:[]}
      : {trend:[]};
    return {ok:true,json:async()=>data};
  };
  const count=()=>calls.filter(url=>url==='/api/prices').length;
  for (let i=0;i<3;i++) await AnalyticsModule.load(true);
  assert.equal(count(),1,'three completed statistics refreshes should share prices');
  assert.equal(calls.filter(url=>url.includes('/summary?')).length,3,'price caching must not cache statistics');
  get('analytics-provider').value='workbuddy';
  await AnalyticsModule.load(true);
  assert.equal(count(),1,'instance-wide prices should survive filter changes');
  assert.equal(get('price-tables').hidden,false,'cleared view failed to redraw cached prices');
  now+=59999;
  await AnalyticsModule.load(true);
  assert.equal(count(),1);
  now++;
  await AnalyticsModule.load(true);
  assert.equal(count(),2,'price cache did not expire at 60 seconds');
  await get('btn-refresh-analytics').emit('click');
  assert.equal(count(),3,'manual refresh must bypass completed price cache');
  priceFailure=true;
  now+=60000;
  await AnalyticsModule.load(true);
  assert.equal(count(),4);
  assert.equal(get('price-tables').hidden,true,'failed prices remained presented as current');
  priceFailure=false;
  await AnalyticsModule.load(true);
  assert.equal(count(),5,'price failure was cached');
  assert.equal(get('price-tables').hidden,false);
}
`+platformBehaviorRunScript)
}

func TestSharedPlatformChangeDoesNotLoadHiddenStatistics(t *testing.T) {
	runPlatformBehavior(t, platformBehaviorDOMScript+`
async function checks() {
  document.hidden = false;
  activeTab = 'history';
  const get = id => document.getElementById(id);
  get('analytics-start').value = get('analytics-end').value = '2026-11-01';
  for (const id of VIEW_CONTROLS.platform) {
    get(id).options = [{value:''},{value:'workbuddy'}];
    get(id).value = '';
  }
  const pending = [];
  fetch = (url, options) => new Promise((resolve, reject) => {
    pending.push({url,signal:options?.signal});
    options?.signal?.addEventListener('abort',()=>reject(new Error('aborted')));
  });
  setViewPlatform('workbuddy');
  const isStats = p => /^\/api\/(analytics|perf|prices|quota)/.test(p.url);
  assert.equal(pending.filter(isStats).length,0,'shared filter started hidden-page requests');
  for (const id of VIEW_CONTROLS.platform) assert.equal(get(id).value,'workbuddy','filter synchronization was lost');
  assert.equal(QuotaModule.provider,'workbuddy');
  for (const [tab, run, count] of [
    ['overview',()=>refreshOverviewUsage(),3],
    ['performance',()=>PerfModule.refresh(),1],
    ['analytics',()=>AnalyticsModule.load(true),3],
  ]) {
    activeTab = tab;
    const before = pending.length;
    const request = run();
    assert.equal(pending.length-before,count,'visible page did not load '+tab);
    document.hidden = true;
    handleVisibilityChange();
    assert.ok(pending.slice(before).every(p=>p.signal.aborted),'hidden page retained active statistics');
    await request;
    document.hidden = false;
  }
}
`+platformBehaviorRunScript)
}

func TestAnalyticsPricesDoNotBlockStats(t *testing.T) {
	runPlatformBehavior(t, platformBehaviorDOMScript+`
async function checks() {
  activeTab = 'analytics';
  const get = id => document.getElementById(id);
  get('analytics-start').value = '2026-11-01';
  get('analytics-end').value = '2026-11-01';
  const pending = [];
  fetch = (url, options) => new Promise((resolve, reject) => {
    pending.push({url, signal:options.signal, resolve:data=>resolve({ok:true,json:async()=>data}), reject});
  });
  const load = AnalyticsModule.load(true);
  pending.find(p=>p.url.includes('/summary?')).resolve({summary:{total_requests:7},models:[]});
  pending.find(p=>p.url.includes('/trend?')).resolve({trend:[]});
  for (let i=0;i<20;i++) await Promise.resolve();
  assert.equal(get('kpi-requests').textContent,'7','prices blocked statistics');
  assert.equal(AnalyticsModule.loading,false);
  const price = pending.find(p=>p.url==='/api/prices');
  assert.equal(price.signal.aborted,false,'statistics completion aborted independent prices');
  price.resolve({tables:{workbuddy:{rules:1}}});
  await load;
  for (let i=0;i<20;i++) await Promise.resolve();
  assert.equal(get('price-tables').hidden,false);
}
`+platformBehaviorRunScript)
}

func TestAnalyticsRequestCancellation(t *testing.T) {
	runPlatformBehavior(t, platformBehaviorDOMScript+`
async function checks() {
  activeTab = 'analytics';
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

func TestAnalyticsTimeoutAndTabLifecycle(t *testing.T) {
	runPlatformBehavior(t, platformBehaviorDOMScript+`
async function checks() {
  activeTab = 'analytics';
  const get = id => document.getElementById(id);
  get('analytics-start').value = '2026-11-01';
  get('analytics-end').value = '2026-11-01';
  const timers = new Map();
  let nextTimer = 0;
  setTimeout = (fn, ms) => { assert.equal(ms,15000); timers.set(++nextTimer,fn); return nextTimer; };
  clearTimeout = id => timers.delete(id);
  const pending = [];
  fetch = (url, options) => new Promise((resolve, reject) => {
    const signal = options?.signal;
    signal?.addEventListener('abort',()=>reject(Object.assign(new Error('aborted'),{name:'AbortError'})));
    pending.push({url,signal,resolve:data=>resolve({ok:true,json:async()=>data})});
  });
  const timed = AnalyticsModule.load(true);
  for (const fn of [...timers.values()]) fn();
  await timed;
  assert.equal(AnalyticsModule.loading,false);
  assert.equal(AnalyticsModule.priceRequest,null);
  assert.equal(timers.size,0,'timeout timers leaked');
  assert.equal(get('analytics-error').hidden,false,'timeout must be visible');
  const retry = AnalyticsModule.load(true);
  assert.equal(pending.length,6,'timed out same query must be retryable');
  activeTab = 'analytics';
  // Drive the actual tab switch, not just its cancellation helper.
  const tab = get('synthetic-history-tab'); tab.dataset.tab = 'history';
  document.querySelectorAll = selector => selector==='.tab' ? [tab] : [];
  refreshCurrentTab = async () => {};
  activateTab('history');
  await retry;
  assert.ok(pending.every(p=>p.signal.aborted),'leaving tab left requests active');
  assert.equal(get('analytics-error').hidden,true,'leaving tab showed a failure');
  assert.equal(AnalyticsModule.loading,false);
  activeTab = 'performance';
  const perf = PerfModule.refresh();
  document.hidden = true;
  handleVisibilityChange();
  await perf;
  assert.equal(PerfModule.request,null);
  assert.equal(get('perf-error').hidden,true);
  const beforeHidden = pending.length;
  await Promise.all([PerfModule.refresh(), AnalyticsModule.load(true), refreshOverviewUsage()]);
  assert.equal(pending.length,beforeHidden,'late filter events restarted background statistics');
  document.hidden = false;
  handleVisibilityChange();
  assert.equal(pending.at(-1).signal.aborted,false,'returning to visible tab did not refresh');
  cancelTabRequests('performance');
  for (let i=0;i<20;i++) await Promise.resolve();
  assert.equal(timers.size,0);
}
`+platformBehaviorRunScript)
}

func TestPerformancePollingOnlyVisibleTab(t *testing.T) {
	runPlatformBehavior(t, platformBehaviorDOMScript+`
async function checks() {
  const intervals = [];
  setInterval = (fn,ms) => { intervals.push({fn,ms}); };
  refreshAll = async () => {};
  syncChannelFilter = () => {};
  startActiveSitePolling = () => {};
  let calls = 0;
  PerfModule.refresh = async () => { calls++; };
  startPolling();
  assert.equal(calls,0,'performance loaded on unrelated tab');
  const poll = intervals.find(p=>p.ms===6000).fn;
  poll();
  assert.equal(calls,0);
  activeTab = 'performance'; poll();
  assert.equal(calls,1);
  document.hidden = true; poll();
  assert.equal(calls,1,'background performance poll still ran');
}
`+platformBehaviorRunScript)
}

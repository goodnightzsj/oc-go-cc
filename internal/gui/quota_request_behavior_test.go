package gui

import "testing"

func TestQuotaCountdownTickerPausesWhileHidden(t *testing.T) {
	runPlatformBehavior(t, platformBehaviorDOMScript+`
async function checks() {
  currentLang='en';
  activeTab='quota';
  document.hidden=false;
  let now=100000;
  Date.now=()=>now;
  const metric=document.getElementById('countdown-metric');
  const note=document.getElementById('countdown-note');
  metric.classList.add('metric-value');
  let writes=0;
  for (const el of [metric,note]) {
    el.dataset.deadline=String(now+90000);
    Object.defineProperty(el,'textContent',{get(){return this.text},set(value){writes++;this.text=value}});
  }
  document.querySelectorAll=selector=>selector==='#tab-quota [data-deadline]' ? [metric,note] : [];
  let tick;
  setInterval=(callback,ms)=>{assert.equal(ms,1000);tick=callback};
  QuotaModule.init();
  for(let i=0;i<3;i++) tick();
  assert.equal(writes,6);
  assert.equal(metric.textContent,'1m 30s');
  document.hidden=true;
  writes=0;
  for(let i=0;i<20;i++) tick();
  assert.equal(writes,0,'hidden browser continued countdown DOM writes');
  document.hidden=false;
  activeTab='history';
  for(let i=0;i<3;i++) tick();
  assert.equal(writes,0,'non-quota tab continued countdown DOM writes');
  activeTab='quota';
  now+=61000;
  refreshCurrentTab=async()=>{};
  handleVisibilityChange();
  assert.equal(writes,2,'visibility resume did not immediately refresh the cached countdown');
  assert.equal(metric.textContent,'0m 29s','hidden time must not reset the deadline');
  assert.equal(note.textContent,t('quota.resetsIn').replace('{d}','0m 29s'));
  now+=30000;
  tick();
  assert.equal(metric.textContent,'0m 00s');
}
`+platformBehaviorRunScript)
}

func TestQuotaCancelledNewScopeReloadsWhenVisible(t *testing.T) {
	runPlatformBehavior(t, platformBehaviorDOMScript+`
async function checks() {
  activeTab='quota';
  const get=id=>document.getElementById(id);
  get('quota-local-days').value='30';
  let now=1000000;
  Date.now=()=>now;
  const pending=[];
  fetch=(url,options)=>url==='/api/sites'
    ? Promise.resolve({ok:false,status:503,text:async()=> 'synthetic site lookup unavailable'})
    : new Promise((resolve,reject)=>{
    pending.push({url,options,resolve:data=>resolve({ok:true,json:async()=>data})});
    options.signal.addEventListener('abort',()=>reject(new Error('aborted')));
  });
  QuotaModule.provider='workbuddy';
  const first=QuotaModule.load(true);
  pending[0].resolve({provider:'workbuddy',status:'unavailable',accounts:[]});
  pending[1].resolve({provider:'workbuddy',summary:{total_requests:1},models:[]});
  await first;
  now++;
  QuotaModule.provider='cline-pass';
  const switched=QuotaModule.load(true);
  assert.equal(pending.length,4);
  document.hidden=true;
  handleVisibilityChange();
  await switched;
  assert.equal(QuotaModule.loading,false);
  assert.equal(QuotaModule.localLoading,false);
  assert.equal(get('quota-local-requests').textContent,'—','cancelled work still looks in flight');
  document.hidden=false;
  handleVisibilityChange();
  for(let i=0;i<10;i++) await Promise.resolve();
  assert.equal(pending.length,6,'uncached new scope inherited the old successful scope throttle');
  pending[4].resolve({provider:'cline-pass',status:'unavailable',accounts:[]});
  pending[5].resolve({provider:'cline-pass',summary:{total_requests:2},models:[]});
  for(let i=0;i<10;i++) await Promise.resolve();
  await QuotaModule.load();
  assert.equal(pending.length,6,'completed same-scope data should retain its throttle');
  assert.equal(QuotaModule.localView.summary.total_requests,2);
}
`+platformBehaviorRunScript)
}

func TestQuotaRequestTimeoutAndCancellation(t *testing.T) {
	runPlatformBehavior(t, platformBehaviorDOMScript+`
async function checks() {
  activeTab = 'quota';
  const get = id => document.getElementById(id);
  get('quota-local-days').value = '30';
  QuotaModule.provider = 'workbuddy';
  const timers = new Map(); let timerID = 0;
  const durations=[];
  setTimeout = (fn, ms) => {durations.push(ms); timers.set(++timerID,fn); return timerID};
  clearTimeout = id => timers.delete(id);
  const pending = [];
  fetch = (url, options) => new Promise((resolve,reject) => {
    pending.push({url,options,resolve:data=>resolve({ok:true,json:async()=>data}),reject});
    options?.signal?.addEventListener('abort',()=>reject(new Error('request aborted')));
  });
  const first = QuotaModule.load(true);
  assert.equal(pending.length,2);
  assert.deepEqual(durations,[20000,15000],'account budget must cover the upstream and optional supplement; local query keeps its deadline');
  assert.ok(pending.every(p=>p.options?.signal),'both requests need cancellation');
  await QuotaModule.load(true);
  assert.equal(pending.length,2,'same-scope in-flight reload must not duplicate requests');
  for (const fn of [...timers.values()]) fn();
  await first;
  assert.equal(timers.size,0,'timeout timers leaked');
  assert.equal(get('btn-refresh-quota').disabled,false,'timeout permanently disabled refresh');
  assert.ok(QuotaModule.accountError);
  assert.ok(QuotaModule.localError);

  const second = QuotaModule.load(true);
  assert.equal(pending.length,4,'timeout must permit a same-scope retry');
  const tab = get('synthetic-history-tab'); tab.dataset.tab = 'history';
  document.querySelectorAll = selector => selector==='.tab' ? [tab] : [];
  refreshCurrentTab = async () => {};
  activateTab('history');
  assert.ok(pending.slice(2).every(p=>p.options.signal.aborted),'leaving quota retained requests');
  await second;
  assert.equal(QuotaModule.accountError,'','navigation abort became account error');
  assert.equal(QuotaModule.localError,'','navigation abort became local error');
  assert.equal(get('btn-refresh-quota').disabled,false);

  activeTab = 'quota';
  const third = QuotaModule.load(true);
  document.hidden = true;
  handleVisibilityChange();
  assert.ok(pending.slice(4).every(p=>p.options.signal.aborted));
  await third;
  await QuotaModule.load(true);
  assert.equal(pending.length,6,'hidden quota must not restart requests');
  assert.equal(timers.size,0);
}
`+platformBehaviorRunScript)
}

func TestQuotaChangedScopeRejectsLateResults(t *testing.T) {
	runPlatformBehavior(t, platformBehaviorDOMScript+`
async function checks() {
  activeTab = 'quota';
  const get = id => document.getElementById(id);
  get('quota-local-days').value = '30';
  QuotaModule.provider = 'workbuddy';
  const pending=[];
  // Deliberately settle obsolete work after abort to check result ownership.
  fetch = (url,options) => new Promise(resolve=>pending.push({url,options,resolve:data=>resolve({ok:true,json:async()=>data})}));
  const old = QuotaModule.load(true);
  QuotaModule.provider = 'cline-pass';
  const current = QuotaModule.load(true);
  assert.ok(pending.slice(0,2).every(p=>p.options?.signal.aborted));
  pending[0].resolve({provider:'workbuddy',status:'unavailable',accounts:[]});
  pending[1].resolve({provider:'workbuddy',summary:{total_requests:99},models:[]});
  await old;
  assert.equal(QuotaModule.loading,true,'old finally released the new request');
  assert.equal(QuotaModule.localLoading,true);
  assert.equal(QuotaModule.view,null);
  assert.equal(QuotaModule.localView,null);
  pending[2].resolve({provider:'cline-pass',status:'unavailable',accounts:[]});
  pending[3].resolve({provider:'cline-pass',summary:{total_requests:3},models:[]});
  await current;
  assert.equal(QuotaModule.localView.summary.total_requests,3);
  assert.equal(get('btn-refresh-quota').disabled,false);

  QuotaModule.provider='aws-bedrock';
  const billing=QuotaModule.loadAccounts(true,true);
  assert.equal(pending.at(-1).options.method,'POST');
  const account=pending.at(-1);
  get('quota-local-days').value='7';
  const local=QuotaModule.loadLocalUsage(true);
  assert.equal(account.options.signal.aborted,false,'local filter cancelled paid billing request');
  const count=pending.length;
  QuotaModule.cancel();
  assert.ok(account.options.signal.aborted);
  account.resolve({provider:'aws-bedrock',status:'unavailable',accounts:[]});
  pending.at(-1).resolve({provider:'aws-bedrock',summary:{total_requests:1},models:[]});
  await Promise.all([billing,local]);
  assert.equal(pending.length,count,'cancellation retried a paid request');
}
`+platformBehaviorRunScript)
}

package gui

import "testing"

func TestNavigationQuotaReentryIsNotManualRefresh(t *testing.T) {
	runPlatformBehavior(t, navigationBehaviorDOMScript+`
async function checks() {
  await bootView();
  const get=id=>document.getElementById(id);
  let now=1000000;
  Date.now=()=>now;
  const original=fetch;
  const accountCalls=[];
  fetch=async(raw,options)=>{
    const url=new URL(raw,'http://synthetic.invalid');
    if(url.pathname==='/api/quota') {
      accountCalls.push(url);
      if(accountCalls.length>1 && url.searchParams.get('refresh')==='1') {
        return {ok:false,status:429,text:async()=> 'synthetic quota refresh rate limit'};
      }
    }
    return original(raw,options);
  };
  get('nav-quota').click();
  await settleView();
  assert.equal(QuotaModule.view?.status,'available');
  assert.equal(accountCalls.length,1);
  get('nav-history').click();
  await settleView();
  location.hash='#quota';
  await settleView();
  assert.equal(activeTab,'quota');
  assert.equal(QuotaModule.accountError,'','returning through history forced an upstream refresh and lost the valid subscription');
  assert.equal(QuotaModule.view?.status,'available');
  assert.equal(accountCalls.length,1,'navigation must reuse the existing same-scope reload policy');

  await get('btn-refresh-quota').emit('click');
  await settleView();
  assert.equal(accountCalls.length,2,'manual refresh must still reach the source');
  assert.equal(accountCalls.at(-1).searchParams.get('refresh'),'1');
  assert.match(QuotaModule.accountError,/HTTP 429/,'a real manual-refresh failure must remain visible');
}
`+platformBehaviorRunScript)
}

func TestNavigationHistoryCancelsAndReloads(t *testing.T) {
	runPlatformBehavior(t, navigationBehaviorDOMScript+`
async function checks() {
  await bootView();
  const original=fetch;
  const pending=[];
  fetch=(raw,options)=>{
    const url=new URL(raw,'http://synthetic.invalid');
    if(['/api/history','/api/history/summary','/api/history/channels'].includes(url.pathname)) {
      return new Promise(resolve=>pending.push({url,options,resolve}));
    }
    return original(raw,options);
  };
  document.getElementById('nav-history').click();
  await settleView();
  assert.ok(pending.length>=2,'history fixture must hold real page reads');
  const old=[...pending];
  document.getElementById('nav-overview').click();
  await settleView();
  assert.ok(old.every(p=>p.options?.signal?.aborted),'leaving history retained its reads');
  location.hash='#history';
  await settleView();
  const current=pending.slice(old.length);
  assert.ok(current.some(p=>p.url.pathname==='/api/history'),'return reused an abandoned in-flight request');
  for(const p of old) p.resolve({ok:false,status:503,text:async()=> 'obsolete source failure'});
  await settleView();
  assert.equal(document.getElementById('history-error').hidden,true,'obsolete rejection overwrote returned page');
  for(const p of current) {
    const data=p.url.pathname==='/api/history' ? {items:[],total:0}
      : p.url.pathname==='/api/history/channels' ? {channels:[]}
      : {total_requests:2};
    p.resolve({ok:true,json:async()=>data});
  }
  await settleView();
  assert.equal(lastHistorySummary.total_requests,2);
  assert.equal(historyRequest,null);
}
`+platformBehaviorRunScript)
}

func TestNavigationRestoresCachedDocument(t *testing.T) {
	runPlatformBehavior(t, navigationBehaviorDOMScript+`context.windowListeners=windowListeners;
async function checks() {
  await bootView();
  document.getElementById('nav-quota').click();
  await settleView();
  const original=fetch;
  const pending=[];
  fetch=(raw,options)=>new Promise(resolve=>pending.push({raw,options,resolve}));
  const old=QuotaModule.load(true);
  assert.equal(pending.length,2);
  const before=[...pending];
  assert.ok(windowListeners.pagehide?.length,'cached-document departure has no cancellation owner');
  for(const handler of windowListeners.pagehide) handler(new Event('pagehide',{persisted:true}));
  assert.ok(before.every(p=>p.options?.signal?.aborted));
  assert.equal(QuotaModule.loading,false);
  assert.ok(windowListeners.pageshow?.length,'cached-document return has no resume owner');
  fetch=original;
  for(const handler of windowListeners.pageshow) handler(new Event('pageshow',{persisted:true}));
  await settleView();
  assert.equal(activeTab,'quota');
  assert.equal(QuotaModule.accountError,'');
  assert.equal(QuotaModule.view?.status,'available');
  for(const p of before) p.resolve({ok:false,status:503,text:async()=> 'abandoned cached-document work'});
  await old;
  assert.equal(QuotaModule.accountError,'','resumed document accepted old failures');
}
`+platformBehaviorRunScript)
}

func TestNavigationHistoryDeadlineAndDebounce(t *testing.T) {
	runPlatformBehavior(t, navigationBehaviorDOMScript+`
async function checks() {
  await bootView();
  document.getElementById('nav-history').click();
  await settleView();
  const timers=new Map(); let timerID=0;
  setTimeout=(callback,ms)=>{timers.set(++timerID,{callback,ms});return timerID};
  clearTimeout=id=>timers.delete(id);
  const pending=[];
  fetch=(raw,options)=>new Promise((resolve,reject)=>{
    const p={raw,options,resolve}; pending.push(p);
    options?.signal?.addEventListener('abort',()=>reject(new Error('aborted')));
  });
  const request=refreshHistory();
  assert.ok(pending.length>=2);
  assert.equal([...timers.values()].filter(timer=>timer.ms===15000).length,2,'history and channels must bound headers plus JSON body');
  for(const timer of [...timers.values()]) timer.callback();
  await request;
  await settleView();
  assert.equal(historyRequest,null);
  assert.equal(channelRequest,null);
  assert.equal(timers.size,0,'history deadline timers leaked');
  assert.equal(document.getElementById('history-error').hidden,false,'real timeout must remain visible');

  scheduleHistoryRefresh({type:'input'});
  assert.ok([...timers.values()].some(timer=>timer.ms===250));
  document.getElementById('nav-quota').click();
  await settleView();
  assert.equal([...timers.values()].filter(timer=>timer.ms===250).length,0,'leaving history retained its delayed search');
  const count=pending.length;
  // A platform broadcast can still invalidate the hidden history view.
  scheduleHistoryRefresh({type:'change'});
  const debounce=[...timers.values()].find(timer=>timer.ms===250);
  debounce.callback();
  await settleView();
  assert.equal(pending.length,count,'hidden history debounce restarted page requests');
  QuotaModule.cancel();
  await settleView();
}
`+platformBehaviorRunScript)
}

func TestNavigationCachedDocumentCancelsSharedReads(t *testing.T) {
	runPlatformBehavior(t, navigationBehaviorDOMScript+`context.windowListeners=windowListeners;
async function checks() {
  await bootView();
  document.getElementById('nav-settings').click();
  await settleView();
  const original=fetch;
  const pending=[];
  const owned=['/api/metrics','/api/catalog/lock','/api/cline-pass/channels'];
  fetch=(raw,options)=>owned.includes(raw) ? new Promise(resolve=>pending.push({raw,options,resolve})) : original(raw,options);
  const oldCore=refreshCore();
  const oldDiagnostics=loadChannelCatalog();
  assert.equal(pending.length,3);
  for(const handler of windowListeners.pagehide) handler(new Event('pagehide',{persisted:true}));
  assert.ok(pending.every(p=>p.options?.signal?.aborted),'cached-document departure retained shared read owners');

  fetch=async(raw,options)=>{
    if(raw==='/api/metrics') return {ok:true,json:async()=>({proxy_running:true,port:5678,model_counts:{}})};
    if(raw==='/api/catalog/lock') return {ok:true,json:async()=>({synced:true,age_seconds:1})};
    if(raw==='/api/cline-pass/channels') return {ok:true,json:async()=>({channels:{},capture_enabled:false})};
    return original(raw,options);
  };
  for(const handler of windowListeners.pageshow) handler(new Event('pageshow',{persisted:true}));
  await settleView();
  const status=document.getElementById('cline-pass-channel-diagnostics').textContent;
  assert.equal(CONN.fails,0);
  assert.ok(document.getElementById('port-info').textContent.includes('5678'));
  for(const p of pending) p.resolve({ok:false,status:503,text:async()=> 'obsolete shared failure'});
  await Promise.all([oldCore,oldDiagnostics]);
  assert.equal(CONN.fails,0,'old rejection marked the resumed connection unhealthy');
  assert.equal(document.getElementById('cline-pass-channel-diagnostics').textContent,status,'old diagnostics failed the resumed settings page');
}
`+platformBehaviorRunScript)
}

func TestNavigationPreservesSettingsDraftsAndFallbackWrites(t *testing.T) {
	runPlatformBehavior(t, navigationBehaviorDOMScript+`context.windowListeners=windowListeners;
async function checks() {
  await bootView();
  const original=fetch;
  const writes=[];
  const config={port:3456,logging:{level:'info'},models:{default:{provider:'commandcode',model_id:'primary'}},fallbacks:{default:[]}};
  fetch=async(raw,options)=>{
    if(raw==='/api/proxy/config') {
      if(options?.method==='POST') {
        writes.push(JSON.parse(options.body));
        return {ok:true};
      }
      return {ok:true,json:async()=>JSON.parse(JSON.stringify(config))};
    }
    if(raw.startsWith('/api/catalog/models')) return {ok:true,json:async()=>({models:[],scope:'commandcode'})};
    return original(raw,options);
  };
  assert.equal(await loadProxyConfig(),true);
  await FallbackModule.loadConfig();
  const get=id=>document.getElementById(id);
  get('cfg-port').value='4567';
  get('cfg-log-level').value='debug';
  FallbackModule.chains.default=[{provider:'commandcode',model_id:'fallback'}];
  const write=FallbackModule.persist();
  await write;
  assert.equal(writes.length,1);
  const chain=JSON.stringify(FallbackModule.chains);
  for(const tab of ['overview','history','performance','analytics','quota','settings','fallback']) {
    get('nav-'+tab).click();
    await settleView();
    location.hash='#settings';
    await settleView();
    assert.equal(get('cfg-port').value,'4567','navigation erased an unsaved port');
    assert.equal(get('cfg-log-level').value,'debug','navigation erased an unsaved log level');
    assert.equal(JSON.stringify(FallbackModule.chains),chain,'navigation reloaded the editor over its live chain');
  }
  for(const handler of windowListeners.pagehide) handler(new Event('pagehide',{persisted:true}));
  for(const handler of windowListeners.pageshow) handler(new Event('pageshow',{persisted:true}));
  await settleView();
  assert.equal(get('cfg-port').value,'4567');
  assert.equal(get('cfg-log-level').value,'debug');
  assert.equal(JSON.stringify(FallbackModule.chains),chain);
  assert.equal(writes.length,1,'history restoration replayed a confirmed configuration write');
}
`+platformBehaviorRunScript)
}

func TestNavigationCachedDocumentRefreshesActiveSite(t *testing.T) {
	runPlatformBehavior(t, navigationBehaviorDOMScript+`context.windowListeners=windowListeners;
async function checks() {
  await bootView();
  setViewPlatform(null);
  document.getElementById('nav-quota').click();
  await settleView();
  const original=fetch;
  let release;
  fetch=(raw,options)=>raw==='/api/sites' ? new Promise(resolve=>{release=resolve}) : original(raw,options);
  const old=applyActiveSite();
  for(const handler of windowListeners.pagehide) handler(new Event('pagehide',{persisted:true}));
  fetch=async(raw,options)=>raw==='/api/sites' ? {ok:true,json:async()=>({active:'opencode-go',sites:[]})} : original(raw,options);
  for(const handler of windowListeners.pageshow) handler(new Event('pageshow',{persisted:true}));
  await settleView();
  assert.equal(lastActiveSite,'opencode-go','resume waited for the next 30s poll instead of reading current site');
  assert.equal(QuotaModule.provider,'opencode-go');
  release({ok:true,json:async()=>({active:'commandcode',sites:[]})});
  await old;
  assert.equal(lastActiveSite,'opencode-go','an abandoned site read changed the resumed default');
  assert.equal(viewPlatformPinned,null,'resume pinned the server default as a user preference');
}
`+platformBehaviorRunScript)
}

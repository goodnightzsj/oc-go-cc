package gui

import "testing"

func TestCorePollingSingleFlightAndDeadline(t *testing.T) {
	runPlatformBehavior(t, platformBehaviorDOMScript+`
async function checks() {
  document.hidden=true;
  const get=id=>document.getElementById(id);
  const requests=[], timers=new Map(); let timerID=0;
  setTimeout=(callback,ms)=>{assert.equal(ms,15000);timers.set(++timerID,callback);return timerID};
  clearTimeout=id=>timers.delete(id);
  const flush=async()=>{for(let i=0;i<20;i++) await Promise.resolve()};
  fetch=(url,init)=>new Promise((resolve,reject)=>{
    const request={url,init,reject,resolve}; requests.push(request);
    init?.signal.addEventListener('abort',()=>reject(new Error('aborted')),{once:true});
    request.body=()=>resolve({ok:true,json:()=>new Promise((done,fail)=>{
      request.resolveBody=done;
      init?.signal.addEventListener('abort',()=>fail(new Error('body aborted')),{once:true});
    })});
    request.done=n=>resolve({ok:true,json:async()=>url==='/api/metrics' ? {proxy_running:true,port:n,model_counts:{}} : {synced:true,age_seconds:n}});
  });
  const first=refreshCore();
  const repeated=Array.from({length:6},()=>refreshCore());
  assert.equal(requests.length,2,'seven ticks must create only one request per endpoint');
  assert.ok(requests.every(r=>r.init.signal));
  assert.equal(timers.size,2);
  requests[0].body(); await flush();
  requests[1].done(10); await flush();
  const next=refreshCore();
  assert.equal(requests.length,3,'completed catalog must keep polling while metrics body is slow');
  assert.equal(requests[2].url,'/api/catalog/lock');
  requests[2].body(); await flush();
  for(const expire of [...timers.values()]) expire();
  await Promise.all([first,next,...repeated]);
  assert.ok(requests[0].init.signal.aborted && requests[2].init.signal.aborted,'deadline must cover JSON bodies');
  assert.equal(timers.size,0,'settled requests leaked deadline timers');
  const failures=CONN.fails;
  const retry=refreshCore();
  assert.equal(requests.length,5,'timed out requests must release both owners');
  requests[3].done(4567); requests[4].done(20); await retry;
  assert.ok(failures>0,'timeout must feed connection health');
  assert.equal(CONN.fails,0);
  assert.ok(get('port-info').textContent.includes('4567'));
  assert.equal(get('catalog-age').textContent,t('setting.catalogAge').replace('{age}',fmtAge(20)));
  const headerTimeout=refreshCore();
  for(const expire of [...timers.values()]) expire();
  await headerTimeout;
  assert.equal(CONN.fails,1,'one timed out poll counts once regardless of skipped ticks');
  const afterFailure=refreshCore();
  requests[7].reject(new Error('network error')); requests[8].reject(new Error('network error'));
  await afterFailure;
  const recovered=refreshCore();
  requests[9].done(4568); requests[10].done(30); await recovered;
  assert.equal(CONN.fails,0);
}
`+platformBehaviorRunScript)
}

func TestCatalogPollingSyncReplacesOldBody(t *testing.T) {
	runPlatformBehavior(t, platformBehaviorDOMScript+`
async function checks() {
  const requests=[];
  const flush=async()=>{for(let i=0;i<20;i++) await Promise.resolve()};
  fetch=async(url,init)=>{
    if(url==='/api/catalog/sync') {assert.equal(init.method,'POST');return {ok:true}};
    if(url==='/api/catalog/models') return {ok:true,json:async()=>({models:[]})};
    assert.equal(url,'/api/catalog/lock');
    return {ok:true,json:()=>new Promise(resolve=>requests.push({init,resolve}))};
  };
  const old=refreshCatalogAge(); await flush();
  const sync=refreshCatalog(); await flush();
  assert.equal(requests.length,2,'successful sync must refresh even with a poll in flight');
  assert.equal(requests[0].init.signal.aborted,true);
  requests[1].resolve({synced:true,age_seconds:0}); await sync;
  const current=document.getElementById('catalog-age').textContent;
  requests[0].resolve({synced:false}); await old;
  assert.equal(document.getElementById('catalog-age').textContent,current,'old body overwrote synchronized age');
}
`+platformBehaviorRunScript)
}

func TestSlowPollCompletesWithoutStarvation(t *testing.T) {
	runPlatformBehavior(t, platformBehaviorDOMScript+`
async function checks() {
  const get = id => document.getElementById(id);
  const views = [
    {id:'overview-provider',run:()=>refreshOverviewUsage(), count:3,
      body:()=>({summary:{total_requests:7},trend:[],avg_latency_ms:10}), check:()=>assert.equal(get('m-total').textContent,'7')},
    {id:'provider-filter',run:()=>refreshHistory(),count:2,
      body:()=>({items:[],total:7,total_requests:7}),check:()=>assert.equal(historyTotal,7)},
    {id:'perf-provider',run:()=>PerfModule.refresh(),count:1,
      body:()=>[{model:'observed',provider:'workbuddy',count:7}],check:()=>assert.equal(PerfModule.data[0].count,7)},
  ];
  for (const view of views) {
    activeTab = {'overview-provider':'overview','perf-provider':'performance','provider-filter':'history'}[view.id];
    const pending = [];
    fetch = (raw, options) => {
      if (raw.startsWith('/api/history/channels')) return Promise.resolve({ok:true,json:async()=>({channels:[]})});
      return new Promise(resolve=>pending.push({raw,options,resolve}));
    };
    get(view.id).value = 'workbuddy';
    const first = view.run();
    const repeated = Array.from({length:5},()=>view.run());
    assert.equal(pending.length,view.count,'same-query slow polls must not duplicate '+view.id);
    pending.forEach(p=>p.resolve({ok:true,json:async()=>view.body()}));
    await Promise.all([first,...repeated]);
    view.check();
    pending.length = 0;
    const old = view.run();
    get(view.id).value = 'commandcode';
    const next = view.run();
    assert.equal(pending.length,view.count*2,'new query starts immediately');
    assert.ok(pending.slice(0,view.count).every(p=>p.options?.signal.aborted),'old request cancelled');
    pending.slice(view.count).forEach(p=>p.resolve({ok:true,json:async()=>view.body()}));
    await next;
    // A fetch adapter may finish despite abort; stale sequence guards must remain.
    pending.slice(0,view.count).forEach(p=>p.resolve({ok:false,status:503,text:async()=>'old failure'}));
    await old;
    view.check();
  }
  const channels = [];
  fetch = (raw, options) => new Promise(resolve=>channels.push({options,resolve}));
  get('provider-filter').value = 'cline-pass';
  const old = syncChannelFilter();
  await syncChannelFilter();
  assert.equal(channels.length,1,'slow channel lookup must not starve');
  get('provider-filter').value = 'workbuddy';
  const next = syncChannelFilter();
  assert.ok(channels[0].options.signal.aborted);
  channels[1].resolve({ok:true,json:async()=>({channels:['current']})});
  await next;
  channels[0].resolve({ok:true,json:async()=>({channels:['stale']})});
  await old;
  assert.equal(get('channel-filter').children[1].value,'current');
}
`+platformBehaviorRunScript)
}

package gui

import "testing"

func TestPendingResponseBodiesKeepLatestViewOwnership(t *testing.T) {
	runPlatformBehavior(t, platformBehaviorDOMScript+`
async function checks() {
  const get=id=>document.getElementById(id);
  get('analytics-start').value=get('analytics-end').value='2026-11-01';
  get('quota-local-days').value='30';
  const flush=async()=>{for(let i=0;i<20;i++) await Promise.resolve()};
  const data=(raw,n)=>{
    const url=new URL(raw,'http://synthetic.invalid');
    const provider=url.searchParams.get('provider');
    if(url.pathname==='/api/prices') return {tables:{workbuddy:{rules:n}}};
    if(url.pathname==='/api/quota') return {provider,status:'unavailable',accounts:[]};
    if(url.pathname==='/api/perf/models') return [{model:'test',provider,count:n}];
    if(url.pathname==='/api/perf/aggregate') return {avg_latency_ms:n};
    if(url.pathname.includes('/trend')) return {provider,trend:[]};
    return {provider,summary:{total_requests:n},models:[]};
  };
  for(const view of [
    {tab:'overview',id:'overview-provider',run:()=>refreshOverviewUsage(),read:()=>get('m-total').textContent,pending:()=>overviewRequest},
    {tab:'performance',id:'perf-provider',run:()=>PerfModule.refresh(),read:()=>String(PerfModule.data[0].count),pending:()=>PerfModule.request},
    {tab:'analytics',id:'analytics-provider',run:()=>AnalyticsModule.load(true),read:()=>get('kpi-requests').textContent,pending:()=>AnalyticsModule.request && AnalyticsModule.loading},
    {tab:'quota',id:'quota-provider',run:()=>QuotaModule.load(true),read:()=>get('quota-local-requests').textContent,pending:()=>QuotaModule.loading && QuotaModule.localLoading},
  ]) {
    activeTab=view.tab;
    for(const oldFails of [false,true]) {
      const pending=[];
      AnalyticsModule.priceCache=null;
      // Headers arrive immediately; body readers deliberately ignore later abort.
      fetch=async(raw,init)=>({ok:true,json:()=>new Promise((resolve,reject)=>pending.push({raw,init,resolve,reject}))});
      const start=async provider=>{
        get(view.id).value=provider;
        if(view.tab==='quota') QuotaModule.provider=provider;
        const promise=view.run();
        await flush();
        return {promise,end:pending.length};
      };
      const older=await start('workbuddy');
      const middle=await start('openrouter');
      const current=await start('cline-pass');
      assert.ok(older.end>0 && middle.end>older.end && current.end>middle.end,'body consumption was not reached '+view.tab);
      assert.ok(pending.slice(0,middle.end).every(p=>p.init.signal.aborted),'superseded body not cancelled');
      const settle=(items,n)=>items.forEach(p=>oldFails ? p.reject(new Error('stale body failure')) : p.resolve(data(p.raw,n)));
      settle(pending.slice(older.end,middle.end),22);
      await middle.promise;
      assert.ok(view.pending(),'stale body finally cleared current request '+view.tab);
      pending.slice(middle.end).forEach(p=>p.resolve(data(p.raw,100)));
      await current.promise;
      assert.equal(view.read(),'100','new body not published '+view.tab);
      settle(pending.slice(0,older.end),11);
      await older.promise;
      assert.equal(view.read(),'100','old body overwrote current view '+view.tab);
      if(view.tab==='analytics') {
        assert.equal(AnalyticsModule.priceCache.data.tables.workbuddy.rules,100,'stale price body overwrote cache');
        assert.equal(get('analytics-error').hidden,true,'stale body error surfaced');
      }
      if(view.tab==='quota') assert.equal(QuotaModule.accountError,'');
      assert.ok(!view.pending(),'completed current request remained pending');
      cancelTabRequests(view.tab);
    }
  }
}
`+platformBehaviorRunScript)
}

func TestPendingChannelBodiesKeepLatestDiagnostics(t *testing.T) {
	runPlatformBehavior(t, platformBehaviorDOMScript+`
async function checks() {
  const flush=async()=>{for(let i=0;i<20;i++) await Promise.resolve()};
  const get=id=>document.getElementById(id);
  const data=n=>({capture_enabled:false,channels:{},diagnostics:{enabled:true,results:[{
    model:'model-'+n,pipeline:'planner',verdict:'matched',endpoint:'https://example.invalid/',
    key_hint:'hint-'+n,checked_at:'2026-11-16T00:00:00Z',expires_at:'2026-11-16T06:00:00Z',
  }]}});
  for(const oldFails of [false,true]) {
    const pending=[];
    fetch=async(_raw,init)=>({ok:true,json:()=>new Promise((resolve,reject)=>pending.push({init,resolve,reject}))});
    const older=loadChannelCatalog(); await flush();
    const middle=loadChannelCatalog(); await flush();
    const current=loadChannelCatalog(); await flush();
    assert.equal(pending.length,3,'all headers must arrive before body completion');
    assert.ok(pending.slice(0,2).every(p=>p.init.signal.aborted));
    const settle=(p,n)=>oldFails ? p.reject(new Error('stale body failure')) : p.resolve(data(n));
    settle(pending[1],22);
    await middle;
    assert.ok(channelCatalogRequest,'stale finally cleared current channel request');
    pending[2].resolve(data(100)); await current;
    const latest=get('cline-pass-channel-diagnostics').textContent;
    assert.ok(latest.includes('model-100'),'new diagnostics never rendered');
    settle(pending[0],11); await older;
    assert.equal(get('cline-pass-channel-diagnostics').textContent,latest);
    assert.equal(channelCatalogRequest,null);
    assert.equal(get('cline-pass-channel-source').classList.contains('is-error'),false);
  }
}
`+platformBehaviorRunScript)
}

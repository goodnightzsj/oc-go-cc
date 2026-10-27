package gui

import "testing"

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

package gui

import "testing"

func TestActiveSiteRequestLifecycle(t *testing.T) {
	runPlatformBehavior(t, navigationBehaviorDOMScript+`context.windowListeners=windowListeners;
async function checks() {
  await bootView();
  const original=fetch, pending=[], timers=new Map(); let id=0;
  setTimeout=(callback,ms)=>{timers.set(++id,{callback,ms});return id};
  clearTimeout=id=>timers.delete(id);
  fetch=(raw,options)=>raw==='/api/sites' ? new Promise((resolve,reject)=>{
    pending.push({options,resolve});
    options?.signal?.addEventListener('abort',()=>reject(new Error('aborted')));
  }) : original(raw,options);
  const first=applyActiveSite();
  for(let i=0;i<6;i++) await applyActiveSite();
  assert.equal(pending.length,1,'slow polls must not accumulate');
  assert.equal([...timers.values()].filter(t=>t.ms===15000).length,1);
  for(const timer of [...timers.values()]) timer.callback();
  assert.equal(await first,false);
  assert.equal(pending[0].options.signal.aborted,true);
  assert.equal(timers.size,0);

  const next=applyActiveSite();
  document.hidden=true;
  handleVisibilityChange();
  await next;
  assert.equal(pending[1].options.signal.aborted,true);
  await applyActiveSite();
  assert.equal(pending.length,2,'hidden document must not poll');
  document.hidden=false;
  handleVisibilityChange();
  assert.equal(pending.length,3,'visible document must reload immediately');
  for(const handler of windowListeners.pagehide) handler(new Event('pagehide',{persisted:true}));
  await settleView();
  assert.equal(pending[2].options.signal.aborted,true);
  assert.equal(activeSiteRequest,null);
}
`+platformBehaviorRunScript)
}

func TestActiveSiteBodyDeadlineAndLateResult(t *testing.T) {
	runPlatformBehavior(t, navigationBehaviorDOMScript+`
async function checks() {
  await bootView();
  const original=fetch, pending=[], timers=new Map();let id=0;
  setTimeout=(callback,ms)=>{timers.set(++id,{callback,ms});return id};
  clearTimeout=id=>timers.delete(id);
  fetch=async(raw,options)=>raw==='/api/sites' ? {ok:true,json:()=>new Promise(resolve=>pending.push({options,resolve}))} : original(raw,options);
  const old=applyActiveSite();
  await settleView();
  for(const timer of [...timers.values()]) timer.callback();
  assert.equal(pending[0].options.signal.aborted,true,'deadline must cover response body');
  const current=applyActiveSite(true);
  await settleView();
  const owner=activeSiteRequest;
  pending[0].resolve({active:'commandcode'});
  assert.equal(await old,false);
  assert.equal(activeSiteRequest,owner,'old finally cleared the new owner');
  pending[1].resolve({active:'opencode-go'});
  assert.equal(await current,true);
  assert.equal(lastActiveSite,'opencode-go');
  assert.equal(activeSiteRequest,null);
  await settleView();
  assert.equal([...timers.values()].filter(t=>t.ms===15000).length,0,'completed reads must clear their deadlines');
}
`+platformBehaviorRunScript)
}

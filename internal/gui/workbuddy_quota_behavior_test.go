package gui

import "testing"

func TestWorkBuddyQuotaBehavior(t *testing.T) {
	runPlatformBehavior(t, workBuddyQuotaBehaviorScript)
}

const workBuddyQuotaBehaviorScript = platformBehaviorDOMScript + `
async function checks() {
  const get = id => document.getElementById(id);
  const totals = {requests:3,errors:1,prompt_tokens:100,completion_tokens:20,total_tokens:120,cache_hit_tokens:40,cache_miss_tokens:60,credits:0,credit_samples:2,avg_latency_ms:250,avg_tokens_per_second:5};
  const report = {
    overview:{total:3,healthy:1,cooling:1,disabled:0,paused:1,in_flight_full:0,balance_refresh_sec:300,balance_refreshed_at:'2026-10-23T12:00:00+08:00',accounts:[
      {realm:'cn',credits:0,credits_total:200,paused:true},
      {realm:'global',credits:999999999,credits_total:-1},
      {realm:'<realm>',credits:null,credits_total:0,cooling:true},
    ]},
    usage:{hours:24,generated:'2026-10-24T12:00:00+08:00',totals,by_model:[
      {key:'global:free',...totals},
      {key:'cn:<unknown>',...totals,credits:0,credit_samples:0},
      {key:'cn:tiny-credit',...totals,credits:0.00002,credit_samples:1},
    ],series:[{t:'2026-10-24T11',scope:'hour',...totals},{t:'2026-10-24T13',scope:'hour',...totals,total_tokens:0,credit_samples:0}]},
  };
  const makeView = () => ({provider:'workbuddy',status:'available',source:'gateway_api',accounts:[{key_hint:'masked-WB01',workbuddy:report}],links:[]});
  let view = makeView();
  const calls = [];
  fetch = async raw => {
    const url = new URL(raw,'http://synthetic.invalid');
    calls.push(url);
    if (url.pathname === '/api/quota') return {ok:true,json:async()=>view};
    if (url.pathname === '/api/analytics/summary') return {ok:true,json:async()=>({provider:url.searchParams.get('provider'),summary:{total_requests:1,input_tokens:5,output_tokens:3,cache_read_tokens:0,cache_creation_tokens:0,unknown_cost_requests:0,est_cost_usd:0.01},models:[]})};
    throw new Error('unexpected URL: ' + raw);
  };
  get('quota-provider').value = 'workbuddy';
  await get('quota-provider').emit('change');
  assert.equal(calls.length,2);
  assert.equal(calls[0].searchParams.get('provider'),'workbuddy');
  assert.equal(get('quota-workbuddy').hidden,false);
  assert.equal(get('quota-unavailable').hidden,true);
  assert.equal(get('quota-go').hidden,true);
  assert.equal(get('quota-source').textContent,t('quota.source.gateway_api'));
  assert.equal(get('quota-local-cost-label').textContent,t('workbuddy.localCost'));
  assert.equal(get('quota-local-requests').textContent,'1','local requests must not be replaced by gateway attempts');
  let html = get('quota-workbuddy-accounts').innerHTML;
  assert.ok(html.includes('<td>0</td><td>200</td>'),'known zero balance retained');
  assert.ok(html.includes(t('workbuddy.unlimited')) && !html.includes('999,999,999'),'unlimited sentinel must not look like finite credits');
  assert.ok(html.includes('<td>—</td><td>—</td>'),'unknown balance and ceiling stay unknown');
  assert.ok(html.includes('&lt;realm&gt;') && !html.includes('<realm>'),'external realm is escaped');
  assert.ok(html.includes('cn:&lt;unknown&gt;') && !html.includes('cn:<unknown>'),'external model is escaped');
  assert.ok(html.includes('global:free</th><td>3</td><td>1</td><td>120</td><td>0</td><td>2</td>'),'observed zero is shown');
  assert.ok(html.includes('cn:&lt;unknown&gt;</th><td>3</td><td>1</td><td>120</td><td>—</td><td>0</td>'),'no credit samples must not claim free usage');
  assert.ok(html.includes('cn:tiny-credit</th><td>3</td><td>1</td><td>120</td><td>0.00002</td><td>1</td>'),'small nonzero credits must not round to zero');
  assert.ok(!html.includes('$'),'upstream credits must not acquire a dollar sign');
  assert.ok(html.includes('2026-10-24 11:00') && html.includes('2026-10-24 13:00') && !html.includes('2026-10-24 12:00'),'hour buckets retain gateway calendar time without filling gaps');
  assert.ok(html.includes('height:0px') && html.includes('<details'),'zero bar and accessible hourly table');
  assert.ok(html.includes(t('workbuddy.creditNote')) && html.includes(t('workbuddy.usageNote')));
  assert.ok(page.includes('id="quota-workbuddy" hidden aria-live="polite"'));

  view = {...makeView(),status:'partial',accounts:[{key_hint:'masked-WB01',workbuddy:{overview:report.overview,usage_error:'HTTP 501 <missing>'}}]};
  await QuotaModule.load(true);
  html = get('quota-workbuddy-accounts').innerHTML;
  assert.ok(html.includes('HTTP 501 &lt;missing&gt;') && html.includes('200'),'usage failure preserves pool');
  assert.ok(!html.includes('global:free'),'failed refresh clears stale usage');
  view = {...makeView(),status:'partial',accounts:[{key_hint:'masked-WB01',workbuddy:{usage:report.usage,overview_error:'HTTP 403 <denied>'}},{key_hint:'masked-WB02',error:'HTTP 401'}]};
  await QuotaModule.load(true);
  html = get('quota-workbuddy-accounts').innerHTML;
  assert.ok(html.includes('HTTP 403 &lt;denied&gt;') && html.includes('global:free'),'pool failure preserves usage');
  assert.ok(html.includes('masked-WB02') && html.includes('HTTP 401'),'rejected key remains visible');
  view = {...makeView(),accounts:[{workbuddy:{overview:{...report.overview,total:0,accounts:[],balance_refreshed_at:'0001-01-01T00:00:00Z'},usage:{...report.usage,totals:{...totals,requests:0,errors:0,credit_samples:0,credits:0,prompt_tokens:0,completion_tokens:0,total_tokens:0,avg_latency_ms:0,avg_tokens_per_second:0},by_model:[],series:[]}}}]};
  await QuotaModule.load(true);
  html = get('quota-workbuddy-accounts').innerHTML;
  assert.ok(html.includes(t('workbuddy.noAccounts')) && html.includes(t('workbuddy.noUsage')),'empty pool and empty window are explicit');
  assert.ok(!html.includes('0001') && !html.includes('daily-bars'),'no fake refresh time or empty chart');
  view = {...makeView(),status:'not_configured',accounts:[]};
  await QuotaModule.load(true);
  assert.ok(get('quota-workbuddy-accounts').innerHTML.includes(t('quota.noProviderKey').replace('{provider}','WorkBuddy')));

  const normalFetch = fetch;
  let release;
  fetch = raw => raw.startsWith('/api/quota?provider=workbuddy') ? new Promise(resolve => {release=resolve}) : normalFetch(raw);
  const old = QuotaModule.load(true);
  view = {provider:'cline-pass',status:'not_configured',source:'official_api',accounts:[]};
  get('quota-provider').value = 'cline-pass';
  await get('quota-provider').emit('change');
  release({ok:true,json:async()=>makeView()});
  await old;
  assert.equal(get('quota-workbuddy').hidden,true,'late WorkBuddy data cannot replace current platform');
  assert.equal(QuotaModule.view.provider,'cline-pass');
  assert.equal(get('quota-local-cost-label').textContent,t('history.filteredCost'));
  assert.equal(get('btn-refresh-quota').disabled,false);
  for (const lang of ['en','zh']) {
    currentLang = lang;
    for (const key of ['quota.source.gateway_api','workbuddy.accountScope','workbuddy.pool','workbuddy.usage','workbuddy.creditNote','workbuddy.noUsage','workbuddy.unlimited','workbuddy.localCost','workbuddy.hourDetails']) assert.notEqual(t(key),key);
  }
}
` + platformBehaviorRunScript

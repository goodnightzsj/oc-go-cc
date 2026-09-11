package gui

import "testing"

func TestPlatformQuotaBehavior(t *testing.T) {
	runPlatformBehavior(t, platformQuotaBehaviorScript)
}

const platformQuotaBehaviorScript = platformBehaviorDOMScript + `
async function checks() {
  const get = id => document.getElementById(id);
  const providers = ['opencode-go','opencode-zen','aws-bedrock','openrouter','commandcode'];
  const selectProvider = provider => {get('quota-provider').value=provider; return get('quota-provider').emit('change')};
  const local = provider => ({provider,summary:{total_requests:3,known_requests:2,input_tokens:10,output_tokens:4,cache_read_tokens:2,cache_creation_tokens:1,est_cost_usd:0.25,unknown_cost_requests:1},models:[{provider,model:'shared',requests:3,input_tokens:10,output_tokens:4,cache_read_tokens:2,cache_creation_tokens:1,est_cost_usd:0.25,unknown_cost_requests:1}]});
  const go = {provider:'opencode-go',status:'available',source:'upstream_api',currency:'USD',accounts:[{key_hint:'masked-GO01',report:{plan:'go',weekly:{has_percent:true,used_percent:25,used_dollars:7.5,limit_dollars:30,resets_at:'2030-01-01T00:00:00Z'},fetched_at:'2026-09-10T00:00:00Z'}}],model_limits:{models:[{model:'shared',allowance_usd:60}]},model_usage:[{model:'shared',used_usd:0.25,allowance_usd:60,percent:0.25/60*100,requests:3,unknown_cost_requests:0}],links:[{kind:'docs',url:'https://synthetic.invalid/go/docs'}],fetched_at:'2026-09-10T00:00:00Z'};
  const key = {limit:null,limit_remaining:null,limit_reset:null,usage:12,usage_daily:0,usage_weekly:null,usage_monthly:10,byok_usage:3,byok_usage_daily:1,byok_usage_weekly:null,byok_usage_monthly:2,include_byok_in_limit:false,is_free_tier:false,expires_at:null};
  let openrouter = {provider:'openrouter',status:'partial',source:'official_api',currency:'USD',credits_status:'not_configured',accounts:[{key_hint:'masked-OR01',openrouter:key},{key_hint:'masked-OR02',openrouter:{...key,limit:0,limit_remaining:0,limit_reset:'daily',usage:0,byok_usage:0,include_byok_in_limit:null}},{key_hint:'masked-BAD1',error:'synthetic HTTP 401 <denied>'}],links:[{kind:'billing',url:'https://synthetic.invalid/openrouter/credits'}]};
  const unavailable = provider => ({provider,status:provider === 'aws-bedrock' || provider === 'commandcode' ? 'not_configured' : 'unavailable',source:provider === 'commandcode' ? 'official_alpha_api' : provider === 'aws-bedrock' ? 'official_api' : 'none',accounts:[],reason:provider === 'aws-bedrock' ? 'aws_billing_disabled' : provider === 'commandcode' ? '' : 'no_public_account_api',links:[{kind:'billing',url:'https://synthetic.invalid/' + provider + '/billing'}]});
  let calls = [];
  const response = raw => {
    const url = new URL(raw,'http://synthetic.invalid');
    const provider = url.searchParams.get('provider');
    if (!providers.includes(provider)) throw new Error('missing/incorrect provider scope: ' + raw);
    if (url.pathname === '/api/analytics/summary') return {ok:true,json:async()=>local(provider)};
    if (url.pathname !== '/api/quota') throw new Error('unexpected synthetic URL: ' + raw);
    return {ok:true,json:async()=>provider === 'opencode-go' ? go : provider === 'openrouter' ? openrouter : unavailable(provider)};
  };
  const normalFetch = async raw => {calls.push(new URL(raw,'http://synthetic.invalid')); return response(raw)};
  fetch = normalFetch;

  const quotaSelect = page.match(/<select[^>]*id="quota-provider"[^>]*>([\s\S]*?)<\/select>/);
  assert.equal(JSON.stringify([...quotaSelect[1].matchAll(/value="([^"]*)"/g)].map(match=>match[1])),JSON.stringify(providers));
  assert.ok(page.includes('id="quota-local-error" hidden role="alert"'));
  assert.ok(page.includes('type="password" id="cfg-openrouter-management-key"'));
  assert.ok(CONFIG_FIELDS.some(field=>field[0] === 'openrouter.management_api_key' && field[1] === 'cfg-openrouter-management-key'));
  for (const provider of providers) {
    calls = [];
    await selectProvider(provider);
    assert.equal(calls.length,2,'account capability and local ledger are separate requests');
    assert.ok(calls.every(url=>url.searchParams.get('provider') === provider));
    assert.equal(calls.find(url=>url.pathname === '/api/quota').searchParams.get('refresh'),'1');
    assert.equal(calls.find(url=>url.pathname === '/api/analytics/summary').searchParams.get('days'),'30');
    assert.equal(get('quota-go').hidden,provider !== 'opencode-go');
    assert.equal(get('quota-openrouter').hidden,provider !== 'openrouter');
    assert.equal(get('quota-bedrock').hidden,provider !== 'aws-bedrock');
    assert.equal(get('quota-commandcode').hidden,provider !== 'commandcode');
    assert.equal(get('quota-unavailable').hidden,provider !== 'opencode-zen');
    assert.equal(calls.find(url=>url.pathname === '/api/quota').searchParams.has('billing_refresh'),false,'changing provider never initiates a paid query');
    assert.equal(get('quota-local-requests').textContent,'3');
    assert.equal(get('quota-local-tokens').textContent,'17');
    assert.equal(get('quota-local-cost').textContent,'Known $0.250');
    assert.equal(get('quota-local-unknown').textContent,'1');
    assert.ok(get('quota-local-model-tbody').innerHTML.includes('<small>' + provider + '</small>'));
    assert.ok(get('quota-local-note').textContent.includes(PROVIDERS[provider].name));
    assert.ok(get('quota-local-note').textContent.includes('not an account bill or balance'));
    assert.equal(get('quota-local-error').hidden,true);
    assert.equal(get('btn-refresh-quota').disabled,false,'refresh must remain enabled on every platform');
    if (provider === 'opencode-go') {
      assert.equal(get('quota-remaining').textContent,'$22.50','Go window gauges are preserved');
      assert.ok(get('quota-accounts').innerHTML.includes('quota-unit'));
      assert.ok(get('quota-model-limits').innerHTML.includes('shared'));
    } else if (provider === 'aws-bedrock') {
      assert.ok(get('quota-bedrock-body').innerHTML.includes(t('quota.reason.aws_billing_disabled')));
      assert.equal(get('btn-fetch-bedrock-billing').disabled,true);
      assert.equal(get('quota-source').textContent,t('quota.source.official_api'));
    } else if (provider === 'commandcode') {
      assert.ok(get('quota-commandcode-accounts').innerHTML.includes('CommandCode'));
      assert.equal(get('quota-source').textContent,t('quota.source.official_alpha_api'));
    } else if (provider !== 'openrouter') {
      assert.ok(get('quota-unavailable-note').textContent.includes('No public account quota API'));
      assert.equal(get('quota-source').textContent,t('quota.source.none'));
      assert.ok(get('quota-links').innerHTML.includes('/' + provider + '/billing'));
    }
  }
  calls = [];
  await QuotaModule.load(false);
  assert.equal(calls.length,0,'unforced same-scope reloads must respect the throttle');
  await QuotaModule.load(true);
  assert.equal(calls.length,2,'manual refresh must bypass the browser throttle');
  for (const days of ['7','90','30']) {
    calls = [];
    get('quota-local-days').value = days;
    await get('quota-local-days').emit('change');
    assert.equal(calls.length,1,'changing the local period must not refetch account data');
    assert.equal(calls[0].pathname,'/api/analytics/summary');
    assert.equal(calls[0].searchParams.get('days'),days);
    assert.equal(calls[0].searchParams.get('provider'),'commandcode');
  }

  await selectProvider('openrouter');
  let accountHTML = get('quota-openrouter-accounts').innerHTML;
  assert.ok(accountHTML.includes(t('openrouter.noLimit')),'a null key limit must be named as an uncapped key');
  assert.ok(!accountHTML.includes(t('openrouter.balance')),'key balances must not masquerade as account balances');
  assert.ok(accountHTML.includes('<td>All Time</td><td>$12.00</td><td>$3.00</td>'),'regular and BYOK usage must remain separate');
  assert.ok(accountHTML.includes('<td>Day</td><td>$0.00</td><td>$1.00</td>'),'known zero and known BYOK daily usage stay visible');
  assert.ok(accountHTML.includes('<td>Weekly</td><td>—</td><td>—</td>'),'missing periods must not become zero');
  assert.ok(accountHTML.includes('<dt>Key spending cap</dt><dd>$0.00</dd>'),'a zero cap is not uncapped');
  assert.ok(accountHTML.includes('masked-OR01') && accountHTML.includes('masked-BAD1'),'one rejected key must not hide other keys');
  assert.ok(accountHTML.includes('synthetic HTTP 401 &lt;denied&gt;') && !accountHTML.includes('<denied>'),'errors must be escaped and visible');
  assert.ok(get('quota-openrouter-credits').innerHTML.includes(t('openrouter.noManagementKey')));
  assert.ok(!get('quota-openrouter-credits').innerHTML.includes('$0.00'),'unqueried account credit is not zero');
  openrouter = {...openrouter,credits_status:'available',credits:{total_credits:100,total_usage:120},credits_endpoint:'https://synthetic.invalid/credits'};
  await QuotaModule.load(true);
  assert.ok(get('quota-openrouter-credits').innerHTML.includes('<dt>Account balance</dt><dd>$-20.00</dd>'),'negative account balances must not be clamped or replaced by sums of key limits');
  openrouter = {...openrouter,credits_status:'error',credits:undefined,credits_error:'synthetic management HTTP 403'};
  await QuotaModule.load(true);
  assert.ok(get('quota-openrouter-credits').innerHTML.includes('synthetic management HTTP 403'));
  assert.ok(get('quota-openrouter-accounts').innerHTML.includes('masked-OR01'),'management lookup failure must not hide valid inference-key data');
  openrouter = {...openrouter,credits_status:'available',credits:{total_credits:0,total_usage:0}};
  await QuotaModule.load(true);
  assert.ok(get('quota-openrouter-credits').innerHTML.includes('<dt>Account balance</dt><dd>$0.00</dd>'),'a known zero balance remains zero');

  fetch = async raw => raw.startsWith('/api/quota?') ? {ok:false,status:503,text:async()=> 'account API unavailable'} : response(raw);
  await QuotaModule.load(true);
  assert.equal(get('quota-error').hidden,false);
  assert.ok(get('quota-error').textContent.includes('503'));
  assert.equal(get('quota-local-error').hidden,true);
  assert.equal(get('quota-local-requests').textContent,'3','account failure must not hide local records');
  assert.ok(!get('quota-openrouter-credits').innerHTML.includes('$0.00'),'failed account data must not keep a stale balance');
  fetch = async raw => raw.startsWith('/api/analytics/summary?') ? {ok:false,status:503,text:async()=> 'local storage unavailable'} : response(raw);
  await QuotaModule.load(true);
  assert.equal(get('quota-error').hidden,true);
  assert.equal(get('quota-local-error').hidden,false);
  assert.equal(get('quota-local-requests').textContent,'—');
  assert.ok(get('quota-openrouter-accounts').innerHTML.includes('masked-OR01'),'local storage failure must not hide official data');
  assert.ok(!get('quota-local-model-tbody').innerHTML.includes('shared'),'failed local refresh must clear stale model records');
  fetch = async raw => raw.startsWith('/api/analytics/summary?')
    ? {ok:true,json:async()=>({provider:'openrouter',summary:{total_requests:0,input_tokens:0,output_tokens:0,cache_read_tokens:0,cache_creation_tokens:0,est_cost_usd:0,unknown_cost_requests:0},models:null})} : response(raw);
  await QuotaModule.load(true);
  assert.equal(get('quota-local-requests').textContent,'0');
  assert.equal(get('quota-local-tokens').textContent,'0');
  assert.equal(get('quota-local-cost').textContent,'$0.00');
  assert.ok(get('quota-local-model-tbody').innerHTML.includes(t('analytics.noData')));
  fetch = async raw => raw.startsWith('/api/analytics/summary?')
    ? {ok:true,json:async()=>({...local('openrouter'),summary:{...local('openrouter').summary,est_cost_usd:0,unknown_cost_requests:3}})} : response(raw);
  await QuotaModule.load(true);
  assert.equal(get('quota-local-cost').textContent,'—','entirely unpriced local traffic is not free');
  assert.equal(get('quota-local-unknown').textContent,'3');
  fetch = async raw => raw.startsWith('/api/analytics/summary?') ? {ok:true,json:async()=>null} : response(raw);
  await QuotaModule.load(true);
  assert.equal(get('quota-local-error').hidden,false);
  assert.equal(get('quota-local-cost').textContent,'—');

  for (const delayedPath of ['/api/quota','/api/analytics/summary','both']) {
    for (const oldFails of [false,true]) {
      const pending = [];
      fetch = raw => {
        const url = new URL(raw,'http://synthetic.invalid');
        return url.searchParams.get('provider') === 'opencode-go' && (url.pathname === delayedPath || delayedPath === 'both')
          ? new Promise(resolve=>pending.push({raw,resolve})) : Promise.resolve(response(raw));
      };
      const previous = selectProvider('opencode-go');
      const latest = selectProvider('commandcode');
      assert.ok(!get('quota-local-model-tbody').innerHTML.includes('opencode-go'),'provider switching must clear previous local rows immediately');
      await latest;
      for (const item of pending) item.resolve(oldFails ? {ok:false,status:401,text:async()=> 'old account failure'} : response(item.raw));
      await previous;
      assert.equal(QuotaModule.view.provider,'commandcode');
      assert.equal(QuotaModule.localView.provider,'commandcode');
      assert.equal(get('quota-go').hidden,true,'late Go data must not replace CommandCode');
      assert.equal(get('quota-error').hidden,true,'stale account errors must be ignored');
      assert.equal(get('quota-local-error').hidden,true,'stale local errors must be ignored');
      assert.equal(get('btn-refresh-quota').disabled,false);
    }
  }
  let pending = [];
  fetch = raw => new URL(raw,'http://synthetic.invalid').searchParams.get('provider') === 'opencode-go'
    ? new Promise(resolve=>pending.push({raw,resolve}))
    : Promise.resolve(raw.startsWith('/api/quota?') ? {ok:false,status:503,text:async()=> 'current account failure'} : response(raw));
  const previous = selectProvider('opencode-go');
  await selectProvider('commandcode');
  for (const item of pending) item.resolve(response(item.raw));
  await previous;
  assert.equal(QuotaModule.view,null,'old success must not revive a failed current platform');
  assert.equal(get('quota-error').hidden,false);
  assert.equal(QuotaModule.localView.provider,'commandcode');
  assert.equal(get('quota-go').hidden,true);

  fetch = normalFetch;
  await selectProvider('commandcode');
  pending = [];
  fetch = raw => new URL(raw,'http://synthetic.invalid').searchParams.get('days') === '7'
    ? new Promise(resolve=>pending.push({raw,resolve})) : Promise.resolve(response(raw));
  get('quota-local-days').value = '7';
  const oldPeriod = get('quota-local-days').emit('change');
  get('quota-local-days').value = '90';
  await get('quota-local-days').emit('change');
  for (const item of pending) item.resolve({ok:true,json:async()=>({...local('commandcode'),summary:{...local('commandcode').summary,total_requests:99}})});
  await oldPeriod;
  assert.equal(get('quota-local-requests').textContent,'3','late period data must not overwrite the selected period');
  assert.ok(get('quota-local-note').textContent.includes('90'));

  QuotaModule.view = {...unavailable('commandcode'),links:[{kind:'billing',url:'javascript:alert(1)'},{kind:'usage',url:'https://synthetic.invalid/usage'}]};
  QuotaModule.render();
  assert.ok(!get('quota-links').innerHTML.includes('javascript:'));
  assert.ok(get('quota-links').innerHTML.includes('https://synthetic.invalid/usage'));
  for (const lang of ['en','zh']) {
    currentLang = lang;
    for (const key of ['quota.localTitle','quota.localNote','quota.reason.no_public_account_api','quota.reason.aws_billing_disabled','openrouter.noLimit','openrouter.managementHint','openrouter.creditsFail']) assert.notEqual(t(key),key,'missing translation: ' + key);
  }
  currentLang = 'en';
  const mask = '••••••••••••••••';
  const config = {openrouter:{api_key:mask,management_api_key:mask}};
  let saved;
  fetch = async (url,options) => {
    assert.equal(url,'/api/proxy/config');
    if (options?.method === 'POST') saved = JSON.parse(options.body);
    return {ok:true,json:async()=>config};
  };
  await loadProxyConfig();
  const management = CONFIG_FIELDS.find(field=>field[0] === 'openrouter.management_api_key');
  assert.equal(readFieldValue(management),undefined,'unchanged masked management key must not be saved');
  get('cfg-openrouter-management-key').value = 'synthetic-management-key';
  await saveProxyConfig();
  assert.equal(JSON.stringify(saved),'{"openrouter":{"management_api_key":"synthetic-management-key"}}','management edits must not change inference keys or other platforms');
}
` + platformBehaviorRunScript

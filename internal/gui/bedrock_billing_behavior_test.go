package gui

import "testing"

func TestBedrockBillingPageBehavior(t *testing.T) {
	runPlatformBehavior(t, bedrockBillingBehaviorScript)
}

const bedrockBillingBehaviorScript = platformBehaviorDOMScript + `
async function checks() {
  const get = id => document.getElementById(id);
  const select = async provider => {get('quota-provider').value=provider; await get('quota-provider').emit('change')};
  const empty = {provider:'aws-bedrock',status:'unavailable',source:'official_api',accounts:[],links:[],reason:'aws_billing_refresh_required'};
  const bill = {linked_account_id:'123456789012',start_date:'2026-08-11',end_date:'2026-09-10',metric:'UnblendedCost',services:['Amazon Bedrock <synthetic>'],currency:'EUR',total_cost:-1.25,estimated:true,daily:[{date:'2026-09-08',cost:-2.5,estimated:false},{date:'2026-09-09',cost:1.25,estimated:true}]};
  let view = empty;
  let calls = [];
  const response = raw => {
    const url = new URL(raw,'http://synthetic.invalid');
    const provider = url.searchParams.get('provider');
    if (url.pathname === '/api/quota') return {ok:true,json:async()=>provider === 'aws-bedrock' ? view : {provider,status:'unavailable',source:'none',accounts:[],links:[],reason:'no_public_account_api'}};
    assert.equal(url.pathname,'/api/analytics/summary');
    return {ok:true,json:async()=>({provider,summary:{total_requests:0,input_tokens:0,output_tokens:0,cache_read_tokens:0,cache_creation_tokens:0,est_cost_usd:0,unknown_cost_requests:0},models:[]})};
  };
  const normalFetch = async (raw, options) => {calls.push({url:new URL(raw,'http://synthetic.invalid'),method:options?.method || 'GET'}); return response(raw)};
  fetch = normalFetch;
  await select('aws-bedrock');
  assert.equal(get('quota-bedrock').hidden,false);
  assert.equal(get('btn-fetch-bedrock-billing').disabled,false);
  assert.ok(get('quota-bedrock-body').innerHTML.includes(t('quota.reason.aws_billing_refresh_required')));
  assert.ok(calls.every(call=>call.method === 'GET' && !call.url.searchParams.has('billing_refresh')),'navigation never spends on AWS');
  calls = [];
  await get('btn-refresh-quota').emit('click');
  assert.ok(calls.every(call=>call.method === 'GET' && !call.url.searchParams.has('billing_refresh')),'ordinary refresh never spends on AWS');

  view = {...empty,status:'available',reason:undefined,currency:'EUR',bedrock_billing:bill};
  calls = [];
  await get('btn-fetch-bedrock-billing').emit('click');
  assert.equal(calls.length,1,'billing query must not issue extra local or account calls');
  assert.equal(calls[0].method,'POST');
  assert.equal(calls[0].url.searchParams.get('provider'),'aws-bedrock');
  assert.equal(calls[0].url.searchParams.get('billing_refresh'),'1');
  const html = get('quota-bedrock-body').innerHTML;
  assert.ok(html.includes('-€1.25'),'negative cost and reported currency are preserved');
  assert.ok(!html.includes('$'),'AWS EUR must not be rendered as local USD');
  for (const text of ['123456789012','2026-08-11','2026-09-10','EUR',t('aws.billingEstimated'),t('aws.billingReported')]) assert.ok(html.includes(text),text);
  assert.ok(html.includes('Amazon Bedrock &lt;synthetic&gt;') && !html.includes('<synthetic>'),'service names are escaped');
  assert.equal(get('quota-local-cost').textContent,'$0.00','local USD and official AWS costs are separate');
  assert.ok(page.includes('data-i18n="aws.billingScope"') && page.includes('aria-describedby="bedrock-billing-query-hint"'));

  for (const value of [0,null]) {
    view = {...view,status:value === null ? 'unavailable' : 'available',reason:value === null ? 'aws_billing_no_data' : undefined,bedrock_billing:{...bill,total_cost:value}};
    await QuotaModule.load(true);
    assert.ok(get('quota-bedrock-body').innerHTML.includes(value === null ? t('quota.reason.aws_billing_no_data') : '€0.00'));
    if (value === null) assert.ok(!get('quota-bedrock-body').innerHTML.includes('€0.00'),'unavailable cost must not become zero');
  }
  view = {...empty,status:'error',error:'synthetic AWS access denied <error>'};
  await QuotaModule.load(true);
  assert.ok(get('quota-bedrock-body').innerHTML.includes('synthetic AWS access denied &lt;error&gt;'));
  assert.ok(!get('quota-bedrock-body').innerHTML.includes('-€1.25'),'errors clear stale bills');
  assert.equal(get('quota-local-cost').textContent,'$0.00');
  view = {...empty,status:'not_configured',reason:'aws_billing_disabled'};
  await QuotaModule.load(true);
  assert.equal(get('btn-fetch-bedrock-billing').disabled,true,'disabled billing cannot be invoked from the page');

  let release;
  view = {...empty,status:'available',reason:undefined,currency:'EUR',bedrock_billing:bill};
  fetch = (raw, options) => options?.method === 'POST' ? new Promise(resolve=>{release=()=>resolve(response(raw))}) : normalFetch(raw,options);
  const pending = QuotaModule.loadAccounts(true,true);
  assert.equal(get('btn-fetch-bedrock-billing').disabled,true,'duplicate clicks are disabled');
  await select('commandcode');
  release();
  await pending;
  assert.equal(QuotaModule.view.provider,'commandcode','late AWS data cannot replace another platform');
  assert.equal(get('quota-bedrock').hidden,true);
  assert.equal(get('quota-local-cost').textContent,'$0.00');

  for (const lang of ['en','zh']) {
    currentLang=lang;
    for (const key of ['aws.billingTitle','aws.billingQuery','aws.billingHint','aws.billingScope','aws.billingEnable','aws.billingProfile','aws.billingConfigHint','aws.billingAccount','aws.billingEstimated','aws.billingReported','quota.reason.aws_billing_disabled','quota.reason.aws_billing_no_data','quota.reason.aws_billing_refresh_required']) assert.notEqual(t(key),key);
    QuotaModule.renderBedrockBilling({provider:'aws-bedrock',status:'available',bedrock_billing:bill});
    assert.ok(get('quota-bedrock-body').innerHTML.includes('<th>'+t('th.status')+'</th>'),'AWS state heading must be translated in '+lang);
  }
  currentLang='en';
  let config={aws_bedrock:{api_key:'••••••••••••••••',billing:{enabled:false,profile:'',linked_account_id:''}},commandcode:{api_key:'••••••••••••••••'}};
  let saved;
  fetch=async (url, options)=>{
    assert.equal(url,'/api/proxy/config');
    if (options?.method === 'POST') {saved=JSON.parse(options.body);config={...config,aws_bedrock:{...config.aws_bedrock,billing:saved.aws_bedrock.billing}}}
    return {ok:true,json:async()=>config};
  };
  await loadProxyConfig();
  get('cfg-bedrock-billing-enabled').checked=true;
  get('cfg-bedrock-billing-profile').value='billing-readonly';
  get('cfg-bedrock-billing-account').value='123456789012';
  await saveProxyConfig();
  assert.equal(JSON.stringify(saved),'{"aws_bedrock":{"billing":{"enabled":true,"profile":"billing-readonly","linked_account_id":"123456789012"}}}','billing save must preserve all inference settings');
  assert.equal(readFieldValue(CONFIG_FIELDS.find(field=>field[0] === 'aws_bedrock.billing.enabled')),undefined);
}
` + platformBehaviorRunScript

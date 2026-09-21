package gui

import "testing"

func TestCommandCodeAccountPresentation(t *testing.T) {
	runPlatformBehavior(t, commandcodeAccountPresentationScript)
}

func TestCommandCodeLedgerCostPresentation(t *testing.T) {
	runPlatformBehavior(t, platformBehaviorDOMScript+`
async function checks() {
  for (const lang of ['en', 'zh']) {
    currentLang = lang;
    for (const [ledger, expected] of [
      [{requests:1, known_requests:1, unknown_cost_requests:1, cost_usd:0}, '—'],
      [{requests:2, known_requests:2, unknown_cost_requests:1, cost_usd:1.25}, t('analytics.knownSubtotal').replace('{value}', '$1.25')],
      [{requests:1, known_requests:1, unknown_cost_requests:0, cost_usd:0}, '$0.00'],
    ]) {
      QuotaModule.provider = 'commandcode';
      QuotaModule.view = {provider:'commandcode', status:'available', accounts:[{
        key_hint:'masked-CC01', ledger, commandcode:{usage:{totalCount:3,totalCost:8.5}},
      }]};
      QuotaModule.render();
      const html = document.getElementById('quota-commandcode-accounts').innerHTML;
      assert.ok(html.includes('commandcode-ledger'), 'the API account.ledger must render');
      assert.ok(html.includes('<dt>' + t('commandcode.ledgerCost') + '</dt><dd>' + expected + '</dd>'), 'local cost must disclose price coverage');
      assert.ok(html.includes('<dt>' + t('commandcode.officialCost') + '</dt><dd>$8.50</dd>'), 'official cost stays separate');
      if (ledger.unknown_cost_requests) assert.ok(html.includes(t('analytics.unknownCosts').replace('{n}', '1')));
    }
  }
}
`+platformBehaviorRunScript)
}

const commandcodeAccountPresentationScript = platformBehaviorDOMScript + `
async function checks() {
  const get = id => document.getElementById(id);
  QuotaModule.provider = 'commandcode';
  const credits = {credits:{freeCredits:0,monthlyCredits:70,purchasedCredits:2},windowLimits:{limited:true,exceeded:null,fiveHour:{used:0,cap:14,exceeded:false,resetAt:0},weekly:{used:10,cap:35,exceeded:false,resetAt:1910000000000}}};
  const subscription = {planId:'individual-goat',status:'active',currentPeriodStart:'2026-09-10T07:35:57.000Z',currentPeriodEnd:'2026-10-10T07:35:57.000Z',cancelAtPeriodEnd:false};
  const usage = {totalCount:2,completedCount:1,failedCount:1,totalTokensIn:100,totalTokensOut:20,totalTokens:120,totalCredits:5,periodBasis:'billing-period'};
  const account = {key_hint:'masked-CC01',commandcode:{credits,subscription,usage}};
  for (const lang of ['en','zh']) {
    currentLang = lang;
    QuotaModule.view = {provider:'commandcode',status:'partial',source:'official_alpha_api',currency:'USD',accounts:[account,{key_hint:'masked-CC02',error:'HTTP 401 <denied>'}]};
    QuotaModule.render();
    let html = get('quota-commandcode-accounts').innerHTML;
    assert.equal(get('quota-commandcode').hidden,false);
    assert.equal(get('quota-go').hidden,true);
    assert.equal(get('quota-unavailable').hidden,true);
    assert.equal(get('quota-source').textContent,t('quota.source.official_alpha_api'));
    assert.ok(html.includes('masked-CC01') && html.includes('masked-CC02'),'account scopes must remain separate');
    assert.ok(html.includes('HTTP 401 &lt;denied&gt;') && !html.includes('<denied>'),'one key error stays visible and escaped');
    assert.ok(html.includes('individual-goat') && html.includes('2026-09-10') && html.includes('2026-10-10'),'official plan and full UTC period are shown');
    assert.ok(html.includes('$70.00') && html.includes('$2.00') && html.includes('$0.00'));
    assert.ok(!html.includes('$72.00'),'balances must not be aggregated');
    assert.ok(html.includes('10.00') && html.includes('35.00') && html.includes('28.6%'),'window percentage comes from its own used and cap');
    assert.ok(!html.includes('1970') && !html.includes('data-deadline="0"'),'an unset reset timestamp is unknown, not epoch');
    assert.ok(html.includes('data-deadline="1910000000000"'),'reset timestamps are milliseconds');
    assert.ok(html.includes(t('commandcode.monthlyLimitUnknown')),'the absent monthly grant must be disclosed');
    assert.ok(html.includes('120') && html.includes(t('commandcode.usageCredits')),'official token and credit totals remain visible');

    QuotaModule.view.accounts = [{key_hint:'masked-CC01',commandcode:{subscription,usage,credits_error:'synthetic credits HTTP 503'}}];
    QuotaModule.render();
    html = get('quota-commandcode-accounts').innerHTML;
    assert.ok(html.includes('synthetic credits HTTP 503') && html.includes('individual-goat') && html.includes('120'),'one block failure must preserve the other blocks');
    assert.ok(!html.includes('$70.00'),'failed refresh must not retain the previous credit balance');
    QuotaModule.view.accounts = [{key_hint:'masked-CC01',commandcode:{credits,usage}}];
    QuotaModule.render();
    assert.ok(get('quota-commandcode-accounts').innerHTML.includes(t('commandcode.noSubscription')),'no returned subscription is explicit');
    QuotaModule.view = {provider:'commandcode',status:'not_configured',source:'official_alpha_api',accounts:[]};
    QuotaModule.render();
    assert.ok(get('quota-commandcode-accounts').innerHTML.includes(t('quota.noProviderKey').replace('{provider}','CommandCode')));
    assert.ok(!get('quota-commandcode-accounts').innerHTML.includes('$70.00'));
    for(const key of ['commandcode.credits','commandcode.monthlyLimitUnknown','commandcode.noSubscription','commandcode.usageCredits','commandcode.billingPeriod']) assert.notEqual(t(key),key);
  }
}
` + platformBehaviorRunScript

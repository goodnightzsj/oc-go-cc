package gui

import "testing"

func TestSmallAmountsRemainNonzero(t *testing.T) {
	runPlatformBehavior(t, platformBehaviorDOMScript+`
async function checks() {
  for (const lang of ['en','zh']) {
    currentLang = lang;
    for (const amount of [0.00002,0.000000412,0.00000000001,-0.00002]) {
      for (const format of [fmtCost,fmtAvgCost]) {
        const value = Number(format(amount).replace(/[$,]/g,''));
        assert.ok(value !== 0 && Number.isFinite(value),'nonzero amount must remain visible: ' + amount);
        assert.equal(Math.sign(value),Math.sign(amount));
      }
    }
    assert.equal(fmtAvgCost(null),'—');
    assert.equal(fmtAvgCost(0),'$0.00');
    assert.equal(fmtAvgCost(0.1234),'$0.12','ordinary averages stay compact');
    const totals = {requests:1,errors:0,total_tokens:10,credits:0.00002,credit_samples:1,avg_tokens_per_second:123.456};
    const html = QuotaModule.renderWorkBuddyAccount({workbuddy:{
      overview:{accounts:[{realm:'cn',credits:0.00002,credits_total:0.00003}]},
      usage:{totals,by_model:[{key:'synthetic',...totals}],series:[]},
    }});
    assert.ok(html.includes('<td>0.00002</td><td>0.00003</td>'),'pool credits use the usage precision');
    assert.ok(html.includes('<td>0.00002</td><td>1</td>'),'model credits keep the same precision');
    assert.ok(!html.includes('$'),'credits are not dollars');
    assert.ok(html.includes('<dd>123.5 tok/s</dd>'),'upstream tok/s uses the shared numeric precision');
  }
}
`+platformBehaviorRunScript)
}

func TestQuickTestNumbersUseSharedFormats(t *testing.T) {
	runPlatformBehavior(t, platformBehaviorDOMScript+`
async function checks() {
  TestModule.init();
  globalThis.performance = {now:()=>0};
  globalThis.fetch = async url => {
    assert.equal(url,'/api/test/send');
    performance.now = ()=>2500;
    return {ok:true,text:async()=>JSON.stringify({content:[{text:'synthetic'}],usage:{input_tokens:1000000,output_tokens:20000}})};
  };
  TestModule.testModelSelect.value = 'synthetic';
  TestModule.testPrompt.value = 'synthetic';
  await TestModule.sendTest();
  assert.equal(TestModule.testLatency.textContent,'2.5 s');
  assert.equal(TestModule.testTokens.textContent,'1,000,000 in / 20,000 out');
  assert.equal(TestModule.testSendBtn.disabled,false);
}
`+platformBehaviorRunScript)
}

func TestModelNumbersUseTheSameFormat(t *testing.T) {
	runPlatformBehavior(t, platformBehaviorDOMScript+`
async function checks() {
  const get = id => document.getElementById(id);
  const model = {id:'synthetic',model_id:'synthetic',provider:'workbuddy',context_window:1000000,max_output:32768,input_per_m:0.123456,output_per_m:0.00002};
  for (const lang of ['en','zh']) {
    currentLang = lang;
    CatalogModule.models = [model];
    get('catalog-platform-filter').value = '';
    get('catalog-search').value = '';
    CatalogModule.render();
    const catalog = get('catalog-model-list').innerHTML;
    const picker = FallbackModule.pickerRow(model);
    for (const text of ['1M ctx','$0.123456 / $0.00002']) {
      assert.ok(catalog.includes(text),'catalog: ' + text);
      assert.ok(picker.includes(text),'picker: ' + text);
    }
    assert.ok(catalog.includes('33K out'),'output context keeps the compact role');
  }
  const data = {currency:'USD',total_cost:1.25,linked_account_id:'synthetic',start_date:'2026-10-01',end_date:'2026-10-02',services:[],daily:[{date:'2026-10-01',cost:1.25}]};
  QuotaModule.renderBedrockBilling({bedrock_billing:data});
  assert.ok(get('quota-bedrock-body').innerHTML.includes('<dd>$1.25</dd>'),'USD looks the same across providers');
  QuotaModule.renderBedrockBilling({bedrock_billing:{...data,currency:'EUR'}});
  assert.ok(!get('quota-bedrock-body').innerHTML.includes('$1.25'),'other currencies are not relabelled USD');
}
`+platformBehaviorRunScript)
}

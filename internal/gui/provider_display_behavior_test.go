package gui

import "testing"

func TestProviderDisplayOrder(t *testing.T) {
	runPlatformBehavior(t, providerDisplayBehaviorScript)
}

const providerDisplayBehaviorScript = platformBehaviorDOMScript + `
async function checks() {
  // want: the platforms the dashboard offers. all: every platform, because
  // rows recorded under a hidden one still sort at its registry position.
  const want = visiblePlatforms;
  const all = allPlatforms;
  const equal = (actual, expected, message) => assert.equal(JSON.stringify(actual),JSON.stringify(expected),message);
  equal(Object.keys(PROVIDERS),want,'shared display registry');
  for (const id of ['overview-provider','provider-filter','perf-provider','analytics-provider','quota-provider','settings-provider-jump']) {
    const select = page.match(new RegExp('<select[^>]*id="' + id + '"[^>]*>([\\s\\S]*?)</select>'));
    assert.ok(select,'missing provider selector: ' + id);
    equal([...select[1].matchAll(/value="([^"]*)"/g)].map(match=>match[1]).filter(Boolean),want,id);
  }
  equal([...page.matchAll(/data-settings-provider="([^"]+)"/g)].map(match=>match[1]),want,'settings DOM and keyboard order');
  equal(['z-custom','aws_bedrock','commandcode','opencode_go','a-custom'].sort(compareProviderDisplay),
    ['opencode_go','commandcode','aws_bedrock','a-custom','z-custom'],'aliases and unknown providers');

  const shuffled = ['z-custom','openrouter','aws-bedrock','commandcode','opencode-zen','opencode-go','a-custom'];
  const rows = shuffled.map((provider,index)=>({provider,model:'model-'+index,requests:index+1,input_tokens:index+1,output_tokens:1,cache_read_tokens:0,cache_creation_tokens:0,cost_usd:index+1}));
  const original = JSON.stringify(rows);
  const providerOrder = id => [...document.getElementById(id).innerHTML.matchAll(/data-provider="([^"]+)"/g)].map(match=>match[1]);
  for (const metric of ['requests','total_tokens','cost_usd']) {
    for (const id of ['overview-provider-distribution','provider-distribution']) {
      AnalyticsModule.renderDistribution(id,rows,metric,'provider');
      equal(providerOrder(id),[...all,'a-custom','z-custom'],'provider distribution order: '+id+' '+metric);
    }
  }
  AnalyticsModule.renderDistribution('model-distribution-test',rows,'requests','model');
  equal(providerOrder('model-distribution-test'),[...shuffled].reverse(),'model ranking must still follow the selected metric');
  assert.equal(JSON.stringify(rows),original,'rendering must not mutate source data');
  historyBreakdownMetric = 'cost';
  renderHistorySummary({providers:rows.map(row=>({...row,name:row.provider,provider:undefined,tokens:row.input_tokens}))});
  const labels = id => [...document.getElementById(id).innerHTML.matchAll(/class="compact-breakdown-name">([^<]*)</g)].map(match=>match[1]);
  equal(labels('history-provider-breakdown'),[...all,'a-custom','z-custom'],'history platform breakdown');

  FallbackModule.currentScenario = 'default';
  FallbackModule.chains = {default:[{provider:'openrouter',model_id:'keep-first'},{provider:'opencode-go',model_id:'keep-second'}]};
  FallbackModule.availableModels = shuffled.map(provider=>({provider,model_id:'model',wire_format:'openai'}));
  const chainBefore = JSON.stringify(FallbackModule.chains);
  const modelsBefore = JSON.stringify(FallbackModule.availableModels);
  FallbackModule.populateAddSelect();
  equal([...document.getElementById('fallback-add-model').innerHTML.matchAll(/value="([^"]+)"/g)].map(match=>match[1].split('/')[0]),
    [...all,'a-custom','z-custom'],'add-model picker');
  assert.equal(JSON.stringify(FallbackModule.chains),chainBefore,'configured fallback order must not change');
  assert.equal(JSON.stringify(FallbackModule.availableModels),modelsBefore,'picker sorting must not mutate model source');

  const targets = Object.fromEntries(shuffled.map(provider=>['alias-'+provider,{provider,model_id:'model'}]));
  fetch = async()=>({ok:true,json:async()=>({model_overrides:targets})});
  TestModule.testModelSelect = document.getElementById('test-model');
  await TestModule.populateModels();
  equal(TestModule.testModelSelect.children.map(option=>option.value),[...all,'a-custom','z-custom'].map(provider=>'alias-'+provider),'test model picker');
  assert.equal(JSON.stringify(FallbackModule.chains),chainBefore);
}
` + platformBehaviorRunScript

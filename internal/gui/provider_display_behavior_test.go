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

  // Every platform in the registry, shuffled: the expectation below is
  // built from site.All, so a platform missing here would be asserted
  // present while no row exists to render it.
  const shuffled = ['z-custom','openrouter','aws-bedrock','commandcode','cline-pass','workbuddy','opencode-zen','opencode-go','a-custom'];
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
  // The model dimension is capped (MODEL_DISTRIBUTION_LIMIT), so the ranking is
  // asserted on the rows that are shown: they must be the top N by the selected
  // metric, in order. A wrong sort still fails here; only the tail is dropped.
  equal(providerOrder('model-distribution-test'),[...shuffled].reverse().slice(0,MODEL_DISTRIBUTION_LIMIT),'model ranking must still follow the selected metric');
  assert.ok(MODEL_DISTRIBUTION_LIMIT < shuffled.length,
    'this fixture must exceed the cap, or it would not exercise truncation');
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
  // The picker is a popover now, so the drawn list is the source of truth: it
  // groups by platform and every row carries its platform-qualified key.
  FallbackModule.togglePicker(true);
  FallbackModule.renderPickerList('');
  const pickerPlatforms = [...document.getElementById('fallback-add-list').innerHTML.matchAll(/data-key="([^"]+)"/g)]
    .map(match=>match[1].split('/')[0]);
  equal([...new Set(pickerPlatforms)], [...all,'a-custom','z-custom'], 'add-model picker');
  assert.equal(JSON.stringify(FallbackModule.chains),chainBefore,'configured fallback order must not change');
  assert.equal(JSON.stringify(FallbackModule.availableModels),modelsBefore,'picker sorting must not mutate model source');

  // The quick-test picker lists what the proxy can actually send to, not the
  // routing aliases: on an instance whose scenarios all point at one platform,
  // the alias list offered nothing else to test. It groups by platform, so the
  // platform selector and the model list must agree with the served roster.
  const served = [
    ...shuffled.filter(p=>all.includes(p)).map((provider,index)=>({id:'m'+index, provider, context_window:128000, input_per_m:1, output_per_m:2})),
    {id:'a-custom/m', provider:'a-custom'},
    {id:'z-custom/m', provider:'z-custom'},
  ];
  fetch = async raw => String(raw).includes('/api/catalog/models')
    ? {ok:true,json:async()=>({models:served, scope:'', total:served.length})}
    : {ok:true,json:async()=>({model_overrides:targets})};
  TestModule.testModelSelect = document.getElementById('test-model');
  TestModule.testProviderSelect = document.getElementById('test-provider');
  await TestModule.populateModels();
  equal(TestModule.testProviderSelect.children.map(option=>option.value),['',...all,'a-custom','z-custom'],'test platform picker');
  // Ordered by platform display order, then by id - the same rule the picker
  // states, recomputed here rather than hard-coded so a fixture change does not
  // silently weaken the assertion.
  const expectedModelOrder = [...served]
    .sort((a,b)=>compareProviderDisplay(a.provider,b.provider)||a.id.localeCompare(b.id))
    .map(m=>m.id);
  equal(TestModule.testModelSelect.children.map(option=>option.value),['',...expectedModelOrder],'test model picker offers every served model');
  assert.ok(TestModule.testModelSelect.children.length > 2,
    'the picker must not collapse to a single platform: this fixture serves several');

  // Narrowing to one platform re-filters the model list to that platform alone.
  TestModule.testProviderSelect.value = 'cline-pass';
  TestModule.renderModels();
  const clineOnly = TestModule.testModelSelect.children.map(option=>option.value).filter(Boolean);
  assert.ok(clineOnly.length > 0, 'the selected platform must keep its own models');
  for (const id of clineOnly) {
    const model = served.find(m=>m.id===id);
    assert.equal(model.provider,'cline-pass','a platform filter must not leak another platform\'s model: '+id);
  }
  assert.equal(JSON.stringify(FallbackModule.chains),chainBefore);
}
` + platformBehaviorRunScript

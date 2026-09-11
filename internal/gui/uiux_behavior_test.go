package gui

import "testing"

func TestUIUXEditingBehavior(t *testing.T) {
	runPlatformBehavior(t, uiuxEditingBehaviorScript)
}

const uiuxEditingBehaviorScript = platformBehaviorDOMScript + `
async function checks() {
  const get = id => document.getElementById(id);
  const first = {provider:'opencode-go',model_id:'shared',wire_format:'openai'};
  const second = {provider:'commandcode',model_id:'shared',wire_format:'anthropic',max_tokens:4096};
  FallbackModule.chains = {default:[first,second]};
  FallbackModule.originalChains = JSON.parse(JSON.stringify(FallbackModule.chains));
  FallbackModule.currentScenario = 'default';
  FallbackModule.availableModels = [first,second];
  FallbackModule.renderChain();
  assert.ok(get('fallback-chain').innerHTML.includes('chain-move'),'keyboard-equivalent reorder buttons must exist');
  assert.ok(get('fallback-chain').innerHTML.includes('aria-label="Move shared (CommandCode) up"'),'reorder label must distinguish same-name models by provider');
  assert.ok(get('fallback-chain').innerHTML.includes('aria-label="Remove shared (OpenCode Go)"'),'remove label must identify provider');
  assert.ok(!get('fallback-chain').innerHTML.includes('role="option"'),'interactive rows must not use a listbox option role');
  FallbackModule.moveModel(1,0);
  assert.equal(FallbackModule.chains.default[0],second,'reordering must retain the complete model object');
  assert.equal(FallbackModule.chains.default[0].wire_format,'anthropic');
  assert.equal(FallbackModule.chains.default[0].max_tokens,4096);
  FallbackModule.moveModel(0,-1);
  FallbackModule.moveModel(NaN,1);
  assert.equal(FallbackModule.chains.default.length,2,'invalid drops must not change the chain');
  assert.equal(FallbackModule.chains.default[0],second);

  const mask = '••••••••••••••••';
  const config = {host:'127.0.0.1',port:3456,commandcode:{api_key:mask,timeout_ms:1000},models:{default:first}};
  const saved = [];
  fetch = async (url,options) => {
    assert.equal(url,'/api/proxy/config');
    if(options?.method === 'POST') saved.push(JSON.parse(options.body));
    return {ok:true,json:async()=>config};
  };
  await loadProxyConfig();
  updateConfigChangeCount();
  assert.equal(get('config-change-count').textContent,t('setting.noChanges'));
  get('cfg-commandcode-timeout').value = '2000';
  updateConfigChangeCount();
  assert.equal(get('config-change-count').textContent,t('setting.changeCount').replace('{n}','1'));
  await FallbackModule.save();
  assert.equal(get('cfg-commandcode-timeout').value,'2000','saving the chain must not erase a pending settings edit');
  assert.deepEqual(Object.keys(saved[0]),['fallbacks'],'chain saving must remain scoped');
  assert.equal(get('fallback-status').textContent,t('fallback.saved'),'chain feedback must appear on its own page');
  await saveProxyConfig();
  assert.equal(JSON.stringify(saved[1]),'{"commandcode":{"timeout_ms":2000}}','only the edited platform field may be saved');
  get('cfg-commandcode-timeout').value = '-1';
  updateConfigChangeCount();
  assert.equal(get('config-change-count').textContent,t('setting.invalidChanges'),'invalid fields must not masquerade as no changes');

  for(const lang of ['en','zh']) {
    currentLang = lang;
    const cost = fmtAggregateCost({requests:2,unknown_cost_requests:1,cost_usd:0.25});
    assert.equal(cost,t('analytics.knownSubtotal').replace('{value}',fmtCost(0.25)));
    assert.ok(costCoverageNote({unknown_cost_requests:1}).includes('1'),'unknown records remain explicit beside a known subtotal');
    for(const key of ['setting.changeCount','setting.noChanges','setting.invalidChanges','quota.openSettings','perf.sampleHint','fallback.moveUp','fallback.moveDown','fallback.remove','commandcode.accountScope']) assert.notEqual(t(key),key);
  }
}
` + platformBehaviorRunScript

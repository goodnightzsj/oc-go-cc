package gui

import "testing"

func TestSettingsDialogCloseKeepsNewerPreview(t *testing.T) {
	runPlatformBehavior(t, platformBehaviorDOMScript+`
async function checks() {
  const get = id => document.getElementById(id);
  const trigger = get('btn-import-config');
  const apply = get('btn-import-apply');
  const footer = {removed:false, remove(){this.removed = true}};
  modal.querySelector = selector => selector === '.modal-footer' ? footer : null;
  modal.showModal = () => {modal.open = true};
  modal.close = () => {modal.open = false};
  trigger.focus();
  openHistoryModal('modal.importPreview');
  closeHistoryModal();
  openHistoryModal('modal.importPreview');
  apply.focus();
  // Deliver the previous native dialog's queued close event after reopening.
  await modal.emit('close');
  assert.equal(footer.removed,false,'a stale close must keep the new preview controls');
  assert.equal(document.activeElement,apply,'a stale close must not steal focus');
  closeHistoryModal();
  await modal.emit('close');
  assert.equal(footer.removed,true,'the current close must still clean up');
  assert.equal(document.activeElement,trigger,'the current close must restore focus');
}
`+platformBehaviorRunScript)
}

func TestSettingsErrorsDraftsAndChannels(t *testing.T) {
	runPlatformBehavior(t, platformBehaviorDOMScript+`
async function checks() {
  currentLang = 'zh';
  const get = id => document.getElementById(id);
  let stored = {cline_pass:{channel_pin:'deepseek',channel_pin_enabled:false},port:3456};
  const ok = value => ({ok:true,json:async()=>JSON.parse(JSON.stringify(value))});
  const failure = (status, error) => ({ok:false,status,json:async()=>({error})});
  let postCount = 0, mode = 'ok', resolvePost;
  fetch = async (url, init) => {
    if (url === '/api/sites') return ok({sites:[],active:''});
    if (url === '/api/proxy/config' && !init) {
      if (mode === 'refresh-failed') throw new Error('private-sentinel');
      return ok(stored);
    }
    postCount++;
    if (mode === 'invalid') return failure(400,{code:'channel_slug',field:'cline_pass.channel_pin',message:'渠道名请使用小写字母。',saved:false});
    if (mode === 'network') throw new Error('private-sentinel');
    if (mode === 'proxy-error') return {ok:false,status:502,json:async()=>{throw new Error('private-sentinel')}};
    if (mode === 'delayed') await new Promise(resolve => {resolvePost = resolve});
    const patch = JSON.parse(init.body);
    if (patch.cline_pass) Object.assign(stored.cline_pass,patch.cline_pass);
    return ok({});
  };
  await loadProxyConfig();
  const target = get('cfg-cline-pass-channel-pin');
  target.setAttribute('aria-describedby','original-help');
  target.value = 'BAD'; mode = 'invalid';
  await saveProxyConfig();
  assert.equal(target.value,'BAD','failed save must retain the draft');
  assert.equal(target.getAttribute('aria-invalid'),'true');
  assert.equal(document.activeElement,target,'invalid field must receive focus');
  assert.match(target.getAttribute('aria-describedby'),/original-help.*-error/);
  assert.equal(target.after.textContent,'渠道名请使用小写字母。');
  assert.equal(get('save-status').getAttribute('role'),'alert');
  assert.equal(get('btn-save-cfg').disabled,false);
  for (const failureMode of ['network','proxy-error']) {
    mode = failureMode;
    await saveProxyConfig();
    assert.equal(get('save-status').textContent,t('save.unknown'));
    assert.equal(target.value,'BAD');
    assert.equal(target.getAttribute('aria-invalid'),null);
    assert.equal(target.getAttribute('aria-describedby'),'original-help');
  }
  mode = 'refresh-failed'; target.value = 'fireworks';
  await saveProxyConfig();
  assert.equal(get('save-status').textContent,t('save.refreshFailed'));
  assert.equal(currentProxyConfig.cline_pass.channel_pin,'fireworks','acknowledged save advances only submitted baseline');
  mode = 'delayed'; target.value = 'novita';
  const pending = saveProxyConfig();
  await Promise.resolve();
  assert.equal(get('btn-save-cfg').disabled,true);
  const count = postCount;
  await saveProxyConfig();
  assert.equal(postCount,count,'duplicate save blocked');
  target.value = 'alibaba';
  resolvePost(); await pending;
  assert.equal(stored.cline_pass.channel_pin,'novita');
  assert.equal(target.value,'alibaba','in-flight edit must survive refresh');
  assert.equal(readFieldValue(CONFIG_FIELDS.find(f=>f[0]==='cline_pass.channel_pin')),'alibaba');
  mode = 'ok'; target.value = '   ';
  await saveProxyConfig();
  assert.equal(stored.cline_pass.channel_pin,'deepseek');
  assert.equal(target.value,'deepseek');
  assert.equal(stored.cline_pass.channel_pin_enabled,false,'default does not enable pinning');

  let timers = new Map(), timerID = 0;
  setTimeout = callback => {timers.set(++timerID,callback);return timerID};
  clearTimeout = id => timers.delete(id);
  showSaveStatus('已保存','success');
  showSaveStatus('请修改渠道','error');
  for(const callback of timers.values()) callback();
  assert.equal(get('save-status').textContent,'请修改渠道','old success timer must not erase new error');

  target.value = 'manual-channel';
  fetch = async()=>ok({capture_enabled:false,channels:{'deepseek/test':{actual:['deepseek'],available:['novita']}}});
  await loadChannelCatalog();
  assert.equal(target.value,'manual-channel','candidate refresh never edits selection');
  assert.equal(get('cline-pass-channel-source').textContent,t('clinepass.channelBuiltin'));
  const choices = get('cline-pass-channel-options').options;
  assert.equal(choices.length,3);
  assert.match(choices[1].textContent,/deepseek.*曾实际返回.*deepseek\/test/);
  assert.match(choices[2].textContent,/novita.*未实测/);
  fetch = async()=>{throw new Error('private-sentinel')};
  await loadChannelCatalog();
  assert.equal(get('cline-pass-channel-options').options,choices,'failed candidate refresh keeps valid options');
  assert.equal(get('cline-pass-channel-source').textContent,t('clinepass.channelFailed'));
  assert.equal(target.value,'manual-channel');

  const toggle = get('toggle-autostart'); toggle.checked = true;
  fetch = async()=>failure(500,{code:'autostart_failed',field:'autostart',message:'系统未能保存开机自启动设置。',saved:false});
  await toggleAutostart(toggle);
  assert.equal(toggle.checked,false);
  assert.equal(toggle.disabled,false);
  assert.equal(toggle.getAttribute('aria-invalid'),'true');
  assert.match(get('save-status').textContent,/自启动/);

  FallbackModule.originalChains = {default:[]};
  FallbackModule.chains = {default:[{model_id:'first'}]};
  fetch = async()=>{await new Promise(resolve => {resolvePost = resolve});return ok({})};
  const fallbackSave = FallbackModule.save();
  FallbackModule.chains.default.push({model_id:'second'});
  resolvePost(); await fallbackSave;
  assert.equal(FallbackModule.originalChains.default.length,1,'save baseline must not include later chain edits');
  assert.equal(FallbackModule.chains.default.length,2);
  assert.ok(get('fallback-status').textContent.includes(t('fallback.unsaved')),'later edits must remain visibly unsaved');
  fetch = async()=>failure(400,{code:'unknown_provider',message:'请选择已有平台。',saved:false});
  await FallbackModule.save();
  assert.equal(get('fallback-status').textContent,'请选择已有平台。');
  assert.equal(FallbackModule.chains.default.length,2);
  await handleConfigImport({text:async()=>'{private-sentinel'});
  assert.equal(get('save-status').textContent,t('save.invalidJSON'));
  assert.ok(!get('save-status').textContent.includes('private-sentinel'));

  modal.querySelector = selector => selector === '.modal-content' ? {insertAdjacentHTML(){}} : null;
  modal.showModal = () => {modal.open = true};
  modal.close = () => {modal.open = false};
  fetch = async()=>ok({config:{port:3456}});
  await handleConfigImport({text:async()=>'{"port":3456}'});
  fetch = async()=>{await new Promise(resolve=>{resolvePost=resolve});return failure(400,{code:'invalid_config',message:'请检查设置项。',saved:false})};
  const applyImport = get('btn-import-apply').onclick();
  closeHistoryModal();
  const lookup = document.getElementById;
  document.getElementById = id => id === 'import-status' ? null : lookup(id);
  resolvePost(); await applyImport;
  assert.equal(get('save-status').textContent,'请检查设置项。','closing the import dialog must not swallow a later save error');
  document.getElementById = lookup;
  fetch = async()=>ok({config:{port:3456}});
  await handleConfigImport({text:async()=>'{"port":3456}'});
  fetch = async()=>failure(500,{code:'redaction_failed',message:'无法安全展示。',saved:true});
  await get('btn-import-apply').onclick();
  assert.equal(get('save-status').textContent,t('save.refreshFailed'));
  assert.equal(get('btn-import-apply').disabled,true,'a confirmed save must not offer resubmission');
}
`+platformBehaviorRunScript)
}

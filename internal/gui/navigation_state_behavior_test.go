package gui

import "testing"

func TestActiveSiteSurvivesTabNavigation(t *testing.T) {
	runPlatformBehavior(t, navigationBehaviorDOMScript+`
async function checks() {
  await bootView();
  const get = id => document.getElementById(id);
  for (const tab of ['quota', 'history', 'performance', 'analytics', 'settings', 'fallback', 'overview']) {
    get('nav-' + tab).click();
    await settleView();
    assert.equal(activeTab, tab);
    for (const id of VIEW_CONTROLS.platform) {
      assert.equal(get(id).value, 'commandcode', tab + ' must retain the active platform: ' + id);
    }
    assert.equal(QuotaModule.provider, 'commandcode', 'the account loader must match its selector');
    assert.equal(parseViewHash(location.hash).params.has('platform'), false, 'an automatic default must not pin the URL');
  }
  assert.equal(get('quota-commandcode').hidden, false);
  assert.equal(get('quota-go').hidden, true);

  // A server-side switch changes the default for later navigation as well.
  syntheticSite.active = 'opencode-go';
  await applyActiveSite();
  get('nav-quota').click();
  await settleView();
  assert.equal(get('quota-provider').value, 'opencode-go');
  assert.equal(parseViewHash(location.hash).params.has('platform'), false);
}
`+platformBehaviorRunScript)
}

func TestPlatformChoicesSurviveNavigationAndPolling(t *testing.T) {
	runPlatformBehavior(t, navigationBehaviorDOMScript+`
async function checks() {
  await bootView();
  const get = id => document.getElementById(id);
  for (const id of VIEW_CONTROLS.platform) {
    get(id).value = 'opencode-go';
    await get(id).dispatchEvent(new Event('change', {bubbles:true}));
    await settleView();
    assert.equal(parseViewHash(location.hash).params.get('platform'), 'opencode-go', 'the changed control wins: ' + id);
    await applyActiveSite();
    get('nav-quota').click();
    await settleView();
    for (const sibling of VIEW_CONTROLS.platform) assert.equal(get(sibling).value, 'opencode-go', sibling);
    location.hash = '#overview';
    await settleView();
  }

  // Empty means an explicit all-platforms filter, not an absent choice. The
  // account page requires one platform and keeps the active account instead.
  get('overview-provider').value = '';
  await get('overview-provider').dispatchEvent(new Event('change', {bubbles:true}));
  await settleView();
  await applyActiveSite();
  get('nav-history').click();
  await settleView();
  assert.equal(parseViewHash(location.hash).params.has('platform'), true);
  assert.equal(parseViewHash(location.hash).params.get('platform'), '');
  for (const id of VIEW_CONTROLS.platform.filter(id => id !== 'quota-provider')) assert.equal(get(id).value, '', id);
  assert.equal(get('quota-provider').value, 'commandcode');

  // Deep links and back/forward must update the pin, not keep the boot choice.
  location.hash = '#history?platform=opencode-go';
  await settleView();
  await applyActiveSite();
  assert.equal(get('provider-filter').value, 'opencode-go');
  location.hash = '#history';
  await settleView();
  assert.equal(get('provider-filter').value, 'commandcode');

  viewProviderHistory('opencode-go');
  await settleView();
  assert.equal(activeTab, 'history');
  assert.equal(get('provider-filter').value, 'opencode-go');
  assert.equal(parseViewHash(location.hash).params.get('platform'), 'opencode-go');
}
`+platformBehaviorRunScript)
}

func TestActiveSiteIgnoresOlderPollsAndClearsDefaults(t *testing.T) {
	runPlatformBehavior(t, navigationBehaviorDOMScript+`
async function checks() {
  await bootView();
  const originalFetch = fetch;
  let release;
  fetch = raw => raw === '/api/sites' ? new Promise(resolve => {release = resolve}) : originalFetch(raw);
  const older = applyActiveSite();
  fetch = originalFetch;
  syntheticSite.active = 'opencode-go';
  await applyActiveSite();
  release({ok:true, json:async()=>({active:'commandcode'})});
  await older;
  for (const id of VIEW_CONTROLS.platform) assert.equal(document.getElementById(id).value, 'opencode-go', 'an older poll cannot undo a newer site: ' + id);

  syntheticSite.active = '';
  await applyActiveSite();
  document.getElementById('nav-quota').click();
  await settleView();
  for (const id of VIEW_CONTROLS.platform.filter(id => id !== 'quota-provider')) assert.equal(document.getElementById(id).value, '', 'removing the route scope restores the statistics default: ' + id);
  assert.equal(document.getElementById('quota-provider').value, 'opencode-go');
  assert.equal(parseViewHash(location.hash).params.has('platform'), false);
}
`+platformBehaviorRunScript)
}

func TestViewStateRestoresHistoryDefaults(t *testing.T) {
	runPlatformBehavior(t, platformBehaviorDOMScript+`
async function checks() {
  captureViewDefaults();
  historyPage = 3;
  currentSort = {field:'duration_ms', dir:'asc'};
  const moved = applyViewState(new URLSearchParams());
  assert.equal(historyPage, 1, 'back to an unqualified history URL restores page one');
  assert.equal(currentSort.field, 'start_time');
  assert.equal(currentSort.dir, 'desc');
  assert.equal(moved, true, 'restoring defaults must refresh history');
}
`+platformBehaviorRunScript)
}

func TestSettingsPreserveUnselectableActiveSite(t *testing.T) {
	runPlatformBehavior(t, navigationBehaviorDOMScript+`
async function checks() {
  await bootView();
  const get = id => document.getElementById(id);
  const stored = {active_site:'aws-bedrock', logging:{level:'info'}};
  let patch;
  const originalFetch = fetch;
  fetch = async (raw, init) => {
    const url = new URL(raw, 'http://synthetic.invalid');
    if (url.pathname === '/api/proxy/config') {
      if (init?.method === 'POST') {
        patch = JSON.parse(init.body);
        Object.assign(stored, patch);
      }
      return {ok:true, json:async()=>({...stored})};
    }
    return originalFetch(raw, init);
  };

  for (const active of ['aws-bedrock', 'commandcode']) {
    stored.active_site = active;
    stored.logging.level = 'info';
    syntheticSite.active = active;
    syntheticSite.sites = visiblePlatforms.map(id => ({id, selectable:false}));
    await loadProxyConfig();
    await applySelectableSites();
    assert.equal(get('cfg-active-site').value, active, 'the form must display the stored platform even when it cannot be selected');
    const option = get('cfg-active-site').options.find(option => option.value === active);
    assert.equal(option.disabled, true);
    get('overview-provider').value = 'opencode-go';
    await get('overview-provider').dispatchEvent(new Event('change', {bubbles:true}));
    await settleView();
    get('cfg-log-level').value = 'debug';
    await saveProxyConfig();
    assert.equal(Object.hasOwn(patch, 'active_site'), false, 'saving another setting must not clear the platform');
    assert.equal(stored.active_site, active);
    assert.equal(get('overview-provider').value, 'opencode-go', 'saving another setting must preserve an explicit view filter');
  }
}
`+platformBehaviorRunScript)
}

func TestViewStateRefreshesRestoredFiltersAndDates(t *testing.T) {
	runPlatformBehavior(t, navigationBehaviorDOMScript+`
async function checks() {
  await bootView();
  const get = id => document.getElementById(id);
  get('nav-history').click();
  await settleView();
  const before = historyLoadSeq;
  location.hash = '#history?q=cache&model=shared&scenario=fast';
  await settleView();
  assert.ok(historyLoadSeq > before, 'restoring text filters invalidates and reloads history');
  assert.ok(requests.some(url => url.pathname === '/api/history' && url.searchParams.get('search') === 'cache'));

  get('nav-analytics').click();
  await settleView();
  requests.length = 0;
  location.hash = '#analytics?afrom=2026-09-01&ato=2026-09-02';
  await settleView();
  assert.equal(get('analytics-date-label').textContent, '2026-09-01 → 2026-09-02 (UTC)');
  assert.ok(requests.some(url => url.pathname === '/api/analytics/summary' && url.searchParams.get('from') === '2026-09-01T00:00:00.000Z'), 'restored dates must reach the data loader');

  get('analytics-start-display').value = '2026-09-03';
  get('analytics-end-display').value = '2026-09-04';
  AnalyticsModule.applyDateRange();
  await settleView();
  assert.equal(parseViewHash(location.hash).params.get('afrom'), '2026-09-03');
  assert.equal(parseViewHash(location.hash).params.get('ato'), '2026-09-04');
}
`+platformBehaviorRunScript)
}

// Extend the existing isolated DOM only with the browser behavior needed by
// navigation: markup defaults, synchronous change bubbling, and asynchronous
// hashchange. No production configuration, account, or upstream is accessed.
const navigationBehaviorDOMScript = platformBehaviorDOMScript + `
const documentListeners = {};
const windowListeners = {};
context.document.addEventListener = (type, handler) => (documentListeners[type] ||= []).push(handler);
context.document.dispatchEvent = event => {
  for (const handler of documentListeners[event.type] || []) handler(event);
};
context.window.addEventListener = (type, handler) => (windowListeners[type] ||= []).push(handler);
context.queueMicrotask = queueMicrotask;
context.getComputedStyle = () => ({colorScheme:'dark'});
context.window.matchMedia = () => ({addEventListener(){}});

for (const match of input.page.matchAll(/<select\b[^>]*id="([^"]+)"[^>]*>([\s\S]*?)<\/select>/g)) {
  const el = node(match[1]);
  el.id = match[1];
  el.tagName = 'SELECT';
  el.options = [...match[2].matchAll(/<option\b([^>]*)value="([^"]*)"([^>]*)>/g)].map(option => ({
    value:option[2], disabled:/\bdisabled\b/.test(option[1] + option[3]), selected:/\bselected\b/.test(option[1] + option[3]),
  }));
  el.value = (el.options.find(option => option.selected) || el.options[0])?.value || '';
  Object.defineProperty(el, 'value', {
    get(){return this._value},
    set(value){this._value = this.options.some(option => option.value === String(value)) ? String(value) : ''},
  });
  el.appendChild = option => {
    el.options.push(option);
    option.remove = () => { el.options.splice(el.options.indexOf(option), 1); };
  };
  el.querySelectorAll = selector => selector === 'option[data-current-site]' ? el.options.filter(option => option.dataset?.currentSite) : [];
}
context.document.createElement = tag => ({tagName:tag.toUpperCase(), dataset:{}});
for (const match of input.page.matchAll(/<input\b[^>]*id="([^"]+)"[^>]*>/g)) {
  const el = node(match[1]);
  el.id = match[1];
  el.tagName = 'INPUT';
  el.defaultValue = match[0].match(/\bvalue="([^"]*)"/)?.[1] || '';
  el.value = el.defaultValue;
  if (/\btype="hidden"/.test(match[0])) {
    Object.defineProperty(el, 'defaultValue', {get(){return this.value}});
  }
}
const tabs = [...input.page.matchAll(/<button\b[^>]*data-tab="([^"]+)"[^>]*id="([^"]+)"/g)].map(match => {
  const el = node(match[2]);
  el.id = match[2];
  el.dataset.tab = match[1];
  el.click = () => el.emit('click');
  return el;
});
context.document.querySelectorAll = selector => selector === '.tab' ? tabs : [];
context.document.querySelector = selector => tabs.find(tab => selector === '[data-tab="' + tab.dataset.tab + '"]') || null;
for (const el of nodes.values()) {
  el.dispatchEvent = event => {
    event.target = el;
    const pending = Promise.all((el.listeners[event.type] || []).map(handler => handler(event)));
    if (event.bubbles) context.document.dispatchEvent(event);
    return pending;
  };
}
let hashValue = '';
Object.defineProperty(location, 'hash', {
  get(){return hashValue},
  set(value){
    const next = value ? '#' + String(value).replace(/^#/, '') : '';
    if (next === hashValue) return;
    hashValue = next;
    setImmediate(() => { for (const handler of windowListeners.hashchange || []) handler(new Event('hashchange')); });
  },
});
history.replaceState = (_state, _title, url) => { hashValue = String(url).slice(String(url).indexOf('#')); };
context.settleView = async () => { for (let i = 0; i < 3; i++) await new Promise(setImmediate); };
context.bootView = async () => {
  context.document.dispatchEvent(new Event('DOMContentLoaded'));
  await context.settleView();
};
context.syntheticSite = {active:'commandcode', sites:input.visible.map(id => ({id, selectable:true}))};
context.requests = [];
context.fetch = async raw => {
  const url = new URL(raw, 'http://synthetic.invalid');
  context.requests.push(url);
  const provider = url.searchParams.get('provider') || '';
  const summary = {total_requests:0, known_requests:0, unknown_cost_requests:0};
  let data;
  if (url.pathname === '/api/sites') data = {...context.syntheticSite};
  else if (url.pathname === '/api/quota') data = {provider, status:'available', accounts:[]};
  else if (url.pathname === '/api/analytics/summary') data = {provider, summary, models:[], providers:[]};
  else if (url.pathname === '/api/analytics/tokens/trend') data = {provider, trend:[]};
  else if (url.pathname === '/api/perf/aggregate') data = {provider, total_requests:0};
  else if (url.pathname === '/api/perf/models') data = [];
  else if (url.pathname === '/api/history') data = {items:[], total:0};
  else if (url.pathname === '/api/history/summary') data = summary;
  else return {ok:false, status:503, text:async()=> 'synthetic unavailable response'};
  return {ok:true, json:async()=>data};
};
`

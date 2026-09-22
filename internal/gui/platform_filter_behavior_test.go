package gui

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"testing"

	"github.com/routatic/proxy/internal/site"
)

// Execute the shipped filters with a synthetic DOM and network, isolated from
// real settings, accounts and storage. The same model name occurs on all providers.
func TestPlatformFilterBehavior(t *testing.T) {
	runPlatformBehavior(t, platformFilterBehaviorScript)
}

func TestHistoryCSVExportKeepsInitialQuery(t *testing.T) {
	runPlatformBehavior(t, historyCSVExportScopeScript)
}

func runPlatformBehavior(t *testing.T, script string) {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is unavailable; platform filter behavior was not checked")
	}
	app, err := assets.ReadFile("assets/app.js")
	if err != nil {
		t.Fatal(err)
	}
	page, err := assets.ReadFile("assets/index.html")
	if err != nil {
		t.Fatal(err)
	}
	// The expected platform lists come from the registry, so a hidden or renamed
	// platform cannot leave these scripts asserting the wrong thing.
	visible := make([]string, 0)
	for _, d := range site.Visible() {
		visible = append(visible, d.ID)
	}
	all := make([]string, 0)
	for _, d := range site.All() {
		all = append(all, d.ID)
	}
	payload, err := json.Marshal(map[string]any{"app": string(app), "page": string(page), "visible": visible, "all": all})
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(node, "-e", script)
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir(), "TZ=America/Los_Angeles"}
	cmd.Stdin = bytes.NewReader(payload)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("platform filter behavior: %v\n%s", err, output)
	}
}

const platformBehaviorDOMScript = `
const assert = require('node:assert/strict');
// The router reads and writes the browser's URL globals, so the shim provides
// them. They are declared here rather than inline in the context object so the
// history stub closes over the same object the vm sees.
const location = {hash: ''};
const history = {replaceState(_state, _title, url) {
  const at = String(url).indexOf('#');
  location.hash = at < 0 ? '' : String(url).slice(at);
}};
// The router dispatches change events at controls it restores from the URL.
const Event = class { constructor(type, init) { this.type = type; Object.assign(this, init || {}); } };
// A class list that actually records state. The earlier stub answered contains()
// with a constant false, which made every assertion about a class vacuous - a
// test could not tell a toggled class from a missing one.
function classListOf() {
  const set = new Set();
  return {
    add(...names){names.forEach(n => set.add(n))},
    remove(...names){names.forEach(n => set.delete(n))},
    toggle(name, force){const on = force === undefined ? !set.has(name) : !!force; on ? set.add(name) : set.delete(name); return on},
    contains(name){return set.has(name)},
    _set: set,
  };
}
const vm = require('node:vm');
const input = JSON.parse(require('node:fs').readFileSync(0, 'utf8'));
const nodes = new Map();
function node(id) {
  if (!nodes.has(id)) nodes.set(id, {
    _value: '', get value(){return this._value}, set value(value){this._value = String(value)},
    id, innerHTML: '', textContent: '', hidden: true, dataset: {}, style: {}, listeners: {}, children: [], options: [],
    classList: classListOf(),
    addEventListener(type, handler){(this.listeners[type] ||= []).push(handler)},
    emit(type){return Promise.all((this.listeners[type] || []).map(handler => handler({target:this})))},
    dispatchEvent(event){return Promise.resolve(this.emit(event.type))},
    setAttribute(){}, getAttribute(){return null}, addEventListener(type,handler){(this.listeners[type] ||= []).push(handler)},
    querySelectorAll(){return []}, querySelector(){return null}, focus(){},
    appendChild(child){this.children.push(child)},
    // CustomSelect wraps the select it enhances, so the node needs somewhere
    // to be inserted.
    parentNode: {insertBefore(){}},
  });
  return nodes.get(id);
}
const context = vm.createContext({
  assert, page:input.page, URL, URLSearchParams,
  // checks() is stringified into the vm, so the registry lists have to be
  // context globals rather than outer-scope values.
  visiblePlatforms:input.visible, allPlatforms:input.all,
  // createElement returns a real-enough element for CustomSelect: it builds the
  // replacement list out of created nodes, so a stub that returns {} would make
  // every assertion about that list vacuous.
  document: {getElementById:node,querySelectorAll(){return []},querySelector(){return null},addEventListener(){},documentElement:{dataset:{},lang:''},
    createElement(tag){
      const el = {
        tagName: tag, children: [], attributes: {}, style: {}, className: '', dataset: {},
        _text: '', get textContent(){return this._text}, set textContent(v){this._text = String(v)},
        title: '', hidden: false, disabled: false, type: '', tabIndex: 0,
        setAttribute(k,v){this.attributes[k] = String(v)}, getAttribute(k){return this.attributes[k]},
        append(...kids){this.children.push(...kids)}, appendChild(kid){this.children.push(kid); return kid},
        insertBefore(n){this.children.push(n); return n}, replaceChildren(...kids){this.children = kids},
        addEventListener(){}, removeEventListener(){}, focus(){},
        querySelectorAll(){return []}, querySelector(){return null},
        classList: classListOf(),
      };
      return el;
    }},
  MutationObserver: class { observe(){} disconnect(){} },
  window: {addEventListener(){}},
  // A working store, not just getItem: the theme control writes, deletes and
  // reads back, and a stub that only answers getItem made its cycle untestable
  // here rather than failing loudly.
  localStorage:(() => { const store = new Map(); return {
    getItem: k => (store.has(k) ? store.get(k) : null),
    setItem: (k, v) => store.set(k, String(v)),
    removeItem: k => store.delete(k),
  }; })(),
  location, history, Event,
  setTimeout(){},clearTimeout(){},setInterval(){},queueMicrotask(){},
  fetch:async()=>({ok:false,status:503,text:async()=> 'synthetic unavailable response'}),
  console:{error(){}},
});
`

const platformFilterBehaviorScript = platformBehaviorDOMScript + `
async function checks() {
  const get = id => document.getElementById(id);
  const providers = allPlatforms;
  for (const id of ['overview-provider','perf-provider','analytics-provider','provider-filter']) {
    const select = page.match(new RegExp('<select[^>]*id="' + id + '"[^>]*>([\\s\\S]*?)</select>'));
    assert.ok(select, 'missing platform filter: ' + id);
    const choices = [...select[1].matchAll(/value="([^"]*)"/g)].map(match => match[1]);
    assert.equal(JSON.stringify(choices), JSON.stringify(['', ...visiblePlatforms]), 'every offered platform plus reset: ' + id);
    assert.ok(get(id).listeners.change?.length, 'filter must actually refresh: ' + id);
  }
  for (const id of ['overview-error','perf-error','analytics-error']) {
    assert.ok(page.includes('id="' + id + '" hidden role="alert"'), 'errors must be announced: ' + id);
  }

  let calls = [];
  let empty = false;
  const summary = provider => ({
    total_requests:empty ? 0 : provider ? providers.indexOf(provider) + 1 : 15,
    known_requests:empty ? 0 : 1, success_rate:empty ? 0 : 1,
    input_tokens:empty ? 0 : 10,output_tokens:empty ? 0 : 4,
    cache_read_tokens:empty ? 0 : 2,cache_creation_tokens:empty ? 0 : 1,
    est_cost_usd:empty ? 0 : 0.1,unknown_cost_requests:0,
  });
  const reply = raw => {
    const url = new URL(raw, 'http://synthetic.invalid');
    const provider = url.searchParams.get('provider') || '';
    const selected = provider ? [provider] : providers;
    const s = summary(provider);
    let data;
    if (url.pathname === '/api/metrics') data = {proxy_running:true,requests_received:90001,model_counts:{}};
    else if (url.pathname === '/api/perf/models') data = empty ? null : selected.map(provider => ({provider,model:'shared',count:1,success:1,failed:0,avg_ms:4,p50_ms:4,p90_ms:4,p99_ms:4}));
    else if (url.pathname === '/api/perf/aggregate') data = {provider,total_requests:empty ? 0 : 1,avg_latency_ms:empty ? 0 : 4};
    else if (url.pathname === '/api/analytics/summary') data = {provider,summary:s,today:s,retained:s,last_minute:s,models:empty ? null : selected.map(provider => ({...s,requests:1,model:'shared',provider})),providers:empty ? null : selected.map(provider => ({...s,requests:1,provider}))};
    else if (url.pathname === '/api/analytics/tokens/trend') data = {provider,trend:empty ? null : [{...s,requests:s.total_requests,date:(url.searchParams.get('from') || new Date().toISOString()).slice(0,10)}]};
    else if (url.pathname === '/api/history') data = {items:empty ? [] : [{id:provider || 'all',provider,model:'shared',details_known:true,success:true}],total:empty ? 0 : 1};
    else if (url.pathname === '/api/history/summary') data = {...s,total_tokens:17,success_rows:1};
    // Price-table provenance is instance state, not window state, so it is
    // unscoped by provider like the endpoint itself.
    else if (url.pathname === '/api/prices') data = {tables:{'opencode-go':{rules:53,seed_rules:53,live:true,refreshed_at:new Date().toISOString()}}};
    else throw new Error('unexpected synthetic URL: ' + raw);
    return {ok:true,json:async()=>data};
  };
  const normalFetch = async raw => {calls.push(new URL(raw,'http://synthetic.invalid')); return reply(raw)};
  fetch = normalFetch;
  const assertScope = (endpoint, provider) => {
    const requests = calls.filter(url => url.pathname === endpoint);
    assert.ok(requests.length, 'missing request to ' + endpoint);
    for (const url of requests) assert.equal(url.searchParams.get('provider'), provider || null, endpoint);
  };
  get('analytics-start').value = '2026-11-01';
  get('analytics-end').value = '2026-11-01';
  AnalyticsModule.granularity = 'hour';
  PerfModule.timeRange = '24h';
  PerfModule.sortField = 'avg_ms';
  PerfModule.sortDir = 'asc';
  for (const provider of [...providers, '']) {
    calls = [];
    get('overview-provider').value = provider;
    await refreshMetrics();
    for (const path of ['/api/analytics/summary','/api/analytics/tokens/trend','/api/perf/aggregate']) assertScope(path, provider);
    assertScope('/api/metrics', '');
    assert.equal(get('m-total').textContent, String(provider ? providers.indexOf(provider) + 1 : 15), 'global process count must not overwrite selected usage');
    assert.equal(get('m-throughput').textContent, String(provider ? providers.indexOf(provider) + 1 : 15) + ' RPM');

    calls = [];
    get('analytics-provider').value = provider;
    await get('analytics-provider').emit('change');
    for (const path of ['/api/analytics/summary','/api/analytics/tokens/trend']) assertScope(path,provider);
    for (const url of calls) {
      // /api/prices describes the instance's price tables, not a time window or
      // a platform, so it carries neither. Every other call in this load must
      // be scoped to the requested range.
      if (url.pathname === '/api/prices') {
        assert.equal(url.searchParams.get('from'), null, 'price provenance is not window-scoped');
        assert.equal(url.searchParams.get('provider'), null, 'price provenance is not platform-scoped');
        continue;
      }
      assert.equal(url.searchParams.get('from'),'2026-11-01T00:00:00.000Z');
      assert.equal(url.searchParams.get('to'),'2026-11-02T00:00:00.000Z');
      assert.equal(url.searchParams.get('granularity'),'hour');
    }
    assert.equal(get('kpi-requests').textContent,String(provider ? providers.indexOf(provider) + 1 : 15));
    assert.equal(get('kpi-tokens').textContent,'17');

    calls = [];
    get('perf-provider').value = provider;
    await get('perf-provider').emit('change');
    assertScope('/api/perf/models',provider);
    assert.equal(calls[0].searchParams.get('range'),'24h');
    assert.equal(PerfModule.sortField,'avg_ms');
    assert.equal(PerfModule.sortDir,'asc');
    assert.equal(PerfModule.data.length, provider ? 1 : providers.length);
  }
  overviewDays = 90;
  calls = [];
  await get('overview-provider').emit('change');
  assert.equal(calls.find(url=>url.pathname === '/api/perf/aggregate').searchParams.get('range'),'90d','90-day totals must not use all-time latency');

  const views = [
    {id:'overview-provider',run:()=>refreshOverviewUsage(),hasProvider:()=>get('overview-provider-distribution').innerHTML,kpi:()=>get('m-total').textContent,error:'overview-error'},
    {id:'analytics-provider',run:()=>AnalyticsModule.load(true),hasProvider:()=>get('provider-distribution').innerHTML,kpi:()=>get('kpi-requests').textContent,error:'analytics-error'},
    {id:'perf-provider',run:()=>PerfModule.refresh(),hasProvider:()=>get('perf-tbody').innerHTML,error:'perf-error'},
    {id:'provider-filter',run:()=>refreshHistory(),hasProvider:()=>get('history-tbody').innerHTML,kpi:()=>get('history-summary-requests').textContent,error:'history-error'},
  ];
  for (const view of views) {
    for (const oldFails of [false,true]) {
      const pending = [];
      fetch = raw => new URL(raw,'http://synthetic.invalid').searchParams.get('provider') === 'opencode-go'
        ? new Promise(resolve => pending.push({raw,resolve})) : Promise.resolve(reply(raw));
      get(view.id).value = 'opencode-go';
      const previous = view.run();
      get(view.id).value = 'commandcode';
      await view.run();
      for (const item of pending) item.resolve(oldFails ? {ok:false,status:503,text:async()=> 'old failure'} : reply(item.raw));
      await previous;
      assert.ok(view.hasProvider().includes('commandcode'), 'late responses changed current scope: ' + view.id);
      assert.ok(!view.hasProvider().includes('opencode-go'), 'previous scope leaked: ' + view.id);
      assert.notEqual(get(view.error).hidden,false,'a stale failure must not replace successful data');
    }
    const pending = [];
    fetch = raw => new Promise(resolve=>pending.push({raw,resolve}));
    get(view.id).value = 'openrouter';
    const current = view.run();
    assert.ok(!view.hasProvider().includes('commandcode'),'clear previous scope before new data arrives: ' + view.id);
    if (view.kpi) assert.equal(view.kpi(),'…');
    for (const item of pending) item.resolve({ok:false,status:503,text:async()=> 'synthetic unavailable storage'});
    await current;
    assert.equal(get(view.error).hidden,false,'failed view has no visible error: ' + view.id);
    assert.ok(get(view.error).textContent.includes('503'));
    if (view.kpi) assert.equal(view.kpi(),'—','failed requests must not look like zero usage');
  }

  fetch = async raw => raw === '/api/metrics' ? reply(raw) : {ok:false,status:503,text:async()=> 'only usage failed'};
  await refreshMetrics();
  assert.equal(CONN.fails,0,'a failed usage endpoint must not mark the service offline');
  assert.equal(get('status-text').textContent,t('status.running'));
  assert.equal(get('overview-error').hidden,false);

  empty = true;
  fetch = normalFetch;
  await refreshOverviewUsage();
  assert.equal(get('m-total').textContent,'0','a successfully queried empty ledger is known zero');
  assert.equal(get('m-cache-hit').textContent,'—');
  assert.equal(get('m-success').textContent,'—','no records must not claim a success rate');
  assert.ok(get('overview-request-trend').innerHTML.includes(t('analytics.noTrend')));
  await AnalyticsModule.load(true);
  assert.equal(get('kpi-requests').textContent,'0');
  assert.equal(get('kpi-input').textContent,'0','a known zero token count remains zero');
  assert.equal(get('kpi-tokens').textContent,'0','known zero totals must not look unavailable');
  assert.equal(get('kpi-cache-rate').textContent,'—');
  assert.ok(get('analytics-model-tbody').innerHTML.includes(t('analytics.noData')));
  await PerfModule.refresh();
  assert.ok(get('perf-tbody').innerHTML.includes(t('empty.noData')));
  assert.ok(!get('perf-tbody').innerHTML.includes('100.0%'));

  fetch = async () => ({ok:true,json:async()=>null});
  await refreshOverviewUsage();
  await AnalyticsModule.load(true);
  assert.equal(get('m-total').textContent,'—');
  assert.equal(get('kpi-requests').textContent,'—');
  assert.equal(get('overview-error').hidden,false);
  assert.equal(get('analytics-error').hidden,false);
  renderOverviewUsage({summary:{total_requests:1,known_requests:1,success_rate:null}},[],null);
  assert.equal(get('m-tokens').textContent,'—');
  assert.equal(get('m-success').textContent,'—');
  assert.equal(get('m-throughput').textContent,'—');
  AnalyticsModule.renderKPIs({summary:{input_tokens:null,cache_read_tokens:null}});
  assert.equal(get('kpi-input').textContent,'—');
  assert.equal(get('kpi-cache-read').textContent,'—');
  assert.equal(get('kpi-tokens').textContent,'—');
  PerfModule.data = [{provider:'commandcode',model:'shared',success:null,failed:1,count:0,avg_ms:0}];
  PerfModule.render();
  assert.ok(!get('perf-tbody').innerHTML.includes('0.0%'),'unknown success must not become a failure rate');

  empty = false;
  fetch = normalFetch;
  get('provider-filter').value = 'commandcode';
  await refreshHistory();
  get('provider-filter').value = 'aws-bedrock';
  await get('provider-filter').emit('change');
  assert.ok(!get('history-tbody').innerHTML.includes('commandcode'),'debounced history filters must clear the old provider immediately');
  await refreshHistory();
  assert.equal(allHistory[0].provider,'aws-bedrock');
  for (const provider of [...providers,'']) {
    get('provider-filter').value = provider;
    calls = [];
    await refreshHistory();
    assertScope('/api/history',provider);
    assertScope('/api/history/summary',provider);
  }
}
` + platformBehaviorRunScript

const platformBehaviorRunScript = `
(async()=>{
  vm.runInContext(input.app,context,{timeout:3000});
  await new Promise(setImmediate);
  await vm.runInContext('(' + checks.toString() + ')()',context,{timeout:3000});
})().catch(error=>{console.error(error);process.exitCode=1});
`

const historyCSVExportScopeScript = platformBehaviorDOMScript + `
context.Blob = require('node:buffer').Blob;
async function checks() {
  const get = id => document.getElementById(id);
  get('provider-filter').value = 'opencode-go';
  get('model-filter').value = 'shared-model';
  get('history-search').value = 'original search';
  get('status-filter').value = 'true';
  get('history-start').value = '2026-09-01';
  currentSort = {field:'start_time',dir:'desc'};
  const urls = [];
  let releaseFirst;
  let csvBlob;
  let revoked;
  let downloads = 0;
  URL.createObjectURL = blob => {csvBlob = blob; return 'blob:synthetic-csv'};
  URL.revokeObjectURL = url => {revoked = url};
  document.createElement = tag => {
    assert.equal(tag,'a');
    return {click(){downloads++}};
  };
  fetch = raw => {
    const url = new URL(raw,'http://synthetic.invalid');
    assert.equal(url.pathname,'/api/history');
    urls.push(url);
    const page = Number(url.searchParams.get('page'));
    const rows = Array.from({length:page === 1 ? 500 : 1},(_,index)=>({
      id:'synthetic-' + page + '-' + index,
      provider:url.searchParams.get('provider'),model:'shared-model',details_known:true,success:true,
    }));
    const response = {ok:true,json:async()=>({items:rows,total:501})};
    return page === 1 ? new Promise(resolve=>{releaseFirst=()=>resolve(response)}) : Promise.resolve(response);
  };
  const exporting = exportHistoryCSV();
  assert.equal(urls.length,1,'the second page must wait for the first response');
  assert.equal(get('history-export').disabled,true);
  assert.equal(urls[0].searchParams.get('provider'),'opencode-go');
  assert.equal(urls[0].searchParams.get('size'),'500');
  get('provider-filter').value = 'commandcode';
  get('model-filter').value = 'different-model';
  get('history-search').value = 'changed search';
  get('status-filter').value = 'false';
  get('history-start').value = '2026-09-02';
  currentSort = {field:'duration_ms',dir:'asc'};
  releaseFirst();
  await exporting;
  assert.equal(urls.length,2);
  assert.equal(urls[1].searchParams.get('page'),'2');
  assert.equal(urls[1].searchParams.get('provider'),'opencode-go','CSV must not mix platforms when filters change between pages');
  urls.forEach(url=>url.searchParams.delete('page'));
  assert.equal(urls[1].search,urls[0].search,'all filters and sort order must stay fixed for the export');
  assert.ok(csvBlob instanceof Blob);
  const csv = await csvBlob.text();
  assert.equal(csv.trim().split('\n').length,502,'the CSV must include its header and all 501 selected rows');
  assert.ok(csv.includes('opencode-go'));
  assert.ok(!csv.includes('commandcode'));
  assert.equal(downloads,1);
  assert.equal(revoked,'blob:synthetic-csv');
  assert.equal(get('history-export').disabled,false);
  assert.equal(get('provider-filter').value,'commandcode','export must not revert the user selection');
}
` + platformBehaviorRunScript

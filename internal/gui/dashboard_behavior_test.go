package gui

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"testing"

	"github.com/routatic/proxy/internal/site"
)

// Run the shipped script with synthetic DOM/network collaborators. No browser,
// platform credentials or third-party JavaScript dependency is required.
func TestDashboardPlatformDataBehavior(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is unavailable; dashboard JavaScript behavior was not checked")
	}
	app, err := assets.ReadFile("assets/app.js")
	if err != nil {
		t.Fatal(err)
	}
	page, err := assets.ReadFile("assets/index.html")
	if err != nil {
		t.Fatal(err)
	}
	visible := make([]string, 0)
	for _, d := range site.Visible() {
		visible = append(visible, d.ID)
	}
	payload, err := json.Marshal(map[string]any{"app": string(app), "page": string(page), "visible": visible})
	if err != nil {
		t.Fatal(err)
	}
	for _, zone := range []string{"Asia/Shanghai", "America/Los_Angeles"} {
		t.Run(zone, func(t *testing.T) {
			cmd := exec.Command(node, "-e", dashboardBehaviorScript)
			cmd.Env = append(os.Environ(), "TZ="+zone)
			cmd.Stdin = bytes.NewReader(payload)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("dashboard behavior: %v\n%s", err, output)
			}
		})
	}
}

const dashboardBehaviorScript = `
const assert = require('node:assert/strict');
const vm = require('node:vm');
const input = JSON.parse(require('node:fs').readFileSync(0, 'utf8'));
const nodes = new Map();
function node(id) {
  if (!nodes.has(id)) nodes.set(id, {
    _value: '', get value(){return this._value}, set value(v){this._value = String(v)},
    innerHTML: '', textContent: '', dataset: {}, style: {}, children: [], listeners: {},
    classList: {add(){},remove(){},toggle(){},contains(){return false}},
    addEventListener(type, handler){(this.listeners[type] ||= []).push(handler)}, setAttribute(){}, querySelectorAll(){return []},
    querySelector(selector){return selector === '.modal-content' ? node('modal-content') : null}, focus(){},
    appendChild(child){this.children.push(child)}, insertAdjacentHTML(_, html){this.innerHTML += html},
  });
  return nodes.get(id);
}
// The router reads and writes the browser's URL globals. Without them a hash
// write throws inside the history render path, where the refresh handler's own
// error handling turns it into what looks like a failed data load.
const location = {hash: ''};
const history = {replaceState(_state, _title, url) {
  const at = String(url).indexOf('#');
  location.hash = at < 0 ? '' : String(url).slice(at);
}};
const context = vm.createContext({
  assert, page: input.page, visiblePlatforms: input.visible, location, history,
  document: {
    getElementById: node, querySelectorAll(){return []}, querySelector(){return null},
    addEventListener(){}, documentElement: {}, createElement(){return {}},
  },
  window: {addEventListener(){}},
  localStorage: {getItem(){return null}},
  fetch: async () => ({ok: false, text: async () => 'synthetic unavailable response'}),
  setTimeout(){}, setInterval(){}, clearTimeout(){}, queueMicrotask(){},
  URLSearchParams, console: {error(){}},
});
vm.runInContext(input.app, context, {timeout: 3000});
vm.runInContext(` + "`" + `
(async () => {
  const ids = [...page.matchAll(/id="([^"]+)"/g)].map(match => match[1]);
  assert.equal(new Set(ids).size, ids.length, 'duplicate DOM identifiers');
  for (const [, id] of CONFIG_FIELDS) assert.ok(ids.includes(id), 'missing setting: ' + id);
  for (const path of ['base_url', 'anthropic_base_url', 'api_key', 'api_keys', 'timeout_ms', 'stream_timeout_ms', 'streaming_timeout_ms', 'zero_data_retention']) {
    assert.ok(CONFIG_FIELDS.some(field => field[0] === 'commandcode.' + path), 'missing CommandCode config field: ' + path);
  }
  assert.ok(!CONFIG_FIELDS.some(field => field[0] === 'commandcode.zdr'), 'obsolete CommandCode config field');
  // Every visible platform's settings block must be bound to the save path.
  // A block with markup but no CONFIG_FIELDS entry renders fine and silently
  // drops whatever is typed into it, so the platform can never be given a
  // credential and never becomes selectable.
  for (const descriptor of visiblePlatforms) {
    const key = descriptor.replace(/-/g, '_');
    assert.ok(CONFIG_FIELDS.some(field => field[0] === key + '.base_url'),
      'platform settings block is not bound to the save path: ' + descriptor);
  }
  for (const path of ['base_url', 'api_key', 'api_keys', 'timeout_ms', 'stream_timeout_ms', 'streaming_timeout_ms']) {
    assert.ok(CONFIG_FIELDS.some(field => field[0] === 'cline_pass.' + path), 'missing ClinePass config field: ' + path);
  }
  assert.ok(!CONFIG_FIELDS.some(field => field[0] === 'cline_pass.anthropic_base_url'), 'ClinePass has no Messages endpoint to configure');
  assert.ok(CONFIG_FIELDS.some(field => field[0] === 'opencode_go.responses_base_url'), 'missing OpenCode Go Responses URL');
  assert.ok(page.includes('id="quota-links"'), 'official platform links need a shared rendering target');
  assert.ok(page.includes('https://api.commandcode.ai/provider/v1/chat/completions'));
  assert.ok(page.includes('https://api.commandcode.ai/provider/v1/messages'));
  for (const provider of visiblePlatforms) {
    assert.ok(page.includes('value="' + provider + '"'), 'missing provider choice: ' + provider);
  }
  assert.ok(document.getElementById('provider-filter').listeners.change?.includes(scheduleHistoryRefresh), 'the themed platform picker must refresh history on change');
  assert.equal(fmtAggregateCost({requests: 2, unknown_cost_requests: 2, cost_usd: 0}), '—');
  assert.equal(fmtAggregateCost({requests: 2, unknown_cost_requests: 1, cost_usd: 0.25}), 'Known $0.25');
  assert.equal(fmtAggregateCost({requests: 1, unknown_cost_requests: 0, cost_usd: 0}), '$0.00');
  assert.ok(costCoverageNote({unknown_cost_requests: 3}).includes('3'));
  assert.equal(utcDateInputValue(new Date('2026-09-10T00:30:00+08:00')), '2026-09-09');

  // A real CommandCode row (req-1789357046-14: 12045 in / 26624 cached /
  // 199 out on deepseek-v4.1-flash, doubled in the peak window) is
  // 0.004012044. Six places is what the platform's own usage page reports, so
  // the two can be compared digit for digit; four places showed $0.0040 and
  // lost the part that reconciles.
  assert.equal(fmtCost(0.004012044), '$0.004012');
  // Trailing zeros are kept below a cent, so a column of rows lines up on the
  // same six places instead of drifting between five and six.
  assert.equal(fmtCost(0.001339782), '$0.001340');
  // The official account total comes back with the same precision and must
  // not be flattened to cents.
  assert.equal(fmtCost(6.1647660797), '$6.164766');
  // Two places remain the floor, and trailing zeros are not padded past it.
  assert.equal(fmtCost(12.3), '$12.30');
  assert.equal(fmtCost(60), '$60.00');
  // Half a dollar is money, not a sub-cent figure: two places, not six.
  assert.equal(fmtCost(0.5), '$0.50');
  assert.equal(fmtCost(0.02), '$0.02');
  // Still smaller than six places: a single-token turn must not read as free.
  assert.equal(fmtCost(0.000000412), '$0.000000412');
  assert.notEqual(fmtCost(0.0000004), '$0.000000');

  document.getElementById('analytics-start').value = '2026-11-01';
  document.getElementById('analytics-end').value = '2026-11-01';
  const params = AnalyticsModule.queryParams();
  assert.equal(params.get('from'), '2026-11-01T00:00:00.000Z');
  assert.equal(params.get('to'), '2026-11-02T00:00:00.000Z');
  AnalyticsModule.granularity = 'hour';
  const hours = AnalyticsModule.fillTrend([{date:'2026-11-01T23:00:00Z',requests:7}]);
  assert.equal(hours.length, 24, 'UTC hours must not repeat/skip with local DST');
  assert.equal(hours[23].requests, 7);
  AnalyticsModule.granularity = 'day';
  assert.equal(AnalyticsModule.fillTrend([{date:'2026-11-01',requests:4}])[0].requests, 4);
  for (const invalid of ['', '2026-99-01', '2026-02-30', '2026-01-00']) {
    document.getElementById('analytics-start-display').value = invalid;
    document.getElementById('analytics-end-display').value = '2026-11-01';
    assert.doesNotThrow(() => AnalyticsModule.applyDateRange(), 'invalid UTC dates must be rejected without throwing');
    assert.equal(document.getElementById('analytics-start').value, '2026-11-01', 'invalid input changed the active range');
  }

  const mask = '••••••••••••••••';
  currentProxyConfig = {commandcode: {api_keys: [mask], timeout_ms: 1000}};
  document.getElementById('cfg-commandcode-api-keys').value = mask;
  const keysField = CONFIG_FIELDS.find(field => field[0] === 'commandcode.api_keys');
  assert.equal(readFieldValue(keysField), undefined);
  document.getElementById('cfg-commandcode-api-keys').value = 'synthetic-a, synthetic-b';
  assert.equal(JSON.stringify(readFieldValue(keysField)), '["synthetic-a","synthetic-b"]');
  document.getElementById('cfg-commandcode-api-keys').value = '';
  assert.equal(JSON.stringify(readFieldValue(keysField)), '[]');
  document.getElementById('cfg-commandcode-timeout').value = '1.5';
  assert.throws(() => readFieldValue(CONFIG_FIELDS.find(field => field[0] === 'commandcode.timeout_ms')));
  document.getElementById('cfg-commandcode-streaming-timeout').value = '';
  assert.equal(readFieldValue(CONFIG_FIELDS.find(field => field[0] === 'commandcode.streaming_timeout_ms')), undefined, 'an untouched optional timeout must not be saved as zero');

  const go = {provider:'opencode-go',model_id:'shared',wire_format:'openai'};
  const cc = {provider:'commandcode',model_id:'shared',wire_format:'anthropic',max_tokens:4096};
  FallbackModule.availableModels = [go, cc];
  FallbackModule.chains = {default:[go]};
  FallbackModule.currentScenario = 'default';
  FallbackModule.populateAddSelect();
  assert.ok(document.getElementById('fallback-add-model').innerHTML.includes('commandcode/shared'));
  assert.ok(!document.getElementById('fallback-add-model').innerHTML.includes('value="opencode-go/shared"'));
  FallbackModule.renderChain = () => {};
  document.getElementById('fallback-add-model').value = 'commandcode/shared';
  FallbackModule.onAddSelectChange();
  assert.equal(FallbackModule.chains.default[1].wire_format, 'anthropic');
  assert.equal(FallbackModule.chains.default[1].max_tokens, 4096);
  assert.equal(configModelKey({provider:'opencode_go',model_id:'shared'}), configModelKey(go));
  // The peak multiplier is the backend's figure, read straight off the record:
  // the browser no longer re-derives it, because the schedule carries a Chinese
  // public holiday calendar it has no copy of. A row the backend billed off-peak
  // must stay off-peak here whatever its provider, model and time say.
  assert.equal(effectivePeakMultiplier({provider:'commandcode',model:'deepseek/deepseek-v4-pro',start_time:'2026-09-07T02:00:00Z',peak_multiplier:2}),2);
  assert.equal(effectivePeakMultiplier({provider:'opencode-go',model:'deepseek-v4-pro',start_time:'2026-09-07T02:00:00Z',peak_multiplier:2}),2);
  // 2026-05-01 is a Chinese public holiday inside the peak window: the backend
  // stores 1 for it, and the badge must not appear.
  assert.equal(effectivePeakMultiplier({provider:'opencode-go',model:'deepseek-v4-pro',start_time:'2026-05-01T02:00:00Z',peak_multiplier:1}),1);
  assert.equal(effectivePeakMultiplier({provider:'opencode-go',model:'deepseek-v4-pro',start_time:'2026-09-07T02:00:00Z'}),1,'an absent multiplier is off-peak, not a re-derivation');
  assert.equal(billingWindowLabel({provider:'opencode-go',model:'deepseek-v4-pro',start_time:'2026-09-07T02:00:00Z',peak_multiplier:2}), t('detail.peak') + ' \u00d72');
  assert.equal(billingWindowLabel({provider:'opencode-go',model:'deepseek-v4-pro',start_time:'2026-05-01T02:00:00Z',peak_multiplier:1}), t('detail.offPeak'));

  // Duration is graded by throughput, not by elapsed time: a long answer must
  // not read as slow just for being long. 200 output tokens over 2s is 100
  // tok/s (good) even though 20s elsewhere would be graded slow.
  const fast = {duration_ms:2000, output_tokens:200, success:true};
  const slowSameLength = {duration_ms:20000, output_tokens:200, success:true};
  assert.ok(gradeDuration(fast, '2.0 s').includes('timing-good'));
  assert.ok(gradeDuration(slowSameLength, '20.0 s').includes('timing-slow'));
  // Below 100 output tokens the ratio is noise, so it grades on seconds: 4
  // tokens in 2s is 2 tok/s and would be "slow" by rate, but the sample cannot
  // support that claim.
  assert.ok(gradeDuration({duration_ms:2000, output_tokens:4}, '2.0 s').includes('timing-good'), 'a short completion must grade on time, not on a meaningless rate');
  assert.equal(requestThroughput({duration_ms:0, output_tokens:500}), null);
  assert.equal(gradeDuration({duration_ms:0, output_tokens:0}, '—').includes('timing'), false, 'an unmeasured duration is not graded');

  // Relative time is used for recent stamps with the absolute value kept in the
  // title, and falls back to the date once "N days ago" stops being useful.
  const recent = fmtRelativeTime(new Date(Date.now() - 120000).toISOString());
  assert.ok(/minute|分钟/.test(recent), 'two minutes ago must read relatively, got ' + recent);
  assert.equal(fmtRelativeTime(''), '—');
  const ancient = fmtRelativeTime('2020-01-02T03:04:05Z');
  assert.ok(ancient.includes('2020'), 'a stamp years old must fall back to the date, got ' + ancient);

  // Tooltip series: sorted largest-first, a long tail collapsed into one Other
  // row that preserves the total, and a Total footer so the reader does not add
  // the rows up themselves.
  const many = Array.from({length: 12}, (_, i) => ({label: 'm' + i, n: (12 - i) * 100, value: String((12 - i) * 100)}));
  const shaped = tooltipSeries(many, {otherLabel: 'Other', totalLabel: 'Total', format: v => v.toLocaleString()});
  assert.ok(shaped.rows.length <= TOOLTIP_MAX_SERIES + 1, 'the tail must collapse, got ' + shaped.rows.length + ' rows');
  assert.equal(shaped.rows[0].value, '1,200', 'the largest series must come first, got ' + shaped.rows[0].value);
  const other = shaped.rows[shaped.rows.length - 1];
  assert.ok(other.label.startsWith('Other'), 'the collapsed row must be labelled, got ' + other.label);
  // The total must be the sum of everything, collapsed rows included - that is
  // the whole point of keeping the bucket.
  const rawTotal = many.reduce((a, m) => a + m.n, 0);
  assert.equal(shaped.total, rawTotal, 'the reported total must be the sum of every series');
  // The bucket's OWN value, which is what a reader adds to the visible rows to
  // reconcile - distinct from shaped.total above, which is computed from the
  // full sorted list and would stay correct even if the bucket were zeroed.
  const hiddenSum = many.slice(TOOLTIP_MAX_SERIES).reduce((a, m) => a + m.n, 0);
  assert.equal(other.n, hiddenSum, 'the collapsed bucket must hold the sum of what it hides');
  assert.ok(shaped.footer.some(f => f.label === 'Total' && f.value === rawTotal.toLocaleString()), 'the footer must carry the total');
  // A single series is its own total, so the footer would be noise.
  const one = tooltipSeries([{label: 'only', n: 5, value: '5'}], {totalLabel: 'Total'});
  assert.ok(!one.footer.some(f => f.label === 'Total'), 'one series needs no total row');
  // Nothing to collapse below the cap: order only.
  const few = tooltipSeries([{label:'a',n:1,value:'1'},{label:'b',n:9,value:'9'}], {totalLabel:'Total'});
  assert.equal(few.rows[0].label, 'b', 'rows must be sorted even when nothing collapses');

  // The Performance tab's health column: its dot must track the same
  // success-rate bands the cell already grades, so a 100% row and an 88% row
  // are not the same colour. Asserted against the rendered HTML rather than the
  // DOM, because the shim's querySelectorAll returns nothing - the same reason
  // the history assertions below read innerHTML.
  PerfModule.data = [
    {provider:'opencode-go', model:'perfect', count:10, success:10, failed:0, avg_ms:900, p50_ms:800, p90_ms:900, p99_ms:950},
    {provider:'opencode-go', model:'shaky',   count:10, success:8,  failed:2, avg_ms:900, p50_ms:800, p90_ms:900, p99_ms:950},
    {provider:'opencode-go', model:'broken',  count:10, success:3,  failed:7, avg_ms:900, p50_ms:800, p90_ms:900, p99_ms:950},
  ];
  PerfModule.sortField = 'count';
  PerfModule.render();
  const perfHTML = document.getElementById('perf-tbody').innerHTML;
  // The model name is followed by <br> and the platform, so match the cell
  // rather than the bare name. Plain string search rather than a regex: this
  // block is part of a large embedded script and a regex literal here survives
  // an editing round-trip badly.
  const dotFor = model => {
    const at = perfHTML.indexOf('>' + model + '<br>');
    if (at < 0) return null;
    const marker = perfHTML.indexOf('success-dot success-', at);
    if (marker < 0) return null;
    const tail = perfHTML.slice(marker + 'success-dot success-'.length);
    const end = tail.search(/[^a-z]/);
    return end < 0 ? tail : tail.slice(0, end);
  };
  assert.equal(dotFor('perfect'), 'good', '100% must read good');
  assert.equal(dotFor('shaky'), 'warn', '80% must read warn');
  assert.equal(dotFor('broken'), 'bad', '30% must read bad');
  // Every row carries the extra cell; a header without one misaligns the table.
  assert.equal((perfHTML.match(/<td/g) || []).length, 24, 'three rows of eight cells');

  const unknownRecord = {id:'unknown', provider:'commandcode', model:'shared', details_known:false, success:false, streaming:false, duration_ms:0, attempt:0};
  allHistory = [unknownRecord];
  renderHistory();
  assert.ok(document.getElementById('history-tbody').innerHTML.includes('badge-unknown'));
  showHistoryDetail(unknownRecord);
  assert.ok(document.getElementById('modal-body').innerHTML.includes('detail-status unknown'));
  assert.ok(HISTORY_CSV_COLUMNS.some(column => column[0] === 'details_known'), 'CSV needs its observation flag');
  for (const field of ['success', 'streaming', 'duration_ms', 'attempt']) {
    assert.equal(escapeCSV(HISTORY_CSV_COLUMNS.find(column => column[0] === field)[1](unknownRecord)), '', 'CSV fabricates unknown ' + field);
  }
  assert.equal(HISTORY_CSV_COLUMNS.find(column => column[0] === 'success')[1]({...unknownRecord, details_known:true}), false, 'a known failed request remains false');
  document.getElementById('provider-filter').value = 'commandcode';
  document.getElementById('cost-source-filter').value = 'estimated';
  assert.equal(historyQueryParams().get('provider'), 'commandcode');
  assert.equal(historyQueryParams().get('cost_source'), 'estimated');

  PerfModule.data = [{provider:'commandcode',model:'shared',count:1,success:2,failed:0,avg_ms:4}];
  PerfModule.render();
  assert.ok(document.getElementById('perf-tbody').innerHTML.includes('100.0%'));
  assert.ok(document.getElementById('perf-tbody').innerHTML.includes('commandcode'));

  AnalyticsModule.renderModelTable([{provider:'commandcode',model:'shared',requests:1,unknown_cost_requests:1,est_cost_usd:0}]);
  assert.ok(document.getElementById('analytics-model-tbody').innerHTML.includes('commandcode'));
  assert.ok(!document.getElementById('analytics-model-tbody').innerHTML.includes('$0.00'));
  QuotaModule.renderModelLimits({model_limits:{models:[{model:'sample',allowance_usd:30}]}});
  assert.ok(!document.getElementById('quota-model-limits').innerHTML.includes('$0.00'));
  assert.ok(!document.getElementById('quota-model-limits').innerHTML.includes('0.0%'));
  const unknownQuotaWindow = {status:'ok',has_percent:false,used_percent:0,resets_at:'2026-11-01T12:00:00Z'};
  QuotaModule.provider = 'opencode-go';
  for (const usedDollars of [undefined, 5]) {
    QuotaModule.view = {accounts:[{report:{rolling_5h:{...unknownQuotaWindow,used_dollars:usedDollars}}}]};
    QuotaModule.render();
    const quotaHTML = document.getElementById('quota-accounts').innerHTML;
    assert.ok(quotaHTML.includes('<strong>—</strong>'), 'an unknown percentage must have an unknown gauge');
    assert.ok(!quotaHTML.includes('class="quota-unit"'), 'missing usage must not be shown as 100% remaining');
    assert.ok(quotaHTML.includes('data-deadline="' + Date.parse(unknownQuotaWindow.resets_at) + '"'), 'an unknown percentage must retain its reset time');
    if (usedDollars !== undefined) assert.ok(quotaHTML.includes(fmtCost(usedDollars)), 'known usage dollars must remain visible');
    assert.equal(document.getElementById('quota-bottleneck').textContent, '--', 'unknown windows must not become the tightest known window');
    assert.equal(document.getElementById('quota-remaining').textContent, '--');
    assert.equal(document.getElementById('quota-next-reset').dataset.deadline, String(Date.parse(unknownQuotaWindow.resets_at)));
  }
  for (const usedPercent of [0, 25]) {
    QuotaModule.view = {accounts:[{report:{
      rolling_5h:unknownQuotaWindow,
      weekly:{has_percent:true,used_percent:usedPercent,used_dollars:30*usedPercent/100,limit_dollars:30},
    }}]};
    QuotaModule.render();
    assert.equal(document.getElementById('quota-bottleneck').textContent, t('quota.weekly'), 'only known percentages may select the tightest window, including known zero usage');
    assert.equal(document.getElementById('quota-remaining').textContent, fmtCost(30*(1-usedPercent/100)));
    assert.equal(document.getElementById('quota-next-reset').dataset.deadline, String(Date.parse(unknownQuotaWindow.resets_at)), 'mixed windows must retain an unknown window reset');
  }
  QuotaModule.provider = 'commandcode';
  const commandcodeQuota = {provider:'commandcode',status:'not_configured',source:'official_alpha_api',accounts:[],links:[{kind:'usage',url:'https://commandcode.ai/usage'},{kind:'billing',url:'https://commandcode.ai/billing'},{kind:'keys',url:'https://commandcode.ai/settings/keys'}]};
  QuotaModule.view = commandcodeQuota;
  QuotaModule.render();
  assert.equal(document.getElementById('quota-go').hidden, true, 'CommandCode must not display Go plan data');
  assert.equal(document.getElementById('quota-unavailable').hidden, true);
  assert.equal(document.getElementById('quota-commandcode').hidden, false);
  assert.ok(document.getElementById('quota-commandcode-accounts').innerHTML.includes('CommandCode'));
  assert.equal(document.getElementById('quota-links').hidden, false);
  for (const path of ['usage', 'billing', 'settings/keys']) {
    assert.ok(document.getElementById('quota-links').innerHTML.includes('href="https://commandcode.ai/' + path + '"'), 'missing official CommandCode link: ' + path);
  }

  AnalyticsModule.renderDistribution('provider-distribution', [{provider:'commandcode',requests:1,unknown_cost_requests:0,cost_usd:2},{provider:'opencode-go',requests:2,unknown_cost_requests:0,cost_usd:1}], 'cost_usd', 'provider');
  const distributionColors = () => Object.fromEntries([...document.getElementById('provider-distribution').innerHTML.matchAll(/data-provider="([^"]+)"[^]*?--distribution-color:([^";]+)/g)].map(match => [match[1], match[2]]));
  const colors = distributionColors();
  AnalyticsModule.renderDistribution('provider-distribution', [{provider:'commandcode',requests:1,unknown_cost_requests:0,cost_usd:1},{provider:'opencode-go',requests:2,unknown_cost_requests:0,cost_usd:2}], 'cost_usd', 'provider');
  assert.deepEqual(Object.keys(colors), ['opencode-go', 'commandcode'], 'provider display order stays fixed when costs change');
  assert.deepEqual(colors, distributionColors(), 'provider colors must not change with costs');
  assert.notEqual(colors['opencode-go'], colors.commandcode, 'providers keep distinct colors');
  AnalyticsModule.renderDistribution('provider-distribution', [{provider:'commandcode',requests:1,cost_usd:0.10},{provider:'opencode-go',requests:1,cost_usd:0.20}], 'cost_usd', 'provider');
  const smallCostsHTML = document.getElementById('provider-distribution').innerHTML;
  assert.ok(smallCostsHTML.includes(' · 66.7%</small>') && smallCostsHTML.includes(' · 33.3%</small>'), 'fractional-dollar shares must use the actual total');
  assert.ok(smallCostsHTML.includes('width:100.0%') && smallCostsHTML.includes('width:50.0%'), 'fractional-dollar bars must use the actual maximum');
  AnalyticsModule.renderDistribution('provider-distribution', [{provider:'commandcode',requests:1,cost_usd:0}], 'cost_usd', 'provider');
  const zeroCostHTML = document.getElementById('provider-distribution').innerHTML;
  assert.ok(zeroCostHTML.includes('$0.00') && zeroCostHTML.includes(' · 0.0%</small>'), 'known zero remains free, not unknown');
  assert.ok(!zeroCostHTML.includes('NaN') && !zeroCostHTML.includes('Infinity'), 'zero totals must not divide by zero');
  historyBreakdownMetric = 'cost';
  renderCompactBreakdown('history-provider-breakdown', [{name:'commandcode',requests:1,cost_usd:0.10},{name:'opencode-go',requests:1,cost_usd:0.20}]);
  const historyCostsHTML = document.getElementById('history-provider-breakdown').innerHTML;
  assert.ok(historyCostsHTML.includes('width:66.7%') && historyCostsHTML.includes('width:33.3%'), 'history cost bars must use the actual fractional-dollar total');
  renderCompactBreakdown('history-provider-breakdown', [{name:'commandcode',requests:1,cost_usd:0}]);
  assert.ok(document.getElementById('history-provider-breakdown').innerHTML.includes('width:0.0%'), 'zero cost must not paint a positive bar');
  const manyModels = Array.from({length:13}, (_, index) => ({provider:'commandcode',model:'model-' + index,requests:1,cost_usd:1}));
  AnalyticsModule.renderDistribution('provider-distribution', manyModels, 'cost_usd', 'model');
  const manyModelsHTML = document.getElementById('provider-distribution').innerHTML;
  assert.equal((manyModelsHTML.match(/analytics-distribution-row/g) || []).length, MODEL_DISTRIBUTION_LIMIT, 'keep the display limit');
  // Every shown row must still carry the share of the FULL total, not of the
  // capped remainder: 13 models at equal cost is 7.7% each, never 20%.
  assert.equal(manyModelsHTML.split(' · 7.7%</small>').length - 1, MODEL_DISTRIBUTION_LIMIT, 'top models must use the total across all models');
  renderCompactBreakdown('history-model-breakdown', manyModels.map(item => ({...item,name:item.model})));
  assert.equal(document.getElementById('history-model-breakdown').innerHTML.split('width:7.7%').length - 1, 5, 'history top models must use the full total');
  historyBreakdownMetric = 'tokens';
  manyModels.push({provider:'commandcode',model:'unknown',requests:1,unknown_cost_requests:1,cost_usd:0});
  AnalyticsModule.renderDistribution('provider-distribution', manyModels, 'cost_usd', 'model');
  assert.ok(!document.getElementById('provider-distribution').innerHTML.includes('%</small>'), 'unknown cost outside the display limit still makes the total incomplete');
  AnalyticsModule.renderDistribution('provider-distribution', [{provider:'commandcode',requests:1,unknown_cost_requests:1,cost_usd:0},{provider:'opencode-go',requests:2,unknown_cost_requests:0,cost_usd:2}], 'cost_usd', 'provider');
  assert.ok(!document.getElementById('provider-distribution').innerHTML.includes(' · 100.0%</small>'), 'a known subtotal is not the complete cross-platform cost share');

  const settings = {host:'127.0.0.1',port:3456,opencode_go:{responses_base_url:'https://synthetic-go.invalid/responses'},commandcode:{base_url:'https://synthetic.invalid/chat/completions',anthropic_base_url:'https://synthetic.invalid/messages',api_key:mask,api_keys:[mask],timeout_ms:1000,stream_timeout_ms:2000,streaming_timeout_ms:3000,zero_data_retention:false}};
  let savedPatch;
  fetch = async (_, options) => {
    if (options?.method === 'POST') savedPatch = JSON.parse(options.body);
    return {ok:true,json:async()=>settings};
  };
  await loadProxyConfig();
  assert.equal(document.getElementById('cfg-go-responses-url').value, settings.opencode_go.responses_base_url);
  assert.equal(document.getElementById('cfg-commandcode-anthropic-url').value, settings.commandcode.anthropic_base_url);
  assert.equal(document.getElementById('cfg-commandcode-streaming-timeout').value, '3000');
  document.getElementById('cfg-commandcode-zdr').checked = true;
  await saveProxyConfig();
  assert.equal(JSON.stringify(savedPatch), '{"commandcode":{"zero_data_retention":true}}', 'an isolated setting must not patch other platforms or persist masks');

  fetch = async () => ({ok:true,json:async()=>({model_overrides:{cc_alias:cc,go_alias:go}})});
  TestModule.testModelSelect = document.getElementById('test-model');
  await TestModule.populateModels();
  assert.ok(TestModule.testModelSelect.children.some(option => option.value === 'cc_alias' && option.textContent.includes('commandcode/shared')));
  assert.ok(TestModule.testModelSelect.children.some(option => option.value === 'go_alias' && option.textContent.includes('opencode-go/shared')));

  const quotaFetches = [];
  fetch = async url => {
    quotaFetches.push(url);
    return {ok:true,json:async()=>url.startsWith('/api/quota?') ? commandcodeQuota : {provider:'commandcode',summary:{total_requests:0,input_tokens:0,output_tokens:0,cache_read_tokens:0,cache_creation_tokens:0,unknown_cost_requests:0,est_cost_usd:0},models:[]}};
  };
  QuotaModule.provider = 'commandcode';
  await QuotaModule.load(true);
  assert.equal(quotaFetches.length, 2, 'account capability and local ledger use independent requests');
  assert.ok(quotaFetches.every(url => new URLSearchParams(url.split('?')[1]).get('provider') === 'commandcode'), 'CommandCode must never request Go quota or Go ledger data');
  assert.ok(quotaFetches.some(url => url.startsWith('/api/quota?') && new URLSearchParams(url.split('?')[1]).get('refresh') === '1'));
  assert.ok(quotaFetches.some(url => url.startsWith('/api/analytics/summary?')));
  assert.equal(document.getElementById('quota-go').hidden, true);
  assert.equal(document.getElementById('quota-local-requests').textContent, '0');

  const pending = [];
  const reply = (url, provider) => ({ok:true,json:async()=> url.startsWith('/api/history?') ? {items:[{id:provider,provider,model:'shared',success:true,details_known:true}],total:1} : {total_requests:1}});
  fetch = url => url.includes('provider=opencode-go') ? new Promise(resolve => pending.push({url,resolve})) : Promise.resolve(reply(url,'commandcode'));
  document.getElementById('provider-filter').value = 'opencode-go';
  const oldHistory = refreshHistory();
  document.getElementById('provider-filter').value = 'commandcode';
  await refreshHistory();
  for (const old of pending) old.resolve(reply(old.url,'opencode-go'));
  await oldHistory;
  assert.equal(allHistory[0].provider, 'commandcode', 'an old response must not overwrite a newer provider filter');

  fetch = async () => ({ok:false,status:503,text:async()=> 'synthetic storage unavailable'});
  await refreshHistory();
  assert.equal(document.getElementById('history-error').hidden, false);
  assert.ok(document.getElementById('history-error').textContent.includes('503'), 'failed log refresh must remain visible');
})()
` + "`" + `, context, {timeout: 3000}).catch(error => {console.error(error); process.exitCode = 1});
`

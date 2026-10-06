package gui

import "testing"

func TestDateRangesThroughToday(t *testing.T) {
	runPlatformBehavior(t, navigationBehaviorDOMScript+`
let clock = Date.parse('2026-11-01T23:30:00Z');
context.Date = class extends Date {
  constructor(...args) { super(...(args.length ? args : [clock])); }
  static now() { return clock; }
};
context.advanceClock = value => { clock = Date.parse(value); };
async function checks() {
  await bootView();
  const get = id => document.getElementById(id);
  for (const prefix of ['history', 'analytics']) {
    assert.ok(page.includes('id="' + prefix + '-through-today"'), 'both date pickers need the option');
    assert.ok(page.includes('id="' + prefix + '-date-error"'), 'invalid ranges need visible feedback');
  }

  // Persist intent, not the day the option was selected. The fixed start must
  // be present even when it equals the opening default.
  activeTab = 'analytics';
  AnalyticsModule.toggleDateRange();
  const start = get('analytics-start').value;
  get('analytics-through-today').checked = true;
  await get('analytics-through-today').emit('change');
  assert.equal(get('analytics-end-display').disabled, true);
  AnalyticsModule.applyDateRange();
  await settleView();
  assert.equal(get('analytics-end').value, 'today');
  assert.equal(parseViewHash(location.hash).params.get('ato'), 'today');
  assert.equal(parseViewHash(location.hash).params.get('afrom'), start);
  assert.equal(AnalyticsModule.queryParams().get('to'), '2026-11-02T00:00:00.000Z');
  const saved = location.hash;

  // UTC crosses midnight before Los Angeles; each picker retains its contract.
  window.HistoryDateRange.open();
  get('history-start-display').value = '2026-10-31';
  get('history-through-today').checked = true;
  await get('history-through-today').emit('change');
  window.HistoryDateRange.apply();
  await settleView();
  assert.equal(get('history-end').value, 'today');
  assert.equal(historyQueryParams().get('start'), '2026-10-31T07:00:00.000Z');
  assert.equal(historyQueryParams().get('end'), '2026-11-02T08:00:00.000Z', 'DST day must include 25 local hours');
  advanceClock('2026-11-02T00:30:00Z');
  assert.equal(historyQueryParams().get('end'), '2026-11-02T08:00:00.000Z');
  assert.equal(AnalyticsModule.queryParams().get('to'), '2026-11-03T00:00:00.000Z');
  requests.length = 0;
  activeTab = 'analytics';
  await refreshCurrentTab();
  assert.ok(requests.some(url => url.pathname === '/api/analytics/summary' && url.searchParams.get('to') === '2026-11-03T00:00:00.000Z'));
  assert.equal(AnalyticsModule.currentTrend.at(-1).date, '2026-11-02');
  assert.ok(get('analytics-date-label').textContent.includes('2026-11-02'));
  assert.ok(get('analytics-retained-range').textContent.includes('2026-11-02'));
  advanceClock('2026-11-02T08:30:00Z');
  assert.equal(historyQueryParams().get('end'), '2026-11-03T08:00:00.000Z');

  // Back/forward and reload resolve today again, without rolling the start.
  applyViewState(parseViewHash(saved).params);
  await settleView();
  assert.equal(get('analytics-start').value, start);
  assert.equal(get('analytics-end').value, 'today');
  assert.equal(get('analytics-through-today').checked, true);
  assert.equal(AnalyticsModule.queryParams().get('to'), '2026-11-03T00:00:00.000Z');

  // Cancelling a draft must not change either the active mode or saved range.
  AnalyticsModule.toggleDateRange();
  get('analytics-through-today').checked = false;
  await get('analytics-through-today').emit('change');
  get('analytics-end-display').value = '2026-10-30';
  AnalyticsModule.closeDateRange();
  AnalyticsModule.toggleDateRange();
  assert.equal(get('analytics-through-today').checked, true);
  assert.equal(get('analytics-end-display').value, '2026-11-02');
  get('analytics-start-display').value = '2026-10-28';
  const originalFetch = fetch;
  const pending = [];
  fetch = raw => new Promise(resolve => pending.push(() => resolve(originalFetch(raw))));
  const slowLoad = AnalyticsModule.load(true);
  const sent = pending.length;
  assert.ok(sent > 0);
  await refreshCurrentTab();
  assert.equal(pending.length, sent, 'polls must not invalidate a slower in-flight load');
  pending.forEach(resolve => resolve());
  await slowLoad;
  fetch = originalFetch;
  assert.equal(get('analytics-start-display').value, '2026-10-28', 'polling must preserve the open draft');
  // Simulate boot on another day with new default dates, then restore the URL.
  AnalyticsModule.initDateRange();
  captureViewDefaults();
  applyViewState(parseViewHash(saved).params);
  await settleView();
  assert.equal(get('analytics-start').value, start, 'reload must not move the chosen start');
  AnalyticsModule.presetDateRange(7);
  assert.equal(get('analytics-through-today').checked, false);
  assert.equal(get('analytics-end-display').disabled, false);
  AnalyticsModule.applyDateRange();
  await settleView();
  assert.equal(get('analytics-end').value, '2026-11-02');
  advanceClock('2026-11-03T08:30:00Z');
  assert.equal(AnalyticsModule.queryParams().get('to'), '2026-11-03T00:00:00.000Z', 'fixed dates do not advance');
  requests.length = 0;
  await refreshCurrentTab();
  assert.equal(requests.length, 0, 'fixed analytics preserves its manual refresh behavior');

  // Both paths must visibly reject malformed dates and reversed ranges.
  for (const prefix of ['history', 'analytics']) {
    const apply = () => prefix === 'history' ? window.HistoryDateRange.apply() : AnalyticsModule.applyDateRange();
    get(prefix + '-through-today').checked = true;
    for (const bad of ['', '2026-02-30', '2026-99-01', '2026-11-04']) {
      const before = get(prefix + '-start').value;
      get(prefix + '-start-display').value = bad;
      assert.doesNotThrow(apply);
      assert.equal(get(prefix + '-start').value, before);
      assert.equal(get(prefix + '-date-error').hidden, false);
    }
  }
  // The existing 92-day cap is inclusive and stays explicit after rollover.
  get('analytics-start-display').value = '2026-08-04';
  AnalyticsModule.applyDateRange();
  await settleView();
  assert.equal(get('analytics-start').value, '2026-08-04');
  advanceClock('2026-11-04T08:30:00Z');
  await AnalyticsModule.load(true);
  assert.equal(get('analytics-error').hidden, false);
  assert.ok(get('analytics-error').textContent.includes('92'));

  window.HistoryDateRange.clear(false);
  assert.equal(get('history-through-today').checked, false);
  assert.equal(get('history-end-display').disabled, false);
  assert.equal(historyQueryParams().has('end'), false);
  advanceClock('2028-02-29T12:00:00Z');
  get('analytics-start').value = '2028-02-29';
  get('analytics-end').value = 'today';
  assert.equal(AnalyticsModule.queryParams().get('to'), '2028-03-01T00:00:00.000Z');
}
`+platformBehaviorRunScript)
}

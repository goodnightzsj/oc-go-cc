package gui

import "testing"

// The derived figures the overview gained. Each one is only worth showing if it
// refuses to answer when its inputs are missing - a runway computed from a
// zero burn rate, or a success rate over no known outcomes, would state
// something the data does not support.
//
// The DOM shim has no HTML parser, so rendered output is asserted through
// innerHTML (matching the other behavior tests) and the pure helpers are
// called directly.
func TestDashboardDerivedFigures(t *testing.T) {
	runPlatformBehavior(t, derivedFiguresScript)
}

const derivedFiguresScript = platformBehaviorDOMScript + `
async function checks() {
  const get = id => document.getElementById(id);

  // --- runway: balance divided by the platform's own burn rate ---
  // $60 left, $30 spent over 30 days = $1/day, so 60 days.
  assert.equal(runwayDays(60, 30, 30), 60, 'runway divides balance by per-day spend');
  // The same rate expressed weekly must give the same answer, or the figure
  // depends on which period the caller happened to pass.
  assert.equal(runwayDays(60, 7, 7), 60, 'runway is period-independent');
  // No burn rate means no projection - not infinity, not zero.
  assert.equal(runwayDays(60, 0, 30), null, 'a zero burn rate yields no runway');
  assert.equal(runwayDays(0, 30, 30), 0, 'an exhausted balance is zero days, not unknown');
  // A missing balance is unknown, not exhausted: Number(null) is 0, so absence
  // has to be checked before coercion or the two collapse into one claim.
  assert.equal(runwayDays(null, 30, 30), null, 'a missing balance yields no runway');
  assert.equal(runwayDays(undefined, 30, 30), null, 'an undefined balance yields no runway');
  assert.equal(runwayDays(60, null, 30), null, 'missing usage yields no runway');

  // --- success thresholds ---
  assert.equal(successColor('ok'), 'var(--ui-green)');
  assert.equal(successColor('warn'), 'var(--ui-amber)');
  assert.equal(successColor('crit'), 'var(--ui-red)');
  assert.equal(successColor('unknown'), 'var(--ui-muted)', 'unknown must not borrow a passing colour');

  // --- breaker keys are provider/model ---
  assert.equal(providerOfModelKey('cline-pass/deepseek-v4-pro'), 'cline-pass');
  assert.equal(providerOfModelKey('a/b/c'), 'a', 'only the first segment is the platform');
  assert.equal(providerOfModelKey('noslash'), '', 'an unkeyed breaker is attributed to nobody');

  // --- the health row itself ---
  const providers = [
    { provider: 'opencode-go', requests: 10, known_requests: 10, success_rate: 1, avg_latency_ms: 100, fallback_rate: 0 },
    { provider: 'cline-pass', requests: 10, known_requests: 10, success_rate: 0.5, avg_latency_ms: 200, fallback_rate: 30 },
    { provider: 'commandcode', requests: 10, known_requests: 0, success_rate: 0, avg_latency_ms: 0, fallback_rate: 0 },
    { provider: 'aws-bedrock', requests: 0, known_requests: 0, success_rate: 0, avg_latency_ms: 0, fallback_rate: 0 },
  ];
  const breakers = { 'opencode-go/glm-5.2': 'closed', 'cline-pass/deepseek-v4-pro': 'open', 'commandcode/x': 'closed' };
  AnalyticsModule.renderPlatformHealth(providers, breakers);
  let html = get('platform-health-rows').innerHTML;
  assert.equal(get('platform-health').hidden, false, 'the section shows when a platform has traffic');
  assert.equal((html.match(/platform-health-row/g) || []).length, 3,
    'a platform with no traffic is not given a health row');
  assert.ok(!html.includes('aws-bedrock'), 'the zero-traffic platform is absent from the rows');
  assert.ok(html.includes('100.0%'), 'a clean platform reads as a full success rate');
  assert.ok(html.includes('50.0%'), 'a degraded platform reads its own rate');
  assert.ok(html.includes('30%'), 'the fallback share appears on the row');
  assert.ok(html.includes(t('health.breakerOpen')) && html.includes('is-crit'),
    'an open breaker must read as a failure, not as normal');
  assert.ok(html.includes(t('health.breakerClosed')) && html.includes('is-ok'),
    'a closed breaker reads as healthy');
  // No known outcome: a dash, not 0.0%.
  assert.ok(html.includes('commandcode') && html.includes(t('health.noKnownOutcome')),
    'a platform with no known outcome says so');
  // Matched against the value element, not the whole string: a bar width of
  // "50.0%" contains the substring "0.0%" and would satisfy a loose check.
  assert.ok(!html.includes('>0.0%<'), 'no platform may claim a 0% success rate it did not measure');

  // No traffic at all: the section hides rather than showing an empty frame.
  AnalyticsModule.renderPlatformHealth([], null);
  assert.equal(get('platform-health').hidden, true, 'the section hides when no platform has traffic');
  assert.equal(get('platform-health-rows').innerHTML, '', 'hidden section keeps no rows');

  // Breaker state unavailable: the dot must not claim "closed".
  AnalyticsModule.renderPlatformHealth([providers[0]], null);
  html = get('platform-health-rows').innerHTML;
  assert.ok(!html.includes('is-ok'), 'missing breaker data must not render as healthy');
  assert.ok(html.includes(t('health.breakerUnknown')), 'missing breaker data is named as unknown');

  // --- daily bars ---
  AnalyticsModule.renderDailySpend([
    { date: '2026-09-20', cost_usd: 10 },
    { date: '2026-09-21', cost_usd: 0 },
    { date: '2026-09-22', cost_usd: 5 },
  ]);
  html = get('daily-spend-bars').innerHTML;
  assert.equal(get('daily-spend').hidden, false);
  // Anchored on the class attribute's end quote so the per-column children
  // (daily-bar-value, -fill, -label) are not counted as columns.
  assert.equal((html.match(/class="daily-bar(?: is-empty)?"/g) || []).length, 3,
    'one column per day in the range');
  assert.ok(html.includes('is-empty'), 'a day with no cost is drawn as an empty column');
  // The tallest day fills the column; the half day is half of it.
  assert.ok(html.includes('height:100.0%'), 'the largest day fills its column');
  assert.ok(html.includes('height:50.0%'), 'a half-size day draws at half height');
  assert.ok(get('daily-spend-note').textContent.includes('1'),
    'the note names how many days have no recorded cost');

  AnalyticsModule.renderDailySpend([]);
  assert.equal(get('daily-spend').hidden, true, 'the section hides with no days');
  assert.equal(get('daily-spend-bars').innerHTML, '', 'hidden section keeps no bars');
}
` + platformBehaviorRunScript

package gui

import (
	"strings"
	"testing"
)

// Every state class the markup emits must be styled, or the element silently
// inherits whatever its container set. That is how a "limited" badge once
// rendered in the success green: .is-warn was emitted but never defined, and
// the badge fell through to the card's --quota-color.
func TestQuotaStateBadgesAreStyled(t *testing.T) {
	data, err := assets.ReadFile("assets/style.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(data)
	for _, selector := range []string{".quota-badge.is-warn", ".quota-badge.is-crit"} {
		if !strings.Contains(css, selector) {
			t.Errorf("quota badge state class is emitted but unstyled: %s", selector)
		}
	}
	// The warning colour must come from the amber token rather than inheriting
	// the card's level colour, which is green whenever the same window is under
	// the crit threshold.
	warn := cssSection(css, ".quota-badge.is-warn")
	if !strings.Contains(warn, "--ui-amber") {
		t.Errorf(".quota-badge.is-warn must use the amber token, got %q", warn)
	}
}

// cssSection returns the declaration block following the selector, up to its
// closing brace. A missing selector yields an empty string.
func cssSection(css, selector string) string {
	at := strings.Index(css, selector)
	if at < 0 {
		return ""
	}
	rest := css[at+len(selector):]
	end := strings.Index(rest, "}")
	if end < 0 {
		return rest
	}
	return rest[:end]
}

// The chart's viewBox is its drawing space and the SVG is stretched to the
// container, so a fixed viewBox scales the axis text with the container: the
// same label rendered at 4.3px in a half-width tile and 10.3px in a full-width
// one. Sizing the drawing space to the measured container keeps that scale at
// 1 at every width.
func TestChartViewBoxFollowsContainerWidth(t *testing.T) {
	runPlatformBehavior(t, chartWidthBehaviorScript)
}

const chartWidthBehaviorScript = platformBehaviorDOMScript + `
async function checks() {
  const get = id => document.getElementById(id);
  // The shim has no layout engine, so clientWidth is supplied per container to
  // stand in for the widths the grid actually produces. The getter needs a
  // matching setter: a getter-only property would swallow the assignments below
  // and silently leave every width at its first value.
  const widths = {'analytics-request-trend': 312, 'analytics-token-trend': 989};
  const setWidth = (id, value) => {
    widths[id] = value;
    const el = get(id);
    Object.defineProperty(el, 'clientWidth', {
      configurable: true,
      get: () => widths[id],
      set: next => { widths[id] = next; },
    });
  };
  for (const id of Object.keys(widths)) setWidth(id, widths[id]);
  const points = [
    {date:'2026-09-20',requests:10,error_requests:1,input_tokens:100,output_tokens:10,cache_read_tokens:5,cache_creation_tokens:1},
    {date:'2026-09-21',requests:20,error_requests:0,input_tokens:200,output_tokens:20,cache_read_tokens:9,cache_creation_tokens:2},
  ];
  const viewBoxWidth = containerId => {
    const match = get(containerId).innerHTML.match(/viewBox="0 0 (\d+(?:\.\d+)?)/);
    assert.ok(match, 'chart must emit a viewBox: ' + containerId);
    return Number(match[1]);
  };

  AnalyticsModule.renderRequestTrend(points, 'analytics-request-trend');
  assert.equal(viewBoxWidth('analytics-request-trend'), 312,
    'the request chart must draw in the container width, not a fixed one');
  AnalyticsModule.renderTokenLines(points, 'analytics-token-trend');
  assert.equal(viewBoxWidth('analytics-token-trend'), 989,
    'the token chart must draw in its own container width');

  // A container that reports no width (hidden tab, unlayouted DOM) must still
  // produce a valid chart rather than a zero-width one.
  get('analytics-request-trend').clientWidth = 0;
  AnalyticsModule.renderRequestTrend(points, 'analytics-request-trend');
  assert.ok(viewBoxWidth('analytics-request-trend') > 0,
    'an unmeasurable container must fall back to a drawable width');

  // A resize redraws only the charts whose width moved.
  get('analytics-request-trend').clientWidth = 640;
  AnalyticsModule.redrawCharts();
  assert.equal(viewBoxWidth('analytics-request-trend'), 640,
    'a resized chart must be redrawn at the new width');
  assert.equal(viewBoxWidth('analytics-token-trend'), 989,
    'an unchanged chart must keep its geometry');

  // An empty dataset clears the stored geometry, so a later resize cannot
  // resurrect a chart from stale points.
  get('analytics-request-trend').clientWidth = 700;
  AnalyticsModule.renderRequestTrend([], 'analytics-request-trend');
  assert.ok(!AnalyticsModule.chartData['analytics-request-trend'],
    'an empty chart must not retain geometry a resize could redraw');
  AnalyticsModule.redrawCharts();
  assert.ok(get('analytics-request-trend').innerHTML.includes(t('analytics.noTrend')),
    'an empty chart must stay empty after a resize');
}
` + platformBehaviorRunScript

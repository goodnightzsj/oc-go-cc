package gui

import (
	"strings"
	"testing"
)

func TestAnalyticsAssetsKeepRefreshAndDrillDownBehavior(t *testing.T) {
	app, err := assets.ReadFile("assets/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	page, err := assets.ReadFile("assets/index.html")
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}

	for _, marker := range []string{"loadSeq", "scheduleRefresh()", `aria-expanded="false"`, "legend-details", "function fmtTok", "bindChartTooltip", "breakdownMetric", "renderOverviewUsage", "overview-model-donut"} {
		if !strings.Contains(string(app), marker) {
			t.Errorf("app.js missing analytics marker %q", marker)
		}
	}
	if !strings.Contains(string(page), `id="analytics-refresh-interval"`) {
		t.Error("Analytics auto-refresh control is missing")
	}
	for _, marker := range []string{`id="analytics-breakdown-metric"`, `id="overview-token-trend"`, `id="overview-provider-donut"`, `id="kpi-cache-read"`} {
		if !strings.Contains(string(page), marker) {
			t.Errorf("Analytics account view is missing %q", marker)
		}
	}
	for _, forbidden := range []string{`id="analytics-source"`, `id="plan-donut"`, `id="analytics-recent"`} {
		if strings.Contains(string(page), forbidden) {
			t.Errorf("Analytics still exposes removed element %q", forbidden)
		}
	}
}

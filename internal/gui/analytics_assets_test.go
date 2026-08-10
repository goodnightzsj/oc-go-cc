package gui

import (
	"strings"
	"testing"
)

func TestAnalyticsAssetsMatchUsageDashboardContract(t *testing.T) {
	app, err := assets.ReadFile("assets/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	page, err := assets.ReadFile("assets/index.html")
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}

	for _, marker := range []string{
		"loadSeq", "function fmtTok", "bindChartTooltip", "breakdownMetric",
		"renderOverviewUsage", "renderRequestTrend", "renderTokenLines",
		"renderDistribution", "renderPeriodTable", "fillTrend", "queryParams",
		"bindHistoryTokenTooltips",
	} {
		if !strings.Contains(string(app), marker) {
			t.Errorf("app.js missing analytics marker %q", marker)
		}
	}
	for _, marker := range []string{
		`id="analytics-date-trigger"`, `id="analytics-granularity"`,
		`id="analytics-period-tbody"`, `id="analytics-model-tbody"`,
		`id="overview-request-trend"`, `id="overview-token-trend"`,
		`id="overview-provider-distribution"`, `id="overview-model-distribution"`,
		`id="kpi-cache-read"`, `id="kpi-cache-write"`,
	} {
		if !strings.Contains(string(page), marker) {
			t.Errorf("Analytics usage view is missing %q", marker)
		}
	}
	for _, forbidden := range []string{
		`id="analytics-source"`, `id="plan-donut"`, `id="analytics-recent"`,
		`id="analytics-refresh-interval"`, `id="model-donut"`, `id="provider-donut"`,
		`id="overview-model-donut"`, `id="overview-provider-donut"`,
	} {
		if strings.Contains(string(page), forbidden) || strings.Contains(string(app), forbidden) {
			t.Errorf("Analytics still exposes removed element %q", forbidden)
		}
	}
	for _, forbidden := range []string{"renderDonutChart(", "scheduleRefresh()", `class="trend-hit"`} {
		if strings.Contains(string(app), forbidden) {
			t.Errorf("app.js still contains superseded chart behavior %q", forbidden)
		}
	}
}

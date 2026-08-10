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

	for _, marker := range []string{"loadSeq", "scheduleRefresh()", `aria-expanded="false"`, "legend-details", "provider_cost_rows", "kpi-cost-source"} {
		if !strings.Contains(string(app), marker) {
			t.Errorf("app.js missing analytics marker %q", marker)
		}
	}
	if !strings.Contains(string(page), `id="analytics-refresh-interval"`) {
		t.Error("Analytics auto-refresh control is missing")
	}
}

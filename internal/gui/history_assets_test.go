package gui

import (
	"strings"
	"testing"
)

func TestHistoryAssetsKeepKeyboardDialogAndPromptTokens(t *testing.T) {
	app, err := assets.ReadFile("assets/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	page, err := assets.ReadFile("assets/index.html")
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}

	for _, marker := range []string{
		`tabindex="0" aria-haspopup="dialog"`, "modal.showModal()", "prompt_tokens",
		"historyQueryParams", "record.error_msg", "legend-drilldown", "cost_usd", `colspan="8"`,
		"window.CustomSelect", "window.HistoryDateRange", "renderHistorySummary", "detail.title",
		"historyBreakdownMetric", "historyTrendMetric", "bindChartTooltip", "details_known",
	} {
		if !strings.Contains(string(app), marker) {
			t.Errorf("app.js missing History accessibility marker %q", marker)
		}
	}
	if strings.Contains(string(app), `e.key === 'r'`) || strings.Contains(string(app), `e.key === "r"`) {
		t.Error("application JavaScript must not intercept the browser refresh shortcut")
	}
	if strings.Contains(string(page), "<datalist") {
		t.Error("History must not expose a browser-native datalist popup")
	}
	if !strings.Contains(string(page), `<dialog class="modal-overlay" id="history-modal"`) {
		t.Error("History detail must remain a native dialog")
	}
	for _, marker := range []string{`type="hidden" id="history-start"`, `type="hidden" id="history-end"`, `id="history-date-popover"`, `id="history-summary-requests"`, `id="status-filter"`, `id="streaming-filter"`, `id="history-breakdown-metric"`, `id="history-trend-metric"`, `data-sort="cost_usd"`} {
		if !strings.Contains(string(page), marker) {
			t.Errorf("index.html missing server-side History filter %q", marker)
		}
	}
	if strings.Contains(string(page), `id="cost-source-filter"`) || strings.Contains(string(app), "costSourceLabel") {
		t.Error("History must not expose internal cost provenance")
	}
}

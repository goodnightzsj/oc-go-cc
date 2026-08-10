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
	} {
		if !strings.Contains(string(app), marker) {
			t.Errorf("app.js missing History accessibility marker %q", marker)
		}
	}
	if !strings.Contains(string(page), `<dialog class="modal-overlay" id="history-modal"`) {
		t.Error("History detail must remain a native dialog")
	}
	for _, marker := range []string{`id="history-start"`, `id="history-end"`, `id="status-filter"`, `id="streaming-filter"`, `data-sort="cost_usd"`} {
		if !strings.Contains(string(page), marker) {
			t.Errorf("index.html missing server-side History filter %q", marker)
		}
	}
}

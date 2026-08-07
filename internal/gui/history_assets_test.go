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

	for _, marker := range []string{`tabindex="0" aria-haspopup="dialog"`, "modal.showModal()", "prompt_tokens"} {
		if !strings.Contains(string(app), marker) {
			t.Errorf("app.js missing History accessibility marker %q", marker)
		}
	}
	if !strings.Contains(string(page), `<dialog class="modal-overlay" id="history-modal"`) {
		t.Error("History detail must remain a native dialog")
	}
}

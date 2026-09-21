package gui

import (
	"strings"
	"testing"
)

// Two elements share .modal-overlay and only one of them is a <dialog>:
// #history-modal gets a browser-painted ::backdrop, #test-modal is a plain div
// and gets nothing. A mask rule that only considers one of them either doubles
// the scrim on the dialog or drops it entirely on the div.
//
// This was a real regression: making the overlay transparent to stop the
// doubling left #test-modal with no mask at all, because nothing else painted
// one for it.
func TestModalMaskIsPaintedExactlyOnce(t *testing.T) {
	css := readStyleSheet(t)

	// The dialog must not paint its own background: ::backdrop already does.
	// Two opaque layers composite (40% over 40% is 64%), which reads as a flat
	// grey wash rather than a scrim.
	dialogOpen := cssSection(css, "dialog.modal-overlay[open]")
	if dialogOpen == "" {
		t.Fatal("dialog.modal-overlay[open] is not styled")
	}
	if !strings.Contains(dialogOpen, "background: transparent") {
		t.Errorf("the dialog must stay transparent so the scrim is painted once, got %q", dialogOpen)
	}

	// The div variant has no ::backdrop, so it must paint its own mask.
	divOverlay := cssSection(css, "div.modal-overlay ")
	if divOverlay == "" {
		t.Fatal("div.modal-overlay must paint its own mask (it has no ::backdrop)")
	}
	if !strings.Contains(divOverlay, "--ui-scrim") {
		t.Errorf("the div variant must use the scrim token, got %q", divOverlay)
	}

	// Both variants must use the token, so the mask cannot be a value that only
	// works in one theme.
	backdrop := cssSection(css, "dialog.modal-overlay::backdrop")
	if !strings.Contains(backdrop, "--ui-scrim") {
		t.Errorf("the backdrop must use the scrim token, got %q", backdrop)
	}
}

// TestScrimTokenDarkensInBothThemes pins the mistake that made light mode look
// muddy: a scrim derived from --ui-bg brightens the page in light mode, where
// --ui-bg is near-white, so the mask pushed the page forward instead of back.
func TestScrimTokenDarkensInBothThemes(t *testing.T) {
	css := readStyleSheet(t)

	// Three theme blocks define the tokens: the :root default (dark), the
	// prefers-color-scheme light override, and the explicit [data-theme] one.
	// Every one of them must carry the scrim, or a theme falls back to the
	// default and paints the wrong weight.
	if got := strings.Count(css, "--ui-scrim:"); got != 3 {
		t.Errorf("--ui-scrim defined %d times, want 3 (default + both light blocks)", got)
	}

	for _, block := range scrimDeclarations(css) {
		if !strings.Contains(block, "rgba(") {
			t.Errorf("scrim must be an explicit dark colour, not a token mix: %q", block)
		}
		// A scrim built from color-mix on --ui-bg would brighten light mode.
		if strings.Contains(block, "color-mix") || strings.Contains(block, "--ui-bg") {
			t.Errorf("scrim must not be derived from the page background: %q", block)
		}
	}
}

func readStyleSheet(t *testing.T) string {
	t.Helper()
	data, err := assets.ReadFile("assets/style.css")
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// scrimDeclarations returns each line that assigns --ui-scrim.
func scrimDeclarations(css string) []string {
	var out []string
	for _, line := range strings.Split(css, "\n") {
		if strings.Contains(line, "--ui-scrim:") {
			out = append(out, strings.TrimSpace(line))
		}
	}
	return out
}

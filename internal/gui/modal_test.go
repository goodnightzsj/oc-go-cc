package gui

import (
	"strings"
	"testing"
)

// The modal has no mask, in either theme. This replaced a scrim that could not
// be made to work in both at once: a dark tint over the dark theme's ground
// darkened it by ~2% (invisible), while the same tint over the light theme's
// near-white page darkened it by ~25% (a grey film). The panel separates itself
// with a border and a large shadow instead.
//
// The tests below pin the absence, because "no mask" is the kind of thing that
// gets added back one convenience property at a time.
func TestModalHasNoMask(t *testing.T) {
	css := readStyleSheet(t)

	// No tint token survives anywhere: a leftover definition would be dead
	// weight that the next reader would reasonably think is in use.
	if got := strings.Count(css, "--ui-scrim"); got != 0 {
		t.Errorf("--ui-scrim still appears %d time(s); the mask was removed", got)
	}

	// No shaped mask layer.
	if strings.Contains(css, ".modal-overlay::before") {
		t.Error(".modal-overlay::before is the mask layer, which was removed")
	}

	// The dialog's backdrop must be explicitly transparent. Leaving the rule out
	// entirely is not enough: the browser supplies its own ::backdrop with
	// rgba(0,0,0,0.1), so deleting the scrim without this override leaves a faint
	// mask behind. Measured: the page was still darkened after every tint token
	// had been removed.
	backdrop := cssSection(css, "dialog.modal-overlay::backdrop")
	if backdrop == "" {
		t.Fatal("the backdrop must be overridden to transparent; the UA default is rgba(0,0,0,0.1)")
	}
	if !strings.Contains(backdrop, "background: transparent") {
		t.Errorf("the backdrop must be transparent, got %q", backdrop)
	}
	if strings.Contains(backdrop, "backdrop-filter") {
		t.Errorf("the backdrop must not frost either, got %q", backdrop)
	}

	// The overlay element itself must not tint either, or the div variant
	// (#test-modal) would get a mask the dialog does not.
	overlay := cssSection(css, ".modal-overlay {")
	if overlay == "" {
		t.Fatal(".modal-overlay is not styled")
	}
	if strings.Contains(overlay, "background") || strings.Contains(overlay, "backdrop-filter") {
		t.Errorf("the overlay must not paint a mask, got %q", overlay)
	}
}

// TestPanelSeparatesItselfWithoutAMask pins what actually distinguishes the
// dialog from the page now that nothing dims the page: a defined edge and an
// elevation shadow. Removing both would leave the panel flat against the
// content behind it.
func TestPanelSeparatesItselfWithoutAMask(t *testing.T) {
	css := readStyleSheet(t)

	// Match on the panel rule itself, not the selector: a reduced-motion
	// override also starts with ".modal-content " and appears earlier in the
	// file, so a plain selector lookup would read the wrong block.
	panel := cssSection(css, ".modal-content { background:")
	if panel == "" {
		t.Fatal(".modal-content is not styled")
	}
	if !strings.Contains(panel, "border: 1px solid") {
		t.Errorf("the panel needs a defined edge in place of the mask, got %q", panel)
	}
	if !strings.Contains(panel, "box-shadow:") {
		t.Errorf("the panel needs an elevation shadow in place of the mask, got %q", panel)
	}
	if !strings.Contains(panel, "border-radius: 14px") {
		t.Errorf("the panel must stay rounded, got %q", panel)
	}

	// The detail dialog is the large variant and repeats this contract.
	detail := cssSection(css, ".history-detail-content {")
	if detail == "" {
		t.Fatal(".history-detail-content is not styled")
	}
	if !strings.Contains(detail, "box-shadow:") || !strings.Contains(detail, "border: 1px solid") {
		t.Errorf("the detail panel needs its own edge and shadow, got %q", detail)
	}
}

// TestDialogStretchesToTheViewport pins the rule that makes a click outside the
// panel close it. A <dialog> shrink-wraps to its content and centres itself with
// `margin: auto`, so its box would be the panel's size rather than the
// viewport's; a click beside the panel would then land on the page behind
// instead of on the dialog. Stretching it is what routes that click to the
// dialog's own close handler.
func TestDialogStretchesToTheViewport(t *testing.T) {
	css := readStyleSheet(t)

	open := cssSectionContaining(css, "dialog.modal-overlay[open]", "max-width")
	if open == "" {
		t.Fatal("dialog.modal-overlay[open] is not styled")
	}
	for _, prop := range []string{"max-width: none", "max-height: none", "margin: 0"} {
		if !strings.Contains(open, prop) {
			t.Errorf("the dialog must stretch to the viewport; missing %q in %q", prop, open)
		}
	}
	if strings.Contains(open, "max-width: calc(") {
		t.Errorf("a computed max-width re-constrains the dialog box: %q", open)
	}
	// The dialog itself must stay invisible: it covers the viewport now, so a
	// background or border here would paint over the whole page.
	if !strings.Contains(open, "background: transparent") {
		t.Errorf("the stretched dialog must stay transparent, got %q", open)
	}
}

// TestReducedMotionKeepsTheModalUsable pins that the reduced-motion block only
// drops the transition. Switching the overlay to `display: none` or hiding the
// panel there would make the modal unreachable for anyone with that preference.
func TestReducedMotionKeepsTheModalUsable(t *testing.T) {
	css := readStyleSheet(t)

	block := cssSectionContaining(css, "@media (prefers-reduced-motion: reduce)", "transition: none")
	if block == "" {
		t.Fatal("reduced-motion must disable the modal transitions")
	}
	for _, forbidden := range []string{"display: none", "visibility: hidden", "opacity: 0"} {
		if strings.Contains(block, forbidden) {
			t.Errorf("reduced-motion must not hide the modal (%q): %q", forbidden, block)
		}
	}
}

// cssSectionContaining returns the text of the at-rule block that starts at
// `marker` and contains `needle` inside it. cssSection cannot be used here: it
// stops at the first closing brace, which for a media query is the end of the
// first inner rule, so it would never see the declaration being asserted on.
func cssSectionContaining(css, marker, needle string) string {
	at := strings.Index(css, marker)
	if at < 0 {
		return ""
	}
	rest := css[at:]
	// Walk braces to find where the at-rule itself closes.
	depth := 0
	for i, r := range rest {
		switch r {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				block := rest[:i+1]
				if strings.Contains(block, needle) {
					return block
				}
				return ""
			}
		}
	}
	return ""
}

func readStyleSheet(t *testing.T) string {
	t.Helper()
	data, err := assets.ReadFile("assets/style.css")
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

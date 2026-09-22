package gui

import (
	"fmt"
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

	// The div variant has no ::backdrop, so the shaped layer is its only mask.
	// That layer carries the tint for both variants.
	shaped := cssSectionContaining(css, ".modal-overlay::before", "inset")
	if shaped == "" {
		t.Fatal(".modal-overlay::before must be the shaped mask layer")
	}
	if !strings.Contains(shaped, "--ui-scrim") {
		t.Errorf("the shaped layer must carry the tint, got %q", shaped)
	}

	// The dialog's own layer must be transparent, not tinted: a tint there fills
	// the inset band too, which is what made the rounded corner invisible.
	backdrop := cssSection(css, "dialog.modal-overlay::backdrop")
	if !strings.Contains(backdrop, "background: transparent") {
		t.Errorf("the backdrop must stay transparent so the shaped layer's edge reads, got %q", backdrop)
	}
	if strings.Contains(backdrop, "backdrop-filter") {
		t.Errorf("the backdrop must not frost: the shaped layer already does: %q", backdrop)
	}
}

// TestMaskShapeIsLegibleAndFrosted pins the two properties that make the mask
// read as a shaped sheet rather than a full-bleed veil: an inset wide enough to
// show the page's own colour around the edge, and a corner radius on that edge.
func TestMaskShapeIsLegibleAndFrosted(t *testing.T) {
	css := readStyleSheet(t)

	shape := cssSectionContaining(css, ".modal-overlay::before", "inset")
	if shape == "" {
		t.Fatal(".modal-overlay::before must be the shaped mask layer")
	}
	if !strings.Contains(shape, "inset:") {
		t.Errorf("the shaped mask must be inset from the viewport, got %q", shape)
	}
	if !strings.Contains(shape, "border-radius:") {
		t.Errorf("the shaped mask must be rounded, got %q", shape)
	}
	if !strings.Contains(shape, "backdrop-filter: blur(") {
		t.Errorf("the shaped mask must carry the frost, got %q", shape)
	}

	// The inset has to be a visible band, not a hairline. At 10px the band was
	// the same width as the rounding, so the corner was indistinguishable from a
	// straight edge and the shape read as a rectangle again.
	inset := 0
	fmt.Sscanf(strings.TrimSpace(strings.SplitN(shape, "inset:", 2)[1]), "%d", &inset)
	if inset < 16 {
		t.Errorf("inset %dpx is too narrow for the rounding to be legible", inset)
	}

	// The panel has to sit above that layer or the mask would cover it.
	if cssSection(css, ".modal-content { position: relative") == "" {
		t.Fatal("the panel must be raised above the mask layer")
	}
}

// TestDialogStretchesToTheViewport pins the root cause of a mask that vanished
// entirely. A <dialog> shrink-wraps to its content and centres itself with
// `margin: auto`, so its box is the panel's size rather than the viewport's. A
// mask painted by ::before is then inset *inside the panel* and hidden behind
// it - measured: the dialog box was 720x552, ::before computed to 676x508, and
// no pixel outside the panel changed when the modal opened (delta +0.0 in both
// themes). Stretching the dialog to the viewport is what makes ::before a mask
// at all, and it is the one part of this design that CSS assertions elsewhere
// cannot see.
func TestDialogStretchesToTheViewport(t *testing.T) {
	css := readStyleSheet(t)

	open := cssSectionContaining(css, "dialog.modal-overlay[open]", "max-width")
	if open == "" {
		t.Fatal("dialog.modal-overlay[open] is not styled")
	}
	// A shrink-wrapped dialog re-imposes the default constrained box. Any of
	// these left in place puts the box back to the panel's size.
	for _, prop := range []string{"max-width: none", "max-height: none", "margin: 0"} {
		if !strings.Contains(open, prop) {
			t.Errorf("the dialog must stretch to the viewport; missing %q in %q", prop, open)
		}
	}
	if strings.Contains(open, "max-width: calc(") {
		t.Errorf("a computed max-width re-constrains the dialog box: %q", open)
	}
}

// TestScrimIsTunedPerTheme pins the mistake that made the dark mask invisible.
// A tint's reach depends on the ground under it, not on its own alpha: against
// a near-white page 0.12 darkens by ~27 steps, while against the dark theme's
// #101622 the same 0.12 darkens by ~3 and reads as no mask at all. The dark
// scrim must therefore be much heavier than the light one.
func TestScrimIsTunedPerTheme(t *testing.T) {
	css := readStyleSheet(t)

	// Three blocks define it: the default (dark), and both light overrides.
	if got := strings.Count(css, "--ui-scrim:"); got != 3 {
		t.Errorf("--ui-scrim defined %d times, want 3 (default + both light blocks)", got)
	}

	dark, light := "", ""
	for _, line := range scrimDeclarations(css) {
		if strings.Contains(line, "rgba(0, 0, 0") {
			dark = line
		} else {
			light = line
		}
	}
	if dark == "" || light == "" {
		t.Fatalf("expected one near-black scrim and one tinted scrim, got dark=%q light=%q", dark, light)
	}

	alpha := func(line string) float64 {
		open := strings.LastIndex(line, ",")
		if open < 0 {
			return 0
		}
		var v float64
		fmt.Sscanf(strings.TrimSpace(strings.TrimSuffix(line[open+1:], ")")), "%f", &v)
		return v
	}
	d, l := alpha(dark), alpha(light)
	if d <= l {
		t.Errorf("the dark scrim must be heavier than the light one (dark=%v light=%v): "+
			"an equal alpha is invisible against the dark theme's ground", d, l)
	}
	// The ground is dark enough that only a high alpha moves it at all.
	if d < 0.6 {
		t.Errorf("the dark scrim alpha %v is too low to be visible against #101622", d)
	}
}

// TestBackdropFrostIsDroppedUnderReducedMotion pins the two halves of the
// decoration contract: the blur is decoration and must be dropped when the
// viewer asks for less motion, but the tint is what keeps the panel separated
// and must survive that. Resetting the background here as well would leave the
// dialog unanchored, floating over an undimmed page.
func TestBackdropFrostIsDroppedUnderReducedMotion(t *testing.T) {
	css := readStyleSheet(t)

	block := cssSectionContaining(css, "@media (prefers-reduced-motion: reduce)", "backdrop-filter")
	if block == "" {
		t.Fatal("reduced-motion must reset the backdrop frost")
	}
	// The tint must not be reset alongside the blur. Asserting on "--ui-scrim"
	// would miss the realistic mistake: writing `background: transparent` to
	// "turn the backdrop off", which names no token at all.
	if strings.Contains(block, "background") {
		t.Errorf("reduced-motion must drop only the blur, not the scrim: %q", block)
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

// TestScrimTokenDarkensInBothThemes pins the mistake that made light mode look
// muddy: a scrim derived from --ui-bg brightens the page in light mode, where
// --ui-bg is near-white, so the mask pushed the page forward instead of back.
// The per-theme weight is pinned separately by TestScrimIsTunedPerTheme.
func TestScrimTokenDarkensInBothThemes(t *testing.T) {
	css := readStyleSheet(t)

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

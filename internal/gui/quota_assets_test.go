package gui

import (
	"strings"
	"testing"
)

// The Quota page is data-driven from /api/quota; these markers pin the element
// ids and the client-side maths the renderer depends on.
func TestQuotaAssetsExposePlanWindows(t *testing.T) {
	app, err := assets.ReadFile("assets/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	page, err := assets.ReadFile("assets/index.html")
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	style, err := assets.ReadFile("assets/style.css")
	if err != nil {
		t.Fatalf("read style.css: %v", err)
	}

	for _, marker := range []string{
		`data-tab="quota"`, `id="tab-quota"`, `id="quota-accounts"`, `id="quota-endpoint"`,
		`id="quota-bottleneck"`, `id="quota-remaining"`, `id="quota-next-reset"`, `id="quota-plan"`,
		`id="quota-model-limits"`, `id="btn-refresh-quota"`, `data-i18n="quota.unofficial"`, `id="cmd-goto-quota"`,
		`id="icon-refresh"`, `class="btn btn-small btn-icon"`,
	} {
		if !strings.Contains(string(page), marker) {
			t.Errorf("index.html missing quota marker %q", marker)
		}
	}
	for _, marker := range []string{
		"QuotaModule", "/api/quota?${params}", "params.set('refresh', '1')", "QUOTA_RING_LENGTH", "fmtCountdown",
		"tickCountdowns", "data-deadline", "rolling_5h", "percent_derived", "key_hint",
		"case 'quota':", "levelOf(", "deadlineOf(", "renderModelLimits", "model_limits", "model_usage",
		"used_usd",
	} {
		if !strings.Contains(string(app), marker) {
			t.Errorf("app.js missing quota marker %q", marker)
		}
	}
	// Both locales must define the quota strings; a missing key renders as the
	// raw key in the UI.
	for _, key := range []string{"'tab.quota'", "'quota.title'", "'quota.rolling5h'", "'quota.noKeyHint'", "'quota.resetsIn'",
		"'quota.modelLimits'", "'quota.modelAllowance'", "'quota.modelLimitsNote'", "'quota.model'",
		"'quota.modelUsed'", "'quota.percent'", "'quota.total'"} {
		if strings.Count(string(app), key+":") < 2 {
			t.Errorf("translation key %s must exist in both en and zh", key)
		}
	}
	for _, marker := range []string{".quota-card", ".quota-gauge-fill", ".quota-figures", "level-crit",
		".quota-model-limits", ".quota-model-scroll", ".quota-model-number", ".btn-icon", ".sr-only"} {
		if !strings.Contains(string(style), marker) {
			t.Errorf("style.css missing quota rule %q", marker)
		}
	}
}

// TestMultiKeyAccountCardsSitSideBySide pins the ClinePass account layout.
//
// Two things had to hold, and each was wrong at some point:
//
//   - The card's single block must fill the card. It used to sit in a
//     two-column panel grid that exists for CommandCode, whose card really does
//     hold four blocks; with one block the content occupied the left 53% and the
//     windows were squeezed into 286px each inside a 1204px card.
//   - Two keys must sit side by side rather than one screen apart, and the
//     card's own windows stay two per row so they fit a half-width card.
//
// The renderer's markup and the stylesheet are checked together: a class with no
// rule does nothing, and a rule with no class never applies.
func TestMultiKeyAccountCardsSitSideBySide(t *testing.T) {
	app, err := assets.ReadFile("assets/app.js")
	if err != nil {
		t.Fatal(err)
	}
	page, err := assets.ReadFile("assets/index.html")
	if err != nil {
		t.Fatal(err)
	}
	source := string(app)

	// ClinePass is the renderer whose single block is the plan windows.
	clinePass := source[strings.Index(source, "renderClinePassAccount"):]
	clinePass = clinePass[:strings.Index(clinePass, "renderAccount(account)")]
	if !strings.Contains(clinePass, `class="commandcode-panels commandcode-panels-single"`) {
		t.Error("the ClinePass card must use the single-column panel wrapper so its block fills the card")
	}
	if strings.Contains(clinePass, `class="commandcode-panels"`) {
		t.Error("the ClinePass card must not use the bare two-column panel wrapper")
	}
	// CommandCode has four blocks and keeps the two-column panels.
	commandCode := source[strings.Index(source, "renderCommandCodeAccount"):]
	commandCode = commandCode[:strings.Index(commandCode, "renderClinePass(view)")]
	if !strings.Contains(commandCode, `class="commandcode-panels"`) || strings.Contains(commandCode, "commandcode-panels-single") {
		t.Error("CommandCode has four blocks and must keep the two-column panel grid")
	}

	// Both multi-key containers carry the side-by-side class.
	for _, id := range []string{"quota-commandcode-accounts", "quota-cline-pass-accounts"} {
		if !strings.Contains(string(page), `id="`+id+`" class="quota-accounts quota-accounts-grid"`) {
			t.Errorf("%s must carry quota-accounts-grid so its keys lay out side by side", id)
		}
	}
	// The single-window-grid platforms keep the one-column container.
	for _, id := range []string{"quota-accounts", "quota-openrouter-accounts"} {
		if strings.Contains(string(page), `id="`+id+`" class="quota-accounts quota-accounts-grid"`) {
			t.Errorf("%s must not carry the two-column class", id)
		}
	}

	css := readStyleSheet(t)
	if !strings.Contains(css, ".commandcode-panels-single { grid-template-columns: minmax(0, 1fr); }") {
		t.Error("the single-column panel modifier must collapse the panel grid to one column")
	}
	if !strings.Contains(css, ".quota-accounts.quota-accounts-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); align-items: start; margin-top: 16px; }") {
		t.Error("the account grid must put two keys side by side, with a top margin off the scope note")
	}
	if !strings.Contains(css, ".quota-accounts.quota-accounts-grid > :only-child") {
		t.Error("a lone account must span the full width rather than sit in the left half")
	}
	// The window grid keeps its default two-per-row; a three-column override
	// would stretch the progress bars across a half-width card.
	if strings.Contains(css, ".commandcode-panels-single .commandcode-windows") {
		t.Error("the card's windows must keep the default two-per-row grid")
	}
	// The narrow breakpoint collapses the side-by-side grid again.
	at := strings.Index(css, "@media (max-width: 1280px) {")
	if at < 0 {
		t.Fatal("the 1280px breakpoint is gone")
	}
	if !strings.Contains(css[at:at+1400], ".quota-accounts.quota-accounts-grid { grid-template-columns: minmax(0, 1fr); }") {
		t.Error("the side-by-side grid must collapse to one column at the 1280px breakpoint")
	}
}

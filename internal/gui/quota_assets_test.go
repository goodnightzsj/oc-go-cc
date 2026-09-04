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
	} {
		if !strings.Contains(string(page), marker) {
			t.Errorf("index.html missing quota marker %q", marker)
		}
	}
	for _, marker := range []string{
		"QuotaModule", "'/api/quota'", "refresh=1", "QUOTA_RING_LENGTH", "fmtCountdown",
		"tickCountdowns", "data-deadline", "rolling_5h", "percent_derived", "key_hint",
		"case 'quota':", "levelOf(", "deadlineOf(", "renderModelLimits", "model_limits", "fmtWeight",
	} {
		if !strings.Contains(string(app), marker) {
			t.Errorf("app.js missing quota marker %q", marker)
		}
	}
	// Both locales must define the quota strings; a missing key renders as the
	// raw key in the UI.
	for _, key := range []string{"'tab.quota'", "'quota.title'", "'quota.rolling5h'", "'quota.noKeyHint'", "'quota.resetsIn'",
		"'quota.modelLimits'", "'quota.modelAllowance'", "'quota.modelLimitsNote'", "'quota.model'",
		"'quota.mixNote'", "'quota.weight'", "'quota.modelLimitsUsed'"} {
		if strings.Count(string(app), key+":") < 2 {
			t.Errorf("translation key %s must exist in both en and zh", key)
		}
	}
	for _, marker := range []string{".quota-card", ".quota-gauge-fill", ".quota-figures", "level-crit",
		".quota-model-limits", ".quota-model-scroll", ".quota-model-allowance", ".quota-mix-note"} {
		if !strings.Contains(string(style), marker) {
			t.Errorf("style.css missing quota rule %q", marker)
		}
	}
}

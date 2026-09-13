package gui

import (
	"regexp"
	"strings"
	"testing"

	"github.com/routatic/proxy/internal/site"
)

func TestUIUXSemanticControls(t *testing.T) {
	data, err := assets.ReadFile("assets/index.html")
	if err != nil {
		t.Fatal(err)
	}
	page := string(data)
	headers := regexp.MustCompile(`(?s)<th\b[^>]*class="sortable[^>]*>.*?</th>`).FindAllString(page, -1)
	if len(headers) != 14 {
		t.Fatalf("sortable header count = %d, want 14", len(headers))
	}
	for _, header := range headers {
		if !strings.Contains(header, `<button type="button" class="sort-button">`) || !strings.Contains(header, `<span data-i18n=`) || !strings.Contains(header, `aria-sort=`) {
			t.Fatalf("sorting must use a labeled native button inside th: %s", header)
		}
	}
	// Only the platforms the dashboard offers: site_parity_test.go holds this
	// list to the registry, so hiding a platform is a registry change rather
	// than an edit here.
	for _, descriptor := range site.Visible() {
		if !strings.Contains(page, `data-settings-provider="`+descriptor.ID+`"`) {
			t.Errorf("missing independently expandable platform: %s", descriptor.ID)
		}
	}
	for _, name := range []string{"proxy", "autostart", "notify"} {
		control := regexp.MustCompile(`<input[^>]*id="toggle-` + name + `"[^>]*>`).FindString(page)
		if !strings.Contains(control, `aria-labelledby="setting-`+name+`-label"`) {
			t.Errorf("unnamed service control: %s", name)
		}
	}
	for _, id := range []string{"settings-provider-jump", "config-change-count", "fallback-status", "fallback-primary", "quota-commandcode", "quota-commandcode-accounts", "quota-open-settings"} {
		if !strings.Contains(page, `id="`+id+`"`) {
			t.Errorf("missing interaction target: %s", id)
		}
	}
	if strings.Contains(page, `id="fallback-chain" role="listbox"`) {
		t.Error("fallback rows contain buttons and must use native list semantics")
	}
	if !strings.Contains(page, `<details class="history-distribution">`) {
		t.Error("history distributions must not push request search below the fold")
	}
	if strings.Index(page, `id="history-search"`) > strings.Index(page, `id="history-model-breakdown"`) {
		t.Error("request search must precede distribution details in DOM order")
	}
}

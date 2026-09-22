package gui

import (
	"regexp"
	"strings"
	"testing"

	"github.com/routatic/proxy/internal/site"
)

// TestNarrowAnalyticsTableRenouncesTheNowrapFloor pins the fix for a horizontal
// scrollbar on the Model details panel. That table shares .analytics-table with
// the full-width period table, so the shared rules have to suit the narrow half
// -width column too: a name cell holds a word like
// "cline-pass/deepseek-v4.1-flash", and inherited nowrap kept the table at 643px
// inside a 548px column. Measured in the browser; the numbers are recorded in
// the CSS comment.
func TestNarrowAnalyticsTableRenouncesTheNowrapFloor(t *testing.T) {
	css := readStyleSheet(t)

	// The base rule must not force a floor. A min-width here applies to every
	// table using the class, including the one in a half-width grid column.
	base := cssSection(css, ".analytics-table {")
	if base == "" {
		t.Fatal(".analytics-table is not styled")
	}
	if strings.Contains(base, "min-width") {
		t.Errorf("a min-width on the shared class re-breaks the narrow table: %q", base)
	}

	// A name must be allowed to wrap; the figures stay nowrap.
	wrap := cssSection(css, ".analytics-table td code,")
	if wrap == "" {
		t.Fatal("the model cell must opt out of the inherited nowrap")
	}
	if !strings.Contains(wrap, "white-space: normal") {
		t.Errorf("the model cell must wrap, got %q", wrap)
	}
}

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

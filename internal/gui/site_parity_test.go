package gui

import (
	"regexp"
	"strings"
	"testing"

	"github.com/routatic/proxy/internal/site"
)

// The dashboard cannot fetch the platform list at runtime: the behaviour tests
// read index.html and app.js straight out of the embed FS and run them in a
// synchronous DOM shim, so the lists have to be present in the source. Go owns
// the answer and the frontend mirrors it, exactly as config.CostScenarioNames
// mirrors the router's scenario constants - and this is the assertion that
// keeps the mirror honest.
//
// Without it, hiding or renaming a platform means editing three places by hand
// and noticing only when someone reads the dashboard.
var (
	providerMapRe    = regexp.MustCompile(`(?s)const PROVIDERS = \{(.*?)\n\};`)
	hiddenMapRe      = regexp.MustCompile(`(?s)const HIDDEN_PLATFORMS = \{(.*?)\n\};`)
	providerEntryRe  = regexp.MustCompile(`'([a-z0-9-]+)':\s*\{\s*name:\s*'([^']*)'`)
	providerOrderRe  = regexp.MustCompile(`const PROVIDER_ORDER = \[\.\.\.Object\.keys\(PROVIDERS\), \.\.\.Object\.keys\(HIDDEN_PLATFORMS\)\];`)
	settingsBlockRe  = regexp.MustCompile(`data-settings-provider="([^"]+)"`)
	platformSelectID = []string{
		"overview-provider", "provider-filter", "perf-provider",
		"analytics-provider", "quota-provider", "settings-provider-jump",
		// The active-platform selector offers the same set; its extra
		// "not restricted" entry has an empty value and is filtered out.
		"cfg-active-site",
	}
)

// jsProviderLists reads the two frontend maps as ordered id/name pairs.
func jsProviderLists(t *testing.T) (visible, hidden []site.Descriptor) {
	t.Helper()
	app := readEmbeddedAsset(t, "assets/app.js")
	parse := func(re *regexp.Regexp, label string) []site.Descriptor {
		m := re.FindStringSubmatch(app)
		if m == nil {
			t.Fatalf("app.js has no %s map", label)
		}
		var out []site.Descriptor
		for _, entry := range providerEntryRe.FindAllStringSubmatch(m[1], -1) {
			out = append(out, site.Descriptor{ID: entry[1], DisplayName: entry[2]})
		}
		return out
	}
	if !providerOrderRe.MatchString(app) {
		t.Error("PROVIDER_ORDER no longer concatenates the two maps, so the full display order would be a third hand-maintained list")
	}
	return parse(providerMapRe, "PROVIDERS"), parse(hiddenMapRe, "HIDDEN_PLATFORMS")
}

func readEmbeddedAsset(t *testing.T, name string) string {
	t.Helper()
	data, err := assets.ReadFile(name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(data)
}

func TestFrontendPlatformListsMatchRegistry(t *testing.T) {
	visible, hidden := jsProviderLists(t)

	wantVisible := site.Visible()
	if len(visible) != len(wantVisible) {
		t.Fatalf("app.js offers %d platforms, registry says %d are visible", len(visible), len(wantVisible))
	}
	for i, want := range wantVisible {
		got := visible[i]
		if got.ID != want.ID || got.DisplayName != want.DisplayName {
			t.Errorf("app.js PROVIDERS[%d] = %s/%q, registry says %s/%q", i, got.ID, got.DisplayName, want.ID, want.DisplayName)
		}
	}

	wantHidden := site.Hidden()
	if len(hidden) != len(wantHidden) {
		t.Fatalf("app.js keeps %d hidden platforms, registry says %d", len(hidden), len(wantHidden))
	}
	for i, want := range wantHidden {
		got := hidden[i]
		if got.ID != want.ID || got.DisplayName != want.DisplayName {
			t.Errorf("app.js HIDDEN_PLATFORMS[%d] = %s/%q, registry says %s/%q", i, got.ID, got.DisplayName, want.ID, want.DisplayName)
		}
	}
}

// The selectors and the settings sections are static markup, so they are a
// third copy that has to agree with the registry too.
func TestMarkupOffersExactlyTheVisiblePlatforms(t *testing.T) {
	page := readEmbeddedAsset(t, "assets/index.html")
	want := make([]string, 0, len(site.Visible()))
	for _, d := range site.Visible() {
		want = append(want, d.ID)
	}

	for _, id := range platformSelectID {
		block := regexp.MustCompile(`(?s)<select[^>]*id="` + regexp.QuoteMeta(id) + `"[^>]*>(.*?)</select>`).FindStringSubmatch(page)
		if block == nil {
			t.Errorf("index.html has no selector %q", id)
			continue
		}
		var got []string
		for _, m := range regexp.MustCompile(`value="([^"]*)"`).FindAllStringSubmatch(block[1], -1) {
			if m[1] != "" { // the reset/placeholder option
				got = append(got, m[1])
			}
		}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("selector %q offers %v, want %v", id, got, want)
		}
	}

	for _, m := range settingsBlockRe.FindAllStringSubmatch(page, -1) {
		found := false
		for _, id := range want {
			if m[1] == id {
				found = true
			}
		}
		if !found {
			t.Errorf("index.html still has a settings section for %q, which the dashboard no longer offers", m[1])
		}
	}
}

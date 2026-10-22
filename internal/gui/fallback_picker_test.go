package gui

import (
	"strings"
	"testing"
)

// The fallback picker must be built from what this instance can actually send
// to. Sourcing it from the config file instead offered models whose platform
// has no credential - a chain that looks configured and can never serve a
// request, which is exactly what happened here: every configured fallback
// belonged to a platform the active site excludes.
func TestFallbackPickerUsesTheReachableListing(t *testing.T) {
	js := readEmbeddedAsset(t, "assets/app.js")

	if !strings.Contains(js, "/api/catalog/models") {
		t.Error("the picker must read the proxy's own listing; that is the only source that applies routing rules and credential presence")
	}
	// Grouped by platform: the platform is what an operator reasons about when
	// choosing a fallback, and a flat list of model ids hides it.
	if !strings.Contains(js, "optgroup") {
		t.Error("the picker must group options by platform")
	}
	if !strings.Contains(js, "platformLabel(provider)") {
		t.Error("groups must use the platform's display name, not its config key")
	}
	// A chain entry outside the active site is never used; the row has to say so
	// rather than implying protection that is not there.
	if !strings.Contains(js, "fallback.unreachable") {
		t.Error("an unreachable chain entry must be marked in the UI")
	}
}

// Every string the new UI uses has to exist in both languages, or the panel
// renders a key name at the operator.
func TestFallbackPickerStringsExistInBothLanguages(t *testing.T) {
	js := readEmbeddedAsset(t, "assets/app.js")
	for _, key := range []string{"fallback.addHint", "fallback.noReachableModels", "fallback.unreachable", "fallback.unreachableHint"} {
		if n := strings.Count(js, "'"+key+"':"); n < 2 {
			t.Errorf("%q is defined %d time(s); it needs an English and a Chinese entry", key, n)
		}
	}
}

func TestFallbackAddHintElementExists(t *testing.T) {
	html := readEmbeddedAsset(t, "assets/index.html")
	if !strings.Contains(html, `id="fallback-add-hint"`) {
		t.Error("the add-model hint element is missing; the picker would have nowhere to explain an empty list")
	}
}

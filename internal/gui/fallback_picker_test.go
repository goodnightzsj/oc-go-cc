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
	// choosing a fallback, and a flat list of model ids hides it. The grouping
	// is drawn by the popover now, not an <optgroup>.
	if !strings.Contains(js, "picker-group") {
		t.Error("the picker must group models by platform")
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

// The cross-platform switch has to exist and actually reach the config: an
// operator who cannot see it cannot learn the feature exists, and one that
// looks enabled but is never sent is worse.
func TestCrossPlatformToggleIsWired(t *testing.T) {
	html := readEmbeddedAsset(t, "assets/index.html")
	if !strings.Contains(html, `id="fallback-cross-platform"`) {
		t.Fatal("the cross-platform toggle is missing from the Fallback tab")
	}
	// Its handler must be defined. The page had none for a while: the markup
	// called FallbackModule.onCrossPlatformChange() and no such method existed,
	// so every toggle threw.
	if !strings.Contains(html, "FallbackModule.onCrossPlatformChange()") {
		t.Error("the toggle has no change handler")
	}
	js := readEmbeddedAsset(t, "assets/app.js")
	if !strings.Contains(js, "enable_cross_platform_fallback") {
		t.Error("the toggle is not written into the settings patch")
	}
	if !strings.Contains(js, "onCrossPlatformChange()") {
		t.Fatal("onCrossPlatformChange is called from the markup but never defined")
	}
	// There is no Save button, so a toggle that does not write through is a
	// toggle that does nothing at all.
	if !strings.Contains(js, `body.enable_cross_platform_fallback = cross`) {
		t.Error("toggling it alone would save nothing; the patch must carry it")
	}
	for _, key := range []string{"fallback.crossPlatform", "fallback.crossPlatformHint"} {
		if n := strings.Count(js, "'"+key+"':"); n < 2 {
			t.Errorf("%q needs an English and a Chinese entry, found %d", key, n)
		}
	}
}

// The chain page applies edits immediately. A Save button that no longer has a
// job is not merely clutter: it leaves the reader believing changes are pending
// when they are already live.
func TestFallbackPageHasNoSaveOrPreview(t *testing.T) {
	html := readEmbeddedAsset(t, "assets/index.html")
	for _, stale := range []string{
		`id="fallback-preview"`, "FallbackModule.save()", "FallbackModule.preview()",
	} {
		if strings.Contains(html, stale) {
			t.Errorf("index.html still contains %s", stale)
		}
	}
	js := readEmbeddedAsset(t, "assets/app.js")
	if strings.Contains(js, "originalChains") {
		t.Error("a pending-changes baseline is meaningless once edits write through")
	}
	// Every mutation path has to reach persist(), or the page draws a change the
	// proxy never received.
	for _, fn := range []string{"  chooseModel(key) {", "  moveModel(fromIndex, toIndex, direction) {",
		"  removeModel(index) {", "  onCrossPlatformChange() {"} {
		at := strings.Index(js, fn)
		if at < 0 {
			t.Fatalf("%s not found", strings.TrimSpace(fn))
		}
		rest := js[at:]
		end := strings.Index(rest[1:], "\n  },")
		if end < 0 {
			end = strings.Index(rest[1:], "\n  }")
		}
		if end < 0 {
			end = len(rest) - 1
		}
		if !strings.Contains(rest[:end], "persist(") {
			t.Errorf("%s does not write through", strings.TrimSpace(fn))
		}
	}
}

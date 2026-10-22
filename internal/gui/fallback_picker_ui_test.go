package gui

import (
	"strings"
	"testing"
)

// A native select cannot be searched and cannot show the facts that decide the
// choice. With hundreds of reachable models the picker has to be a popover with
// search, platform grouping and price context.
func TestFallbackPickerIsASearchablePopover(t *testing.T) {
	html := readEmbeddedAsset(t, "assets/index.html")
	for _, want := range []string{
		`id="fallback-add-trigger"`, // the button that opens it
		`id="fallback-add-popover"`, // the popover itself
		`id="fallback-add-search"`,  // the search box
		`id="fallback-add-list"`,    // the grouped list
		`role="listbox"`,            // announced as a list of options
		`aria-haspopup="listbox"`,   // and as a popup from the trigger
	} {
		if !strings.Contains(html, want) {
			t.Errorf("index.html is missing %s", want)
		}
	}
	if strings.Contains(html, `id="fallback-add-model"`) {
		t.Error("the native select is still present; it cannot be searched")
	}
}

func TestFallbackPickerBehaviourExists(t *testing.T) {
	js := readEmbeddedAsset(t, "assets/app.js")
	for _, fn := range []string{
		"renderPickerList", // filtered, grouped drawing
		"filterPicker",     // search wiring
		"chooseModel",      // selection
		"onPickerKeydown",  // arrows / Enter / Escape
		"priceTier",        // relative price bucketing
		"togglePicker",     // open/close
	} {
		if !strings.Contains(js, fn+"(") {
			t.Errorf("app.js has no %s", fn)
		}
	}
	// A picker that stays open on an outside click reads as stuck.
	if !strings.Contains(js, "pointerdown") {
		t.Error("the popover has no outside-click dismissal")
	}
	// Price is bucketed against the cheapest visible model, not a fixed number:
	// absolute rates differ by two orders of magnitude across platforms.
	// The class is assembled from the tier, so the literal lives in the CSS.
	if !strings.Contains(js, "picker-price-${tier}") {
		t.Error("rows do not carry a price tier class")
	}
	if !strings.Contains(js, "cheapest") {
		t.Error("price highlighting is not relative to the visible set")
	}
	css := readEmbeddedAsset(t, "assets/style.css")
	for _, want := range []string{".picker-price-low", ".picker-price-mid", ".picker-price-high"} {
		if !strings.Contains(css, want) {
			t.Errorf("style.css is missing %s", want)
		}
	}
}

func TestFallbackPickerStringsAreBilingual(t *testing.T) {
	js := readEmbeddedAsset(t, "assets/app.js")
	for _, key := range []string{
		"fallback.noOptions", "fallback.noMatches", "fallback.currentSite", "fallback.priceHint",
	} {
		if n := strings.Count(js, "'"+key+"':"); n < 2 {
			t.Errorf("%q needs an English and a Chinese entry, found %d", key, n)
		}
	}
}

// The search box needs a label that is not merely a visual placeholder.
func TestFallbackPickerSearchIsLabelled(t *testing.T) {
	html := readEmbeddedAsset(t, "assets/index.html")
	start := strings.Index(html, `id="fallback-add-search"`)
	if start < 0 {
		t.Fatal("search box missing")
	}
	window := html[start:min(start+400, len(html))]
	if !strings.Contains(window, `aria-label=`) {
		t.Error("the search input has no accessible name")
	}
}

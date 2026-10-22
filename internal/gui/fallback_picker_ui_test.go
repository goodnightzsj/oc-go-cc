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

// The platform filter is what keeps the model list short. It has to exist, and
// the candidate list has to actually be narrowed by it - a control that only
// changes its own label would be worse than none.
func TestFallbackPlatformFilterNarrowsTheModelList(t *testing.T) {
	html := readEmbeddedAsset(t, "assets/index.html")
	if !strings.Contains(html, `id="fallback-platform-filter"`) {
		t.Fatal("the platform filter is missing")
	}
	js := readEmbeddedAsset(t, "assets/app.js")
	if !strings.Contains(js, "populatePlatformFilter") || !strings.Contains(js, "onPlatformFilterChange") {
		t.Error("the platform filter is not wired")
	}
	// The narrowing has to happen in the candidate computation, not in drawing.
	if !strings.Contains(js, "this.platformFilter || ''") {
		t.Error("populateAddSelect does not read the selected platform")
	}
}

// A single-option scenario dropdown asks the operator a question with one
// answer. It is hidden unless the config really does have several.
func TestScenarioDropdownHidesWhenThereIsOnlyOne(t *testing.T) {
	html := readEmbeddedAsset(t, "assets/index.html")
	if !strings.Contains(html, `id="fallback-scenario-field"`) {
		t.Fatal("the scenario field wrapper is missing; hiding needs a container")
	}
	js := readEmbeddedAsset(t, "assets/app.js")
	if !strings.Contains(js, "scenarioField.hidden = scenarioKeys.length <= 1") {
		t.Error("the scenario dropdown is shown even when it has one option")
	}
}

// An unpriced model must not render as $0 - zero is a real rate for a free
// model, and conflating the two makes a platform with no price table look like
// the cheapest thing on the page.
func TestUnpricedModelsShowADashNotZero(t *testing.T) {
	js := readEmbeddedAsset(t, "assets/app.js")
	if !strings.Contains(js, "picker-price-none") {
		t.Error("unpriced rows have no distinct presentation")
	}
	if !strings.Contains(js, "fallback.priceUnknown") {
		t.Error("unpriced rows do not explain themselves")
	}
	for _, key := range []string{"fallback.priceUnknown", "fallback.platformFilter"} {
		if n := strings.Count(js, "'"+key+"':"); n < 2 {
			t.Errorf("%q needs an English and a Chinese entry, found %d", key, n)
		}
	}
}

// Choosing a platform then opening the picker must list that platform's models.
// The failure this guards is silent: the list renders empty and reads as "this
// platform has no models" rather than "the filter is stale".
func TestPlatformFilterThenPickerListsThatPlatformsModels(t *testing.T) {
	runPlatformBehavior(t, platformBehaviorDOMScript+`
async function checks() {
  // Shaped exactly as /api/catalog/models returns it: the field is named id,
  // not model_id. A fixture that pre-mapped it hid a crash for a whole round.
  const listing = [
    {id:'deepseek-v4.1-flash', provider:'cline-pass', input_per_m:0.15, output_per_m:0.6, context_window:1000000},
    {id:'kimi-k3', provider:'cline-pass', input_per_m:3, output_per_m:15, context_window:1000000},
    {id:'deepseek/deepseek-v4-pro', provider:'commandcode', input_per_m:0.66, output_per_m:1.98, context_window:1000000},
  ];
  // Go through the real merge, not a hand-built array: that is the code that
  // dropped the field, and a fixture that maps it by hand cannot see it.
  FallbackModule.availableModels = [...FallbackModule.mergeReachable(listing).values()];
  assert.equal(FallbackModule.availableModels[0].model_id, 'deepseek-v4.1-flash', 'merge must set model_id');
  FallbackModule.chains = {default:[]};
  FallbackModule.currentScenario = 'default';
  FallbackModule.platformFilter = '';
  FallbackModule.populateAddSelect();
  FallbackModule.togglePicker(true);
  FallbackModule.renderPickerList('');
  const count = () => (document.getElementById('fallback-add-list').innerHTML.match(/class="picker-row"/g) || []).length;
  assert.equal(count(), 3, 'all platforms before filtering');
  // The comparator reads model_id; a record without it throws and the whole
  // list fails to draw, which surfaces to the reader as "no matching models".
  assert.ok(document.getElementById('fallback-add-list').innerHTML.includes('picker-row'),
    'the list must render rows, not fail on a missing field');

  // Select cline-pass the way the dropdown does, then redraw.
  FallbackModule.platformFilter = 'cline-pass';
  FallbackModule.populateAddSelect();
  FallbackModule.renderPickerList('');
  assert.equal(count(), 2, 'only the chosen platform remains');

  // Each row must carry its price and context window; rebuilding the record as
  // {provider, model_id} silently dropped both.
  const rows = document.getElementById('fallback-add-list').innerHTML;
  assert.ok(rows.includes('$0.15'), 'priced rows must show the rate');
  assert.ok(!rows.includes('picker-price-none'), 'a priced model must not render as unpriced');
}
`)
}

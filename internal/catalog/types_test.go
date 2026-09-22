package catalog

import (
	"encoding/json"
	"testing"
)

// TestCatalogParsesModelsDevPricingField. models.dev publishes per-model rates
// under "cost"; this package's Rates type uses "input"/"output" too, but the
// field name on Model is models.dev's own.
//
// This is a regression guard for a live bug: Model.Rates carried the tag
// `rates`, models.dev sends `cost`, and the mismatch was silent. Rates stayed
// nil, the model was stored with no price, and its cost read as unknown - which
// looks the same as a platform that publishes no prices, so nothing caught it.
// Three platforms masked it with hand-written seed tables; any platform priced
// only from the catalog got no prices at all.
//
// The fixture is a verbatim slice of the real payload (an OpenRouter model with
// pricing, and one without), so a rename upstream fails here rather than in the
// dashboard months later.
func TestCatalogParsesModelsDevPricingField(t *testing.T) {
	body := []byte(`{"models":{
		"deepseek/deepseek-v4.1-flash":{"id":"deepseek/deepseek-v4.1-flash","name":"DeepSeek V4.1 Flash",
			"cost":{"input":0.15,"output":0.6,"cache_read":0.003}},
		"aion-labs/aion-2.0":{"id":"aion-labs/aion-2.0","name":"Aion-2.0"}
	}}`)
	var cat Catalog
	if err := json.Unmarshal(body, &cat); err != nil {
		t.Fatalf("parse: %v", err)
	}

	priced := cat.Models["deepseek/deepseek-v4.1-flash"]
	if got := priced.Rates(); got == nil {
		t.Fatal("a model the catalog prices parsed with no rates; the models.dev field name has changed")
	} else if got.Input != 0.15 || got.Output != 0.6 {
		t.Errorf("rates = %+v, want input 0.15 output 0.6", got)
	}
	if priced.CostInputPerM() != 0.15 || priced.CostOutputPerM() != 0.6 {
		t.Errorf("accessors disagree with Rates(): %v / %v", priced.CostInputPerM(), priced.CostOutputPerM())
	}

	// A model with no pricing must stay nil rather than becoming zero: free and
	// unpriced are different claims, and the cost path treats them differently.
	if got := cat.Models["aion-labs/aion-2.0"].Rates(); got != nil {
		t.Errorf("an unpriced model reported rates: %+v", got)
	}
}

// TestPromptTiersApplyStrictlyAboveTheirThreshold. models.dev words the
// condition as "applies when total prompt tokens are strictly greater than this
// threshold", so a request of exactly the threshold bills at the base rate.
// Getting the direction wrong is a factor-of-two error on exactly the largest
// requests - the ones a tier exists to price - and the boundary is the only
// place the two readings differ.
func TestPromptTiersApplyStrictlyAboveTheirThreshold(t *testing.T) {
	m := Model{ID: "x", Cost: &Cost{
		Input: 1, Output: 2,
		Overrides: []CostOverride{
			{MinPromptTokens: 200000, Prompt: 2, Completion: 4},
		},
	}}

	for _, tc := range []struct {
		prompt     int64
		wantInput  float64
		wantOutput float64
		why        string
	}{
		{199999, 1, 2, "below the threshold"},
		{200000, 1, 2, "exactly at the threshold is NOT above it"},
		{200001, 2, 4, "one token above the threshold"},
	} {
		got := m.RatesAt(tc.prompt)
		if got == nil || got.Input != tc.wantInput || got.Output != tc.wantOutput {
			t.Errorf("RatesAt(%d) = %+v, want input %v output %v (%s)", tc.prompt, got, tc.wantInput, tc.wantOutput, tc.why)
		}
	}
}

// TestHighestApplicableTierWins. A model may publish several bands, and the one
// that applies is the highest threshold the prompt exceeds - not the first one
// listed, which is whatever order the payload happened to use.
func TestHighestApplicableTierWins(t *testing.T) {
	m := Model{ID: "x", Cost: &Cost{
		Input: 1, Output: 2,
		Overrides: []CostOverride{
			{MinPromptTokens: 400000, Prompt: 8, Completion: 16},
			{MinPromptTokens: 200000, Prompt: 2, Completion: 4},
		},
	}}
	for _, tc := range []struct {
		prompt int64
		want   float64
	}{
		{199999, 1}, {200001, 2}, {400001, 8},
	} {
		if got := m.RatesAt(tc.prompt); got == nil || got.Input != tc.want {
			t.Errorf("RatesAt(%d) input = %v, want %v", tc.prompt, got, tc.want)
		}
	}
}

// TestTimeWindowOverridesAreNotTreatedAsTiers is the guard against the worst
// available outcome. An override whose condition is unread becomes an
// unconditional one, so a time-window band - which is only in force for part of
// the day - would otherwise price every request. Two OpenRouter models publish
// exactly this shape today.
func TestTimeWindowOverridesAreNotTreatedAsTiers(t *testing.T) {
	midnight, noon := 0, 1600
	m := Model{ID: "tencent/hy3", Cost: &Cost{
		Input: 0.000000132, Output: 0.000000528,
		Overrides: []CostOverride{
			{UTCStart: &midnight, UTCEnd: &noon, Prompt: 0.000000132, Completion: 0.000000528},
			{UTCStart: &noon, UTCEnd: &midnight, Prompt: 0.0000000825, Completion: 0.00000033},
			{UTCDays: []string{"saturday", "sunday"}, Prompt: 0.00000015, Completion: 0.0000006},
		},
	}}
	if tiers := m.PromptTiers(); len(tiers) != 0 {
		t.Errorf("time-window overrides were read as prompt tiers: %+v", tiers)
	}
	// Every prompt size must bill at the listed base rate, since no window can
	// be evaluated without a clock.
	for _, p := range []int64{0, 1, 1_000_000} {
		if got := m.RatesAt(p); got == nil || got.Input != 0.000000132 {
			t.Errorf("RatesAt(%d) = %+v, want the base rate", p, got)
		}
	}
}

// TestTierFallbackIsPerField. An override that changes only the completion
// price must leave input alone; a zero here means "not stated", not "free".
func TestTierFallbackIsPerField(t *testing.T) {
	m := Model{ID: "x", Cost: &Cost{
		Input: 1, Output: 2,
		Overrides: []CostOverride{{MinPromptTokens: 100, Prompt: 5}},
	}}
	got := m.RatesAt(200)
	if got.Input != 5 || got.Output != 2 {
		t.Errorf("RatesAt = %+v, want input 5 (from the tier) and output 2 (base)", got)
	}
}

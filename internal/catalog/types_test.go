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

package storage

import (
	"testing"
)

// The gateway's ids carry a realm prefix and its rates live in a table keyed by
// the bare family name. Both halves have to line up or the cost column is
// empty, which is what shipped: the table existed but every lookup missed.
func TestWorkBuddyRealmPrefixedModelsArePriced(t *testing.T) {
	for _, tc := range []struct {
		model          string
		wantInputPerM  float64
	}{
		{"global:deepseek-v4.1-flash", 0.15},
		{"global:deepseek-v4-pro", 0.66},
		{"global:glm-5.3", 2.01},
		{"global:glm-5.3-flash", 0.35},
		{"global:kimi-k3", 4.0},
		{"global:gemini-3.8-flash", 1.5},
	} {
		got, _, _, _, ok := PriceForProviderModel("workbuddy", modelNameForPricing(tc.model), 0)
		if !ok {
			t.Errorf("%s: no rate matched", tc.model)
			continue
		}
		if got != tc.wantInputPerM {
			t.Errorf("%s input rate = %v, want %v", tc.model, got, tc.wantInputPerM)
		}
	}
}

// The longest rule wins, so a flash variant is not priced at its parent's rate.
func TestWorkBuddyLongestMatchWins(t *testing.T) {
	glm, _, _, _, _ := PriceForProviderModel("workbuddy", "glm-5.3", 0)
	flash, _, _, _, _ := PriceForProviderModel("workbuddy", "glm-5.3-flash", 0)
	if glm == flash {
		t.Errorf("glm-5.3 and glm-5.3-flash share rate %v; the longer rule must win", glm)
	}
}

// A routing alias names no model, so there is no family to borrow a price
// from. Reporting unknown is the honest answer; zero would read as free.
func TestWorkBuddyAliasesAreUnpriced(t *testing.T) {
	for _, alias := range []string{"default-model", "fast-model", "balanced-model", "primary-model"} {
		if _, _, _, _, ok := PriceForProviderModel("workbuddy", alias, 0); ok {
			t.Errorf("%s was priced; it names no model", alias)
		}
	}
}

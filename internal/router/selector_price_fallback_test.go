package router

import (
	"testing"

	"github.com/routatic/proxy/internal/catalog"
	"github.com/routatic/proxy/internal/config"
)

// A model the mirror has not priced yet must still be selectable when the
// platform's own table prices it. Reading the catalog alone excluded exactly
// those models, silently - they simply never appeared as candidates.
func TestCandidateIsPricedFromTheTableWhenTheCatalogHasNoRate(t *testing.T) {
	// cline-pass/deepseek-v4.1-flash is priced in the embedded table; a catalog
	// entry with no Cost models the mirror's silence about the same model.
	model := catalog.Model{
		ID:    "cline-pass/deepseek-v4.1-flash",
		Name:  "DeepSeek V4.1 Flash",
		Limit: &catalog.Limit{Context: 1000000, Output: 384000},
	}
	if _, _, priced := catalogRate(model); priced {
		t.Fatal("the fixture must start unpriced, or it does not test the fallback")
	}

	cfg := &config.Config{
		ClinePass: config.ClinePassConfig{APIKeys: []string{"synthetic"}},
	}
	cat := &catalog.IndexedCatalog{
		Catalog: catalog.Catalog{
			Providers: map[string]catalog.Provider{"cline-pass": {Name: "cline-pass"}},
			Models:    map[string]catalog.Model{"cline-pass/deepseek-v4.1-flash": model},
		},
	}
	s := NewSelector(cat, cfg)
	candidates := s.resolveCandidates(config.CostScenario{}, ScenarioConstraints{Context: 1000})
	if len(candidates) == 0 {
		t.Fatal("a model priced only by its platform's table was dropped from the candidates")
	}
	if candidates[0].CostInputPerM == 0 && candidates[0].CostOutputPerM == 0 {
		t.Errorf("the table's rates were not carried onto the candidate: %+v", candidates[0])
	}
}

// A catalog rate always wins: it is the mirror's own figure for that model.
func TestCatalogRateWinsOverTheTable(t *testing.T) {
	model := catalog.Model{
		ID:    "cline-pass/deepseek-v4.1-flash",
		Cost:  &catalog.Cost{Input: 9, Output: 11},
		Limit: &catalog.Limit{Context: 1000000, Output: 1000},
	}
	in, out, priced := catalogRate(model)
	if !priced || in != 9 || out != 11 {
		t.Errorf("catalogRate = (%v, %v, %v), want its own 9/11", in, out, priced)
	}
}

// An explicit zero is a price, not an absence: a free model must remain a
// candidate and must be able to win the cheapest-model sort.
func TestZeroRateIsPriced(t *testing.T) {
	model := catalog.Model{ID: "x/y", Cost: &catalog.Cost{Input: 0, Output: 0}}
	if _, _, priced := catalogRate(model); !priced {
		t.Error("a model with a cost object reporting zero was treated as unpriced")
	}
}

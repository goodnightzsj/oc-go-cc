package router

import (
	"errors"
	"testing"

	"github.com/routatic/proxy/internal/catalog"
	"github.com/routatic/proxy/internal/config"
)

func TestSelectCheapestDistinguishesUnknownFromFree(t *testing.T) {
	cat := selectorTestCatalog(t)
	cat.Models = map[string]catalog.Model{
		"opencode-go/unknown": {ID: "unknown"},
		"opencode-go/known":   {ID: "known", Rates: &catalog.Rates{Input: 1, Output: 2}},
	}
	selector := NewSelector(cat, &config.Config{OpenCodeGo: config.OpenCodeGoConfig{APIKey: "synthetic"}})
	model, err := selector.SelectCheapest("default", ScenarioConstraints{})
	if err != nil || model.ModelID != "known" {
		t.Fatalf("selection = %s, %v; want known", model.ModelID, err)
	}
	delete(cat.Models, "opencode-go/known")
	if _, err := selector.SelectCheapest("default", ScenarioConstraints{}); !errors.Is(err, ErrNoCandidateModel) {
		t.Fatalf("unknown-only selection error = %v", err)
	}
	cat.Models["opencode-go/free"] = catalog.Model{ID: "free", Rates: &catalog.Rates{}}
	model, err = selector.SelectCheapest("default", ScenarioConstraints{})
	if err != nil || model.ModelID != "free" {
		t.Fatalf("selection = %s, %v; want free", model.ModelID, err)
	}
}

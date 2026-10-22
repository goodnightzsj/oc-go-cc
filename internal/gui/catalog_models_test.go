package gui

import (
	"testing"

	"github.com/routatic/proxy/internal/catalog"
)

// A platform serves ids the catalog files under a different provider. ClinePass
// is a reseller: its gateway serves `cline-pass/deepseek-v4.1-flash-1017`, and
// the catalog carries that model only as `deepseek/deepseek-v4.1-flash-1017`.
// Resolving by exact id alone left that row with an id and a price but no
// context window and no capability flags - which reads as missing data rather
// than as a model the catalog does not file under that name.
func TestCatalogModelForFallsBackToTheBareName(t *testing.T) {
	cat := &catalog.IndexedCatalog{Catalog: catalog.Catalog{Models: map[string]catalog.Model{
		"deepseek/deepseek-v4.1-flash-1017": {
			ID: "deepseek/deepseek-v4.1-flash-1017", Name: "DeepSeek V4.1 Flash 1017",
			ToolCall: true, Reasoning: true,
			Limit: &catalog.Limit{Context: 1_000_000, Output: 384_000},
		},
	}}}

	model, ok := catalogModelFor(cat, "cline-pass/deepseek-v4.1-flash-1017", "cline-pass")
	if !ok {
		t.Fatal("a served id the catalog files under another provider must still resolve")
	}
	if model.Limit == nil || model.Limit.Context != 1_000_000 {
		t.Errorf("context window = %v, want the catalog's 1000000", model.Limit)
	}
	if !model.ToolCall || !model.Reasoning {
		t.Errorf("capability flags lost: tool=%v reasoning=%v", model.ToolCall, model.Reasoning)
	}
	if model.DisplayName() != "DeepSeek V4.1 Flash 1017" {
		t.Errorf("display name = %q, want the catalog's", model.DisplayName())
	}
}

// The fallback must never shadow a provider-specific entry: an exact id or an
// entry under the model's own provider wins over a same-named model elsewhere.
func TestCatalogModelForPrefersTheProviderEntry(t *testing.T) {
	cat := &catalog.IndexedCatalog{Catalog: catalog.Catalog{Models: map[string]catalog.Model{
		"cline-pass/shared": {ID: "cline-pass/shared", Name: "ClinePass variant",
			Limit: &catalog.Limit{Context: 111}},
		"other/shared": {ID: "other/shared", Name: "Other variant",
			Limit: &catalog.Limit{Context: 999}},
		"cline-pass/exact": {ID: "cline-pass/exact", Name: "Exact"},
	}}}

	if got, _ := catalogModelFor(cat, "cline-pass/shared", "cline-pass"); got.Name != "ClinePass variant" {
		t.Errorf("resolved %q, want the provider's own entry", got.Name)
	}
	if got, _ := catalogModelFor(cat, "cline-pass/exact", "cline-pass"); got.Name != "Exact" {
		t.Errorf("an exact id must win, got %q", got.Name)
	}
}

// An id no provider describes stays unresolved rather than borrowing an
// unrelated model's detail.
func TestCatalogModelForReportsAnUnknownModel(t *testing.T) {
	cat := &catalog.IndexedCatalog{Catalog: catalog.Catalog{Models: map[string]catalog.Model{
		"deepseek/deepseek-v4.1-flash": {ID: "deepseek/deepseek-v4.1-flash"},
	}}}
	if _, ok := catalogModelFor(cat, "cline-pass/some-unlisted-model", "cline-pass"); ok {
		t.Error("an unlisted model must not resolve to a different one")
	}
}

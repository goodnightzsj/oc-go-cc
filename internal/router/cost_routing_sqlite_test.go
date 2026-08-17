package router

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/routatic/proxy/internal/catalog"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/storage"
)

// TestCostScenarioNamesMatchRouterScenarios guards config.CostScenarioNames
// against drift from the Scenario constants. config cannot import router, so the
// list is duplicated there; this test is the only thing keeping them in sync.
//
// "override" is intentionally excluded: resolveRequestedModel returns before
// scenario routing, so the cost selector is never consulted for it.
func TestCostScenarioNamesMatchRouterScenarios(t *testing.T) {
	routerScenarios := []Scenario{
		ScenarioDefault,
		ScenarioBackground,
		ScenarioThink,
		ScenarioComplex,
		ScenarioLongContext,
		ScenarioFast,
		ScenarioVision,
		ScenarioVisionComplex,
		ScenarioVisionLongContext,
	}

	if len(config.CostScenarioNames) != len(routerScenarios) {
		t.Fatalf("CostScenarioNames has %d entries, router has %d reachable scenarios",
			len(config.CostScenarioNames), len(routerScenarios))
	}

	inConfig := make(map[string]bool, len(config.CostScenarioNames))
	for _, name := range config.CostScenarioNames {
		inConfig[name] = true
	}
	for _, s := range routerScenarios {
		if !inConfig[string(s)] {
			t.Errorf("scenario %q is reachable but missing from config.CostScenarioNames", s)
		}
	}
}

// newCatalogDB builds a real SQLite database populated from the selector
// fixture, which is exactly how production gets its catalog: models and
// providers imported into SQLite, with no scenarios anywhere.
func newCatalogDB(t *testing.T) *storage.Database {
	t.Helper()

	db, err := storage.Open(storage.Config{DatabasePath: filepath.Join(t.TempDir(), "catalog.db")})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	_, models, err := catalog.ImportFromJSON(context.Background(), db, filepath.Join("testdata", "selector_catalog.json"))
	if err != nil {
		t.Fatalf("import catalog into SQLite: %v", err)
	}
	if models == 0 {
		t.Fatal("catalog import brought in no models")
	}
	return db
}

// TestCostBasedRouting_SQLiteCatalogSelectsCheapest is the end-to-end guard for
// the bug this feature had: production builds the router with a database, that
// catalog carries no scenarios, and cost routing silently degraded to the
// legacy scenario model on every request. Routing through the SQLite path must
// now actually pick the cheapest catalog model.
func TestCostBasedRouting_SQLiteCatalogSelectsCheapest(t *testing.T) {
	db := newCatalogDB(t)

	cfg := &config.Config{
		APIKey: "global-key",
		Models: map[string]config.ModelConfig{
			"default": {Provider: "opencode-go", ModelID: "legacy-default"},
		},
		CostRouting: &config.CostRoutingConfig{Enabled: true},
	}
	r := NewModelRouterWithDB(config.NewAtomicConfig(cfg, "/tmp/test-config.json"), db)

	result, err := r.Route([]MessageContent{{Role: "user", Content: "Hello"}}, 100, "")
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}

	if result.Primary.ModelID == "legacy-default" {
		t.Fatal("cost routing was skipped: primary is still the legacy scenario model")
	}
	if result.Primary.ModelID != "cheap-no-tools" {
		t.Errorf("primary = %q, want the cheapest catalog model cheap-no-tools", result.Primary.ModelID)
	}
	if result.Primary.Provider != "opencode-go" {
		t.Errorf("primary provider = %q, want opencode-go", result.Primary.Provider)
	}
}

// A scenario policy nothing can satisfy must fall back to the legacy model
// rather than failing the request.
func TestCostBasedRouting_SQLiteCatalogFallsBackWhenUnsatisfiable(t *testing.T) {
	db := newCatalogDB(t)

	cfg := &config.Config{
		APIKey: "global-key",
		Models: map[string]config.ModelConfig{
			"default": {Provider: "opencode-go", ModelID: "legacy-default"},
		},
		CostRouting: &config.CostRoutingConfig{
			Enabled: true,
			Scenarios: map[string]config.CostScenario{
				"default": {MinContextWindow: 99_000_000},
			},
		},
	}
	r := NewModelRouterWithDB(config.NewAtomicConfig(cfg, "/tmp/test-config.json"), db)

	result, err := r.Route([]MessageContent{{Role: "user", Content: "Hello"}}, 100, "")
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}
	if result.Primary.ModelID != "legacy-default" {
		t.Errorf("primary = %q, want the legacy fallback legacy-default", result.Primary.ModelID)
	}
}

// Cost routing must stay off unless it is enabled, even with a usable catalog.
func TestCostBasedRouting_SQLiteCatalogDisabledKeepsLegacy(t *testing.T) {
	db := newCatalogDB(t)

	cfg := &config.Config{
		APIKey: "global-key",
		Models: map[string]config.ModelConfig{
			"default": {Provider: "opencode-go", ModelID: "legacy-default"},
		},
	}
	r := NewModelRouterWithDB(config.NewAtomicConfig(cfg, "/tmp/test-config.json"), db)

	result, err := r.Route([]MessageContent{{Role: "user", Content: "Hello"}}, 100, "")
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}
	if result.Primary.ModelID != "legacy-default" {
		t.Errorf("primary = %q, want legacy-default when cost routing is disabled", result.Primary.ModelID)
	}
}

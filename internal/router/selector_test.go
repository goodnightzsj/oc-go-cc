package router

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/routatic/proxy/internal/catalog"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/site"
)

// selectorTestCatalog loads the shared fixture catalog used by selector tests.
func selectorTestCatalog(t *testing.T) *catalog.IndexedCatalog {
	t.Helper()
	cat, err := catalog.Load(filepath.Join("testdata", "selector_catalog.json"))
	if err != nil {
		t.Fatalf("load selector catalog: %v", err)
	}
	return cat
}

// selectorTestScenarios mirrors the scenario policies these tests previously
// read from the catalog fixture, now that cost_routing.scenarios in the config
// is the only source. Scenario names are validated at config load time
// (see config.validateCostScenarios); the selector itself accepts any key, so
// these synthetic names exercise the matching logic directly.
func selectorTestScenarios() map[string]config.CostScenario {
	yes := true
	return map[string]config.CostScenario{
		"default":            {},
		"fast":               {},
		"tools_required":     {RequiresTools: &yes},
		"vision_required":    {RequiresVision: &yes},
		"reasoning_required": {RequiresReasoning: &yes},
		"long_context":       {MinContextWindow: 500000},
		"preferred_only":     {PreferredProviders: []string{"openrouter"}},
		"complex":            {RequiresTools: &yes, MinContextWindow: 500000},
		"vision":             {RequiresVision: &yes},
		"vision_complex":     {RequiresVision: &yes, RequiresTools: &yes},
	}
}

// withSelectorScenarios attaches the shared scenario policies to cfg while
// preserving any cost_routing settings the caller already made.
func withSelectorScenarios(cfg *config.Config) *config.Config {
	if cfg == nil {
		cfg = &config.Config{}
	}
	if cfg.CostRouting == nil {
		cfg.CostRouting = &config.CostRoutingConfig{}
	}
	if cfg.CostRouting.Scenarios == nil {
		cfg.CostRouting.Scenarios = selectorTestScenarios()
	}
	return cfg
}

func TestSelectCheapest_SelectsCheapestModel(t *testing.T) {
	cfg := &config.Config{
		OpenCodeGo: config.OpenCodeGoConfig{APIKey: "go-key"},
		OpenRouter: config.OpenRouterConfig{APIKey: "or-key"},
	}
	selector := NewSelector(selectorTestCatalog(t), withSelectorScenarios(cfg))

	got, err := selector.SelectCheapest("default", ScenarioConstraints{})
	if err != nil {
		t.Fatalf("SelectCheapest returned error: %v", err)
	}

	want := "cheap-no-tools"
	if got.ModelID != want {
		t.Errorf("SelectCheapest(default) = %q, want %q", got.ModelID, want)
	}
	if got.Provider != "opencode-go" {
		t.Errorf("SelectCheapest(default) provider = %q, want %q", got.Provider, "opencode-go")
	}
	if got.CostInputPerM+got.CostOutputPerM != 2.0 {
		t.Errorf("SelectCheapest(default) total cost = %v, want 2.0", got.CostInputPerM+got.CostOutputPerM)
	}
}

func TestSelectCheapestRespectsActiveSite(t *testing.T) {
	for _, tc := range []struct {
		name     string
		active   string
		global   []string
		scenario []string
		want     string
	}{
		{"unrestricted", "", nil, nil, site.OpenCodeGo},
		{"active OpenRouter", site.OpenRouter, nil, nil, site.OpenRouter},
		{"active CommandCode", site.CommandCode, nil, nil, site.CommandCode},
		{"global preference intersects", site.OpenRouter, []string{site.OpenCodeGo}, nil, ""},
		{"scenario preference intersects", site.OpenRouter, nil, []string{site.OpenCodeGo}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cat := selectorTestCatalog(t)
			cat.Providers[site.CommandCode] = catalog.Provider{Name: site.CommandCode}
			cat.Models["commandcode/published"] = catalog.Model{ID: "published", Cost: &catalog.Cost{Input: 10, Output: 10}}
			cfg := &config.Config{
				ActiveSite: tc.active, APIKey: "synthetic-global",
				CommandCode: config.CommandCodeConfig{APIKey: "synthetic-commandcode"},
				CostRouting: &config.CostRoutingConfig{
					PreferProviders: tc.global,
					Scenarios:       map[string]config.CostScenario{"default": {PreferredProviders: tc.scenario}},
				},
			}
			got, err := NewSelector(cat, cfg).SelectCheapest("default", ScenarioConstraints{})
			if tc.want == "" {
				if !errors.Is(err, ErrNoCandidateModel) {
					t.Fatalf("conflicting scope/preferences selected %+v: %v", got, err)
				}
				return
			}
			if err != nil || got.Provider != tc.want {
				t.Fatalf("provider=%q, want %q; err=%v", got.Provider, tc.want, err)
			}
		})
	}
}

func TestSelectCheapestDoesNotEnableKeylessCommandCode(t *testing.T) {
	cat := selectorTestCatalog(t)
	cat.Providers[site.CommandCode] = catalog.Provider{Name: site.CommandCode}
	cat.Models["commandcode/cheap"] = catalog.Model{ID: "cheap", Cost: &catalog.Cost{Input: 0.01, Output: 0.01}}
	cfg := &config.Config{APIKey: "synthetic-global"}
	got, err := NewSelector(cat, cfg).SelectCheapest("default", ScenarioConstraints{})
	if err != nil || got.Provider == site.CommandCode {
		t.Fatalf("keyless CommandCode must not be selected: %+v err=%v", got, err)
	}
	cfg.ActiveSite = site.CommandCode
	if _, err := NewSelector(cat, cfg).SelectCheapest("default", ScenarioConstraints{}); !errors.Is(err, ErrNoCandidateModel) {
		t.Fatalf("keyless active site must have no candidates: %v", err)
	}
}

func TestCostRoutingRespectsActiveSiteForBothRequestModes(t *testing.T) {
	cfg := &config.Config{
		APIKey: "synthetic-global", ActiveSite: site.OpenRouter,
		Models:      map[string]config.ModelConfig{"default": {Provider: site.OpenRouter, ModelID: "configured"}},
		CostRouting: &config.CostRoutingConfig{Enabled: true},
	}
	r := NewModelRouterWithCatalog(config.NewAtomicConfig(cfg, ""), filepath.Join("testdata", "selector_catalog.json"))
	for name, route := range map[string]func([]MessageContent, int, string) (RouteResult, error){
		"non-streaming": r.Route, "streaming": r.RouteForStreaming,
	} {
		t.Run(name, func(t *testing.T) {
			got, err := route([]MessageContent{{Role: "user", Content: "hi"}}, 100, "")
			if err != nil || got.Primary.Provider != site.OpenRouter {
				t.Fatalf("cost routing left the active site: %+v err=%v", got, err)
			}
		})
	}
}

func TestSelectCheapest_FiltersByToolsConstraint(t *testing.T) {
	cfg := &config.Config{
		OpenCodeGo: config.OpenCodeGoConfig{APIKey: "go-key"},
	}
	selector := NewSelector(selectorTestCatalog(t), withSelectorScenarios(cfg))

	got, err := selector.SelectCheapest("default", ScenarioConstraints{Tools: true})
	if err != nil {
		t.Fatalf("SelectCheapest returned error: %v", err)
	}

	want := "cheap-tools"
	if got.ModelID != want {
		t.Errorf("SelectCheapest(default, tools) = %q, want %q", got.ModelID, want)
	}
	if !got.Tools {
		t.Errorf("SelectCheapest(default, tools).Tools = false, want true")
	}
}

func TestSelectCheapest_FiltersByVisionConstraint(t *testing.T) {
	cfg := &config.Config{
		OpenCodeGo: config.OpenCodeGoConfig{APIKey: "go-key"},
		OpenRouter: config.OpenRouterConfig{APIKey: "or-key"},
	}
	selector := NewSelector(selectorTestCatalog(t), withSelectorScenarios(cfg))

	got, err := selector.SelectCheapest("default", ScenarioConstraints{Vision: true})
	if err != nil {
		t.Fatalf("SelectCheapest returned error: %v", err)
	}

	want := "vision-model"
	if got.ModelID != want {
		t.Errorf("SelectCheapest(default, vision) = %q, want %q", got.ModelID, want)
	}
	if !got.Vision {
		t.Errorf("SelectCheapest(default, vision).Vision = false, want true")
	}
}

func TestSelectCheapest_FiltersByReasoningConstraint(t *testing.T) {
	cfg := &config.Config{
		OpenRouter: config.OpenRouterConfig{APIKey: "or-key"},
	}
	selector := NewSelector(selectorTestCatalog(t), withSelectorScenarios(cfg))

	got, err := selector.SelectCheapest("default", ScenarioConstraints{Reasoning: true})
	if err != nil {
		t.Fatalf("SelectCheapest returned error: %v", err)
	}

	want := "reasoning-model"
	if got.ModelID != want {
		t.Errorf("SelectCheapest(default, reasoning) = %q, want %q", got.ModelID, want)
	}
	if !got.Reasoning {
		t.Errorf("SelectCheapest(default, reasoning).Reasoning = false, want true")
	}
}

func TestSelectCheapest_FiltersByContextConstraint(t *testing.T) {
	cfg := &config.Config{
		OpenCodeGo: config.OpenCodeGoConfig{APIKey: "go-key"},
		OpenRouter: config.OpenRouterConfig{APIKey: "or-key"},
	}
	selector := NewSelector(selectorTestCatalog(t), withSelectorScenarios(cfg))

	got, err := selector.SelectCheapest("default", ScenarioConstraints{Context: 500000})
	if err != nil {
		t.Fatalf("SelectCheapest returned error: %v", err)
	}

	want := "large-context"
	if got.ModelID != want {
		t.Errorf("SelectCheapest(default, context=500000) = %q, want %q", got.ModelID, want)
	}
	if got.ContextWindow < 500000 {
		t.Errorf("SelectCheapest(default, context=500000).ContextWindow = %d, want >= 500000", got.ContextWindow)
	}
}

func TestSelectCheapest_FiltersByScenarioRequirements(t *testing.T) {
	cfg := &config.Config{
		OpenRouter: config.OpenRouterConfig{APIKey: "or-key"},
	}
	selector := NewSelector(selectorTestCatalog(t), withSelectorScenarios(cfg))

	tests := []struct {
		scenario string
		want     string
	}{
		{"vision_required", "vision-model"},
		{"reasoning_required", "reasoning-model"},
	}

	for _, tt := range tests {
		t.Run(tt.scenario, func(t *testing.T) {
			got, err := selector.SelectCheapest(tt.scenario, ScenarioConstraints{})
			if err != nil {
				t.Fatalf("SelectCheapest returned error: %v", err)
			}
			if got.ModelID != tt.want {
				t.Errorf("SelectCheapest(%q) = %q, want %q", tt.scenario, got.ModelID, tt.want)
			}
		})
	}
}

func TestSelectCheapest_EnabledProvidersOnly(t *testing.T) {
	cat := selectorTestCatalog(t)

	tests := []struct {
		name        string
		cfg         *config.Config
		scenario    string
		constraints ScenarioConstraints
		want        string
		wantErr     bool
	}{
		{
			name: "provider-specific key enables only that provider",
			cfg: &config.Config{
				OpenCodeGo: config.OpenCodeGoConfig{APIKey: "go-key"},
			},
			scenario:    "default",
			constraints: ScenarioConstraints{},
			want:        "cheap-no-tools",
		},
		{
			name: "openrouter-only key excludes opencode-go models",
			cfg: &config.Config{
				OpenRouter: config.OpenRouterConfig{APIKey: "or-key"},
			},
			scenario:    "default",
			constraints: ScenarioConstraints{},
			want:        "large-context",
		},
		{
			name: "global key enables all providers",
			cfg: &config.Config{
				APIKey: "global-key",
			},
			scenario:    "default",
			constraints: ScenarioConstraints{},
			want:        "cheap-no-tools",
		},
		{
			name: "disabled catalog provider is ignored even with key",
			cfg: &config.Config{
				APIKeys: []string{"global-key"},
			},
			scenario:    "default",
			constraints: ScenarioConstraints{},
			want:        "cheap-no-tools",
		},
		{
			name: "no keys disables all providers",
			cfg: &config.Config{
				OpenCodeGo: config.OpenCodeGoConfig{},
				OpenRouter: config.OpenRouterConfig{},
			},
			scenario:    "default",
			constraints: ScenarioConstraints{},
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selector := NewSelector(cat, withSelectorScenarios(tt.cfg))
			got, err := selector.SelectCheapest(tt.scenario, tt.constraints)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("SelectCheapest expected error, got model %q", got.ModelID)
				}
				return
			}
			if err != nil {
				t.Fatalf("SelectCheapest returned error: %v", err)
			}
			if got.ModelID != tt.want {
				t.Errorf("SelectCheapest(%q) = %q, want %q", tt.scenario, got.ModelID, tt.want)
			}
		})
	}
}

func TestSelectCheapest_PreferredProvidersFilter(t *testing.T) {
	cfg := &config.Config{
		OpenCodeGo: config.OpenCodeGoConfig{APIKey: "go-key"},
		OpenRouter: config.OpenRouterConfig{APIKey: "or-key"},
	}
	selector := NewSelector(selectorTestCatalog(t), withSelectorScenarios(cfg))

	got, err := selector.SelectCheapest("preferred_only", ScenarioConstraints{})
	if err != nil {
		t.Fatalf("SelectCheapest returned error: %v", err)
	}

	// openrouter models only, cheapest is large-context at cost 3.0
	if got.Provider != "openrouter" {
		t.Errorf("SelectCheapest(preferred_only) provider = %q, want %q", got.Provider, "openrouter")
	}
	if got.ModelID != "large-context" {
		t.Errorf("SelectCheapest(preferred_only) = %q, want %q", got.ModelID, "large-context")
	}
}

func TestSelectCheapest_NoCandidateReturnsError(t *testing.T) {
	cfg := &config.Config{
		OpenCodeGo: config.OpenCodeGoConfig{APIKey: "go-key"},
	}
	selector := NewSelector(selectorTestCatalog(t), withSelectorScenarios(cfg))

	_, err := selector.SelectCheapest("default", ScenarioConstraints{Reasoning: true})
	if err == nil {
		t.Fatal("SelectCheapest expected error for unmatched constraints, got nil")
	}
	if !errors.Is(err, ErrNoCandidateModel) {
		t.Errorf("SelectCheapest error = %v, want ErrNoCandidateModel", err)
	}
}

// An unconfigured scenario must NOT disable cost routing. Before this was
// fixed, SelectCheapest returned "unknown scenario" for every real scenario
// name, and the caller discarded that error, so cost routing silently did
// nothing in production while appearing to be enabled.
func TestSelectCheapest_UnconfiguredScenarioUsesZeroPolicy(t *testing.T) {
	cfg := &config.Config{
		OpenCodeGo: config.OpenCodeGoConfig{APIKey: "go-key"},
		// Scenarios deliberately left unset.
		CostRouting: &config.CostRoutingConfig{Enabled: true},
	}
	selector := NewSelector(selectorTestCatalog(t), cfg)

	for _, scenario := range config.CostScenarioNames {
		got, err := selector.SelectCheapest(scenario, ScenarioConstraints{})
		if err != nil {
			t.Fatalf("SelectCheapest(%q) with no configured scenarios: %v", scenario, err)
		}
		if got.ModelID != "cheap-no-tools" {
			t.Errorf("SelectCheapest(%q) = %q, want the cheapest model cheap-no-tools", scenario, got.ModelID)
		}
	}
}

func TestSelectCheapest_Constraints_ToolsRequired(t *testing.T) {
	cfg := &config.Config{
		OpenCodeGo: config.OpenCodeGoConfig{APIKey: "go-key"},
		OpenRouter: config.OpenRouterConfig{APIKey: "or-key"},
	}
	selector := NewSelector(selectorTestCatalog(t), withSelectorScenarios(cfg))

	got, err := selector.SelectCheapest("tools_required", ScenarioConstraints{})
	if err != nil {
		t.Fatalf("SelectCheapest returned error: %v", err)
	}

	if got.ModelID != "cheap-tools" {
		t.Errorf("SelectCheapest(tools_required) = %q, want %q", got.ModelID, "cheap-tools")
	}
	if got.Provider != "opencode-go" {
		t.Errorf("SelectCheapest(tools_required) provider = %q, want %q", got.Provider, "opencode-go")
	}
	if !got.Tools {
		t.Errorf("SelectCheapest(tools_required).Tools = false, want true")
	}
	// cheap-no-tools has the same total cost but lacks tools and must not win.
	if got.CostInputPerM+got.CostOutputPerM != 2.0 {
		t.Errorf("SelectCheapest(tools_required) total cost = %v, want 2.0", got.CostInputPerM+got.CostOutputPerM)
	}
}

func TestSelectCheapest_Constraints_VisionRequired(t *testing.T) {
	cfg := &config.Config{
		OpenCodeGo: config.OpenCodeGoConfig{APIKey: "go-key"},
		OpenRouter: config.OpenRouterConfig{APIKey: "or-key"},
	}
	selector := NewSelector(selectorTestCatalog(t), withSelectorScenarios(cfg))

	got, err := selector.SelectCheapest("vision_required", ScenarioConstraints{})
	if err != nil {
		t.Fatalf("SelectCheapest returned error: %v", err)
	}

	if got.ModelID != "vision-model" {
		t.Errorf("SelectCheapest(vision_required) = %q, want %q", got.ModelID, "vision-model")
	}
	if got.Provider != "openrouter" {
		t.Errorf("SelectCheapest(vision_required) provider = %q, want %q", got.Provider, "openrouter")
	}
	if !got.Vision {
		t.Errorf("SelectCheapest(vision_required).Vision = false, want true")
	}
	// vision-model is not the cheapest overall model; cheaper non-vision models must be ignored.
	if got.CostInputPerM+got.CostOutputPerM != 8.0 {
		t.Errorf("SelectCheapest(vision_required) total cost = %v, want 8.0", got.CostInputPerM+got.CostOutputPerM)
	}
}

func TestSelectCheapest_Constraints_ReasoningRequired(t *testing.T) {
	cfg := &config.Config{
		OpenCodeGo: config.OpenCodeGoConfig{APIKey: "go-key"},
		OpenRouter: config.OpenRouterConfig{APIKey: "or-key"},
	}
	selector := NewSelector(selectorTestCatalog(t), withSelectorScenarios(cfg))

	got, err := selector.SelectCheapest("reasoning_required", ScenarioConstraints{})
	if err != nil {
		t.Fatalf("SelectCheapest returned error: %v", err)
	}

	if got.ModelID != "reasoning-model" {
		t.Errorf("SelectCheapest(reasoning_required) = %q, want %q", got.ModelID, "reasoning-model")
	}
	if got.Provider != "openrouter" {
		t.Errorf("SelectCheapest(reasoning_required) provider = %q, want %q", got.Provider, "openrouter")
	}
	if !got.Reasoning {
		t.Errorf("SelectCheapest(reasoning_required).Reasoning = false, want true")
	}
}

func TestSelectCheapest_Constraints_ContextWindow(t *testing.T) {
	cfg := &config.Config{
		OpenCodeGo: config.OpenCodeGoConfig{APIKey: "go-key"},
		OpenRouter: config.OpenRouterConfig{APIKey: "or-key"},
	}
	selector := NewSelector(selectorTestCatalog(t), withSelectorScenarios(cfg))

	got, err := selector.SelectCheapest("long_context", ScenarioConstraints{})
	if err != nil {
		t.Fatalf("SelectCheapest returned error: %v", err)
	}

	if got.ModelID != "large-context" {
		t.Errorf("SelectCheapest(long_context) = %q, want %q", got.ModelID, "large-context")
	}
	if got.ContextWindow < 500000 {
		t.Errorf("SelectCheapest(long_context).ContextWindow = %d, want >= 500000", got.ContextWindow)
	}
	// Cheaper models with smaller context windows must be excluded.
	if got.CostInputPerM+got.CostOutputPerM != 3.0 {
		t.Errorf("SelectCheapest(long_context) total cost = %v, want 3.0", got.CostInputPerM+got.CostOutputPerM)
	}
}

func TestSelectCheapest_Constraints_CombinedVisionAndTools(t *testing.T) {
	cfg := &config.Config{
		OpenCodeGo: config.OpenCodeGoConfig{APIKey: "go-key"},
		OpenRouter: config.OpenRouterConfig{APIKey: "or-key"},
	}
	selector := NewSelector(selectorTestCatalog(t), withSelectorScenarios(cfg))

	got, err := selector.SelectCheapest("vision_complex", ScenarioConstraints{})
	if err != nil {
		t.Fatalf("SelectCheapest returned error: %v", err)
	}

	if got.ModelID != "vision-model" {
		t.Errorf("SelectCheapest(vision_complex) = %q, want %q", got.ModelID, "vision-model")
	}
	if got.Provider != "openrouter" {
		t.Errorf("SelectCheapest(vision_complex) provider = %q, want %q", got.Provider, "openrouter")
	}
	if !got.Vision {
		t.Errorf("SelectCheapest(vision_complex).Vision = false, want true")
	}
	if !got.Tools {
		t.Errorf("SelectCheapest(vision_complex).Tools = false, want true")
	}
	// vision-model is the only model satisfying both constraints and is not the cheapest overall.
	if got.CostInputPerM+got.CostOutputPerM != 8.0 {
		t.Errorf("SelectCheapest(vision_complex) total cost = %v, want 8.0", got.CostInputPerM+got.CostOutputPerM)
	}
}

// TestSelectCheapest_PenaltyPerProvider verifies that cost_routing.penalty_per_provider
// inflates a provider's effective cost during selection. When opencode-go is penalised
// enough, large-context on openrouter (unpenalised, cost 3.0) becomes cheaper than
// cheap-no-tools on opencode-go (cost 2.0 + penalty).
func TestSelectCheapest_PenaltyPerProvider(t *testing.T) {
	cfg := &config.Config{
		OpenCodeGo: config.OpenCodeGoConfig{APIKey: "go-key"},
		OpenRouter: config.OpenRouterConfig{APIKey: "or-key"},
		CostRouting: &config.CostRoutingConfig{
			PenaltyPerProvider: map[string]float64{
				"opencode-go": 2.0,
			},
		},
	}
	selector := NewSelector(selectorTestCatalog(t), withSelectorScenarios(cfg))

	got, err := selector.SelectCheapest("default", ScenarioConstraints{})
	if err != nil {
		t.Fatalf("SelectCheapest returned error: %v", err)
	}

	// Without the penalty cheap-no-tools (opencode-go, cost 2.0) would win at 2.0.
	// With a 2.0 penalty on opencode-go its effective cost becomes 4.0, so
	// large-context on the unpenalised openrouter (cost 3.0) should win.
	if got.ModelID != "large-context" {
		t.Errorf("SelectCheapest(default) = %q, want %q (penalty should flip to unpenalised provider)", got.ModelID, "large-context")
	}
	if got.Provider != "openrouter" {
		t.Errorf("SelectCheapest(default) provider = %q, want %q", got.Provider, "openrouter")
	}
	// The raw cost should be 3.0 (openrouter's large-context), not the penalised cost.
	if got.CostInputPerM+got.CostOutputPerM != 3.0 {
		t.Errorf("SelectCheapest(default) raw cost = %v, want 3.0", got.CostInputPerM+got.CostOutputPerM)
	}
}

// TestSelectCheapest_PenaltyPerProvider_NoEffectOnUnlisted verifies that a penalty
// only applies to the named providers and does not affect unlisted providers.
func TestSelectCheapest_PenaltyPerProvider_NoEffectOnUnlisted(t *testing.T) {
	cfg := &config.Config{
		OpenRouter: config.OpenRouterConfig{APIKey: "or-key"},
		OpenCodeGo: config.OpenCodeGoConfig{APIKey: "go-key"},
		CostRouting: &config.CostRoutingConfig{
			PenaltyPerProvider: map[string]float64{
				"nonexistent-provider": 100.0,
			},
		},
	}
	selector := NewSelector(selectorTestCatalog(t), withSelectorScenarios(cfg))

	got, err := selector.SelectCheapest("default", ScenarioConstraints{})
	if err != nil {
		t.Fatalf("SelectCheapest returned error: %v", err)
	}

	// A penalty on a provider that does not exist in the catalog must not affect
	// selection — cheap-no-tools (opencode-go, cost 2.0) should still win.
	if got.ModelID != "cheap-no-tools" {
		t.Errorf("SelectCheapest(default) = %q, want %q (unused penalty must not affect selection)", got.ModelID, "cheap-no-tools")
	}
	if got.Provider != "opencode-go" {
		t.Errorf("SelectCheapest(default) provider = %q, want %q", got.Provider, "opencode-go")
	}
}

// TestSelectCheapest_MaxContextWindow verifies that cost_routing.max_context_window
// caps the context window of candidate models, filtering out those that exceed it.
func TestSelectCheapest_MaxContextWindow(t *testing.T) {
	t.Run("filters models exceeding the cap", func(t *testing.T) {
		cfg := &config.Config{
			OpenCodeGo: config.OpenCodeGoConfig{APIKey: "go-key"},
			OpenRouter: config.OpenRouterConfig{APIKey: "or-key"},
			CostRouting: &config.CostRoutingConfig{
				MaxContextWindow: 200000,
			},
		}
		selector := NewSelector(selectorTestCatalog(t), withSelectorScenarios(cfg))

		got, err := selector.SelectCheapest("default", ScenarioConstraints{})
		if err != nil {
			t.Fatalf("SelectCheapest returned error: %v", err)
		}

		// large-context (1M) and vision-model (256K) exceed the 200K cap and must be excluded.
		// The cheapest remaining model is cheap-no-tools (128K) on opencode-go at cost 2.0.
		if got.ModelID != "cheap-no-tools" {
			t.Errorf("SelectCheapest(default) = %q, want %q", got.ModelID, "cheap-no-tools")
		}
		if got.ContextWindow > 200000 {
			t.Errorf("SelectCheapest(default) context = %d, want <= 200000", got.ContextWindow)
		}
	})

	t.Run("zero cap has no effect", func(t *testing.T) {
		cfg := &config.Config{
			OpenCodeGo: config.OpenCodeGoConfig{APIKey: "go-key"},
			OpenRouter: config.OpenRouterConfig{APIKey: "or-key"},
			CostRouting: &config.CostRoutingConfig{
				MaxContextWindow: 0,
			},
		}
		selector := NewSelector(selectorTestCatalog(t), withSelectorScenarios(cfg))

		got, err := selector.SelectCheapest("default", ScenarioConstraints{})
		if err != nil {
			t.Fatalf("SelectCheapest returned error: %v", err)
		}

		// Without the cap, large-context (1M) is eligible and expensive models are available.
		// The cheapest remains cheap-no-tools.
		if got.ModelID != "cheap-no-tools" {
			t.Errorf("SelectCheapest(default) = %q, want %q", got.ModelID, "cheap-no-tools")
		}
	})

	t.Run("cap filters all models producing error", func(t *testing.T) {
		cfg := &config.Config{
			OpenCodeGo: config.OpenCodeGoConfig{APIKey: "go-key"},
			CostRouting: &config.CostRoutingConfig{
				MaxContextWindow: 100,
			},
		}
		selector := NewSelector(selectorTestCatalog(t), withSelectorScenarios(cfg))

		_, err := selector.SelectCheapest("default", ScenarioConstraints{})
		if err == nil {
			t.Fatal("SelectCheapest expected error when MaxContextWindow excludes every model, got nil")
		}
		if !errors.Is(err, ErrNoCandidateModel) {
			t.Errorf("SelectCheapest error = %v, want ErrNoCandidateModel", err)
		}
	})
}

// TestSelectCheapest_GlobalPreferProviders verifies that
// cost_routing.prefer_providers filters the eligible provider set globally.
func TestSelectCheapest_GlobalPreferProviders(t *testing.T) {
	t.Run("global pref limits to listed providers", func(t *testing.T) {
		cfg := &config.Config{
			OpenCodeGo: config.OpenCodeGoConfig{APIKey: "go-key"},
			OpenRouter: config.OpenRouterConfig{APIKey: "or-key"},
			CostRouting: &config.CostRoutingConfig{
				PreferProviders: []string{"openrouter"},
			},
		}
		selector := NewSelector(selectorTestCatalog(t), withSelectorScenarios(cfg))

		// "default" scenario has no scenario-level preferences, so the global
		// prefer_providers list is used alone. Only openrouter models are eligible.
		got, err := selector.SelectCheapest("default", ScenarioConstraints{})
		if err != nil {
			t.Fatalf("SelectCheapest returned error: %v", err)
		}

		if got.Provider != "openrouter" {
			t.Errorf("SelectCheapest(default) provider = %q, want %q", got.Provider, "openrouter")
		}
		// Cheapest openrouter model is large-context at cost 3.0.
		if got.ModelID != "large-context" {
			t.Errorf("SelectCheapest(default) = %q, want %q", got.ModelID, "large-context")
		}
	})

	t.Run("global pref intersects with scenario pref", func(t *testing.T) {
		cfg := &config.Config{
			OpenCodeGo: config.OpenCodeGoConfig{APIKey: "go-key"},
			OpenRouter: config.OpenRouterConfig{APIKey: "or-key"},
			CostRouting: &config.CostRoutingConfig{
				PreferProviders: []string{"opencode-go"},
			},
		}
		selector := NewSelector(selectorTestCatalog(t), withSelectorScenarios(cfg))

		// "preferred_only" scenario prefers openrouter, global pref prefers
		// opencode-go. The intersection is empty → no candidates.
		_, err := selector.SelectCheapest("preferred_only", ScenarioConstraints{})
		if err == nil {
			t.Fatal("SelectCheapest expected error when global and scenario prefs intersect to empty, got nil")
		}
		if !errors.Is(err, ErrNoCandidateModel) {
			t.Errorf("SelectCheapest error = %v, want ErrNoCandidateModel", err)
		}
	})

	t.Run("scenario pref used when global pref is empty", func(t *testing.T) {
		cfg := &config.Config{
			OpenCodeGo: config.OpenCodeGoConfig{APIKey: "go-key"},
			OpenRouter: config.OpenRouterConfig{APIKey: "or-key"},
		}
		selector := NewSelector(selectorTestCatalog(t), withSelectorScenarios(cfg))

		// No global prefer_providers, so "preferred_only" scenario's own
		// preferred_providers (openrouter) is used.
		got, err := selector.SelectCheapest("preferred_only", ScenarioConstraints{})
		if err != nil {
			t.Fatalf("SelectCheapest returned error: %v", err)
		}

		if got.Provider != "openrouter" {
			t.Errorf("SelectCheapest(preferred_only) provider = %q, want %q", got.Provider, "openrouter")
		}
	})
}

// enabledProviders used to name four platforms in a hand-written map, so
// CommandCode was absent however many keys it had and cost routing could never
// pick it. The set now comes from the registry, and this pins that every
// configured platform is reachable - a new platform must not need a second edit
// here to become selectable.
func TestEnabledProvidersCoversEveryConfiguredPlatform(t *testing.T) {
	cfg := &config.Config{
		OpenCodeGo:  config.OpenCodeGoConfig{APIKey: "go-key"},
		CommandCode: config.CommandCodeConfig{APIKey: "cc-key"},
		OpenRouter:  config.OpenRouterConfig{APIKey: "or-key"},
	}
	enabled := enabledProviders(cfg)

	for _, want := range []string{"opencode-go", "commandcode", "openrouter"} {
		if !enabled[want] {
			t.Errorf("a platform with its own key must be selectable: %s (got %v)", want, enabled)
		}
	}
	// A platform with no key of its own and no global fallback stays out.
	if enabled["aws-bedrock"] {
		t.Error("aws-bedrock has no key configured and must not be enabled")
	}

	// Only the legacy platforms permit a global-key fallback.
	global := enabledProviders(&config.Config{APIKey: "global-key"})
	for _, want := range []string{"opencode-go", "opencode-zen", "aws-bedrock", "openrouter"} {
		if !global[want] {
			t.Errorf("a global key must enable %s (got %v)", want, global)
		}
	}
	if global[site.CommandCode] {
		t.Error("CommandCode requires its own key and must not inherit the global key")
	}
}

// The same guard config keeps over providerKeySource and providerTimeoutSource:
// a hand-written platform map must cover the registry, or a platform silently
// stops being selectable. This is the general form of the CommandCode omission
// above, so a platform added later cannot reintroduce it here.
func TestEnabledProvidersMatchesRegistryCoverage(t *testing.T) {
	// A global key covers legacy platforms; CommandCode and ClinePass each have
	// a dedicated key and must not inherit one.
	enabled := enabledProviders(&config.Config{
		APIKey:      "global-key",
		CommandCode: config.CommandCodeConfig{APIKey: "cc-key"},
		ClinePass:   config.ClinePassConfig{APIKey: "cp-key"},
	})
	for _, d := range site.All() {
		if !enabled[d.ID] {
			t.Errorf("platform %q is in the registry but cannot be selected by cost routing", d.ID)
		}
	}
	for id := range enabled {
		if _, ok := site.Lookup(id); !ok {
			t.Errorf("cost routing enables %q, which is not a known platform", id)
		}
	}
}

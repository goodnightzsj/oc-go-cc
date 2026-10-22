package router

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/routatic/proxy/internal/catalog"
	"github.com/routatic/proxy/internal/config"
)

func boolPtr(b bool) *bool { return &b }
func writeTestCatalog(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "catalog.json")
	data := []byte(`{
  "providers": {
    "opencode-go": {
      "name": "opencode-go",
      "base_url": "https://go.opencode.ai",
      "api_key": "",
      "enabled": true,
      "anthropic_tools_disabled": false
    },
    "openrouter": {
      "name": "openrouter",
      "base_url": "https://openrouter.ai/api/v1",
      "api_key": "",
      "enabled": true,
      "anthropic_tools_disabled": false
    }
  },
  "models": {
    "opencode-go/deepseek-v4-flash": {
      "id": "opencode-go/deepseek-v4-flash",
      "name": "DeepSeek V4 Flash",
      "limit": {"context": 1000000},
      "rates": {"input": 0.0, "output": 0.0},
      "tool_call": true,
      "modalities": {"input": ["text"], "output": ["text"]}
    },
    "opencode-go/kimi-k2.6": {
      "id": "opencode-go/kimi-k2.6",
      "name": "Kimi K2.6",
      "limit": {"context": 256000},
      "rates": {"input": 0.0, "output": 0.0},
      "tool_call": true,
      "modalities": {"input": ["text", "image"], "output": ["text"]}
    },
    "openrouter/kimi-k2.6": {
      "id": "openrouter/kimi-k2.6",
      "name": "Kimi K2.6",
      "limit": {"context": 256000},
      "rates": {"input": 0.0, "output": 0.0},
      "tool_call": true,
      "modalities": {"input": ["text", "image"], "output": ["text"]}
    },
    "opencode-go/glm-5": {
      "id": "opencode-go/glm-5",
      "name": "GLM 5",
      "limit": {"context": 200000},
      "rates": {"input": 0.0, "output": 0.0},
      "tool_call": true,
      "modalities": {"input": ["text"], "output": ["text"]}
    },
    "openrouter/glm-5": {
      "id": "openrouter/glm-5",
      "name": "GLM 5",
      "limit": {"context": 200000},
      "rates": {"input": 0.0, "output": 0.0},
      "tool_call": true,
      "modalities": {"input": ["text"], "output": ["text"]}
    }
  }
}`)

	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("failed to write test catalog: %v", err)
	}
	return path
}

func newTestAtomicConfig(cfg *config.Config) *config.AtomicConfig {
	return config.NewAtomicConfig(cfg, "/tmp/test-config.json")
}

func TestRoute_RespectRequestedModel_BypassesScenarioRouting(t *testing.T) {
	cfg := &config.Config{
		RespectRequestedModel: boolPtr(true),
		Models: map[string]config.ModelConfig{
			"default": {
				Provider: "opencode-go",
				ModelID:  "kimi-k2.6",
			},
			"kimi-k2.6": {
				Provider:         "opencode-go",
				ModelID:          "kimi-k2.6",
				Temperature:      0.7,
				MaxTokens:        4096,
				ContextThreshold: 80000,
			},
		},
		Fallbacks: map[string][]config.ModelConfig{
			"default": {
				{Provider: "opencode-go", ModelID: "qwen3.5-plus"},
			},
		},
	}

	router := NewModelRouter(newTestAtomicConfig(cfg))

	// Complex message that would normally route to GLM-5.1
	messages := []MessageContent{
		{Role: "user", Content: "Architect a new microservice"},
	}

	result, err := router.Route(messages, 100, "kimi-k2.6")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Primary.ModelID != "kimi-k2.6" {
		t.Errorf("expected model kimi-k2.6, got %s", result.Primary.ModelID)
	}
	if result.Primary.Temperature != 0.7 {
		t.Errorf("expected temperature 0.7, got %f", result.Primary.Temperature)
	}
	if result.Primary.MaxTokens != 4096 {
		t.Errorf("expected max_tokens 4096, got %d", result.Primary.MaxTokens)
	}
	if result.Scenario != ScenarioDefault {
		t.Errorf("expected ScenarioDefault, got %s", result.Scenario)
	}
}

func TestRoute_RespectRequestedModel_False_UsesScenarioRouting(t *testing.T) {
	cfg := &config.Config{
		RespectRequestedModel: boolPtr(false),
		Models: map[string]config.ModelConfig{
			"default": {ModelID: "kimi-k2.6"},
			"complex": {ModelID: "glm-5.1"},
		},
		Fallbacks: map[string][]config.ModelConfig{
			"default": {{ModelID: "qwen3.5-plus"}},
		},
	}

	router := NewModelRouter(newTestAtomicConfig(cfg))

	messages := []MessageContent{
		{Role: "user", Content: "Architect a new microservice"},
	}

	result, err := router.Route(messages, 100, "kimi-k2.6")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should use scenario routing, not the requested model
	if result.Primary.ModelID != "glm-5.1" {
		t.Errorf("expected scenario-routed model glm-5.1, got %s", result.Primary.ModelID)
	}
}

func TestRoute_RespectRequestedModel_EmptyModel_FallsThrough(t *testing.T) {
	cfg := &config.Config{
		RespectRequestedModel: boolPtr(true),
		Models: map[string]config.ModelConfig{
			"default": {ModelID: "kimi-k2.6"},
		},
		Fallbacks: map[string][]config.ModelConfig{
			"default": {{ModelID: "qwen3.5-plus"}},
		},
	}

	router := NewModelRouter(newTestAtomicConfig(cfg))

	messages := []MessageContent{
		{Role: "user", Content: "Hello"},
	}

	result, err := router.Route(messages, 100, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Empty model should fall through to scenario routing
	if result.Primary.ModelID != "kimi-k2.6" {
		t.Errorf("expected default model kimi-k2.6, got %s", result.Primary.ModelID)
	}
}

func TestRoute_RespectRequestedModel_UnknownModel_UsesDefaults(t *testing.T) {
	cfg := &config.Config{
		RespectRequestedModel: boolPtr(true),
		Models: map[string]config.ModelConfig{
			"default": {
				Provider:    "opencode-go",
				ModelID:     "kimi-k2.6",
				Temperature: 0.5,
				MaxTokens:   8192,
			},
		},
		Fallbacks: map[string][]config.ModelConfig{
			"default": {{ModelID: "qwen3.5-plus"}},
		},
	}

	router := NewModelRouter(newTestAtomicConfig(cfg))

	messages := []MessageContent{
		{Role: "user", Content: "Hello"},
	}

	result, err := router.Route(messages, 100, "some-unknown-model")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Primary.ModelID != "some-unknown-model" {
		t.Errorf("expected model some-unknown-model, got %s", result.Primary.ModelID)
	}
	// Unknown model should inherit default temperature/max_tokens
	if result.Primary.Temperature != 0.5 {
		t.Errorf("expected inherited temperature 0.5, got %f", result.Primary.Temperature)
	}
	if result.Primary.MaxTokens != 8192 {
		t.Errorf("expected inherited max_tokens 8192, got %d", result.Primary.MaxTokens)
	}
}

func TestRouteForStreaming_RespectRequestedModel_BypassesScenarioRouting(t *testing.T) {
	cfg := &config.Config{
		RespectRequestedModel: boolPtr(true),
		Models: map[string]config.ModelConfig{
			"default": {ModelID: "qwen3.6-plus"},
			"kimi-k2.6": {
				Provider: "opencode-go",
				ModelID:  "kimi-k2.6",
			},
		},
		Fallbacks: map[string][]config.ModelConfig{
			"default": {{ModelID: "qwen3.5-plus"}},
		},
	}

	router := NewModelRouter(newTestAtomicConfig(cfg))

	messages := []MessageContent{
		{Role: "user", Content: "Hello"},
	}

	result, err := router.RouteForStreaming(messages, 100, "kimi-k2.6")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Primary.ModelID != "kimi-k2.6" {
		t.Errorf("expected model kimi-k2.6, got %s", result.Primary.ModelID)
	}
	if result.Scenario != ScenarioDefault {
		t.Errorf("expected ScenarioDefault, got %s", result.Scenario)
	}
}

func TestRouteForStreaming_RespectRequestedModel_False_UsesScenarioRouting(t *testing.T) {
	cfg := &config.Config{
		RespectRequestedModel: boolPtr(false),
		Models: map[string]config.ModelConfig{
			"default": {ModelID: "qwen3.6-plus"},
		},
		Fallbacks: map[string][]config.ModelConfig{
			"default": {{ModelID: "qwen3.5-plus"}},
		},
	}

	router := NewModelRouter(newTestAtomicConfig(cfg))

	messages := []MessageContent{
		{Role: "user", Content: "Hello"},
	}

	result, err := router.RouteForStreaming(messages, 100, "kimi-k2.6")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should use streaming scenario routing, not the requested model
	if result.Primary.ModelID != "qwen3.6-plus" {
		t.Errorf("expected streaming model qwen3.6-plus, got %s", result.Primary.ModelID)
	}
}

func TestResolveRequestedModel_UsesFallbacks(t *testing.T) {
	cfg := &config.Config{
		RespectRequestedModel: boolPtr(true),
		Models: map[string]config.ModelConfig{
			"kimi-k2.6": {ModelID: "kimi-k2.6"},
		},
		Fallbacks: map[string][]config.ModelConfig{
			"default": {
				{Provider: "opencode-go", ModelID: "qwen3.5-plus"},
				{Provider: "opencode-go", ModelID: "glm-5.1"},
			},
		},
	}

	router := NewModelRouter(newTestAtomicConfig(cfg))

	result, ok, err := router.resolveRequestedModel(cfg, "kimi-k2.6", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected resolveRequestedModel to match")
	}
	if len(result.Fallbacks) != 2 {
		t.Errorf("expected 2 fallbacks, got %d", len(result.Fallbacks))
	}
	if result.Fallbacks[0].ModelID != "qwen3.5-plus" {
		t.Errorf("expected first fallback qwen3.5-plus, got %s", result.Fallbacks[0].ModelID)
	}
}

func TestRouteWithOverride_MatchesKey(t *testing.T) {
	cfg := &config.Config{
		ModelOverrides: map[string]config.ModelConfig{
			"kimi-k2.6": {
				Provider:    "opencode-go",
				ModelID:     "kimi-k2.6",
				Temperature: 0.3,
				MaxTokens:   2048,
			},
		},
		Fallbacks: map[string][]config.ModelConfig{
			"kimi-k2.6": {
				{Provider: "opencode-go", ModelID: "qwen3.5-plus"},
			},
		},
	}

	router := NewModelRouter(newTestAtomicConfig(cfg))

	result, ok := router.RouteWithOverride("kimi-k2.6")
	if !ok {
		t.Fatal("expected RouteWithOverride to match")
	}
	if result.Primary.ModelID != "kimi-k2.6" {
		t.Errorf("expected primary kimi-k2.6, got %s", result.Primary.ModelID)
	}
	if result.Primary.Temperature != 0.3 {
		t.Errorf("expected temperature 0.3, got %f", result.Primary.Temperature)
	}
	if result.Scenario != ScenarioOverride {
		t.Errorf("expected ScenarioOverride, got %s", result.Scenario)
	}
	if len(result.Fallbacks) != 1 || result.Fallbacks[0].ModelID != "qwen3.5-plus" {
		t.Errorf("expected single fallback qwen3.5-plus, got %+v", result.Fallbacks)
	}
}

func TestRouteWithOverride_NoMatch(t *testing.T) {
	cfg := &config.Config{
		ModelOverrides: map[string]config.ModelConfig{
			"kimi-k2.6": {Provider: "opencode-go", ModelID: "kimi-k2.6"},
		},
	}

	router := NewModelRouter(newTestAtomicConfig(cfg))

	result, ok := router.RouteWithOverride("some-other-model")
	if ok {
		t.Errorf("expected no match, got result %+v", result)
	}
}

func TestRouteWithOverride_NilMap(t *testing.T) {
	cfg := &config.Config{} // ModelOverrides is nil

	router := NewModelRouter(newTestAtomicConfig(cfg))

	if _, ok := router.RouteWithOverride("anything"); ok {
		t.Error("expected no match for nil ModelOverrides map (must not panic)")
	}
}

func TestRouteWithOverride_MissingFallbacksKey_FallsBackToDefault(t *testing.T) {
	cfg := &config.Config{
		ModelOverrides: map[string]config.ModelConfig{
			"kimi-k2.6": {Provider: "opencode-go", ModelID: "kimi-k2.6"},
		},
		Fallbacks: map[string][]config.ModelConfig{
			"default": {
				{Provider: "opencode-go", ModelID: "qwen3.5-plus"},
				{Provider: "opencode-go", ModelID: "mimo-v2.5-pro"},
			},
		},
		// No entry in Fallbacks for "kimi-k2.6" — should fall back to
		// fallbacks["default"], matching Route/RouteForStreaming behavior.
	}

	router := NewModelRouter(newTestAtomicConfig(cfg))

	result, ok := router.RouteWithOverride("kimi-k2.6")
	if !ok {
		t.Fatal("expected RouteWithOverride to match")
	}
	if len(result.Fallbacks) != 2 {
		t.Fatalf("expected 2 default fallbacks, got %d: %+v", len(result.Fallbacks), result.Fallbacks)
	}
	if result.Fallbacks[0].ModelID != "qwen3.5-plus" || result.Fallbacks[1].ModelID != "mimo-v2.5-pro" {
		t.Errorf("expected default fallbacks [qwen3.5-plus, mimo-v2.5-pro], got %+v", result.Fallbacks)
	}
	chain := result.GetModelChain()
	if len(chain) != 3 {
		t.Errorf("expected 3-element chain (primary + 2 default fallbacks), got %d", len(chain))
	}
}

func TestRouteWithOverride_NoFallbacksAnywhere(t *testing.T) {
	cfg := &config.Config{
		ModelOverrides: map[string]config.ModelConfig{
			"kimi-k2.6": {Provider: "opencode-go", ModelID: "kimi-k2.6"},
		},
		// Both the override key and "default" are missing.
	}

	router := NewModelRouter(newTestAtomicConfig(cfg))

	result, ok := router.RouteWithOverride("kimi-k2.6")
	if !ok {
		t.Fatal("expected RouteWithOverride to match")
	}
	if len(result.Fallbacks) != 0 {
		t.Errorf("expected empty fallbacks, got %+v", result.Fallbacks)
	}
	chain := result.GetModelChain()
	if len(chain) != 1 {
		t.Errorf("expected 1-element chain, got %d", len(chain))
	}
}

func TestUnknownProvider(t *testing.T) {
	catalogPath := writeTestCatalog(t)
	cfg := &config.Config{
		RespectRequestedModel: boolPtr(true),
		Models: map[string]config.ModelConfig{
			"default": {
				Provider:    "opencode-go",
				ModelID:     "kimi-k2.6",
				Temperature: 0.5,
				MaxTokens:   8192,
			},
		},
		Fallbacks: map[string][]config.ModelConfig{
			"default": {{Provider: "opencode-go", ModelID: "qwen3.5-plus"}},
		},
	}
	atomic := newTestAtomicConfig(cfg)
	router := NewModelRouterWithCatalog(atomic, catalogPath)

	t.Run("unknown provider in canonical reference returns ErrUnknownProvider", func(t *testing.T) {
		_, _, err := router.resolveRequestedModel(cfg, "deepseek/deepseek-v4-flash@nonexistent-provider", false)
		if err == nil {
			t.Fatal("expected error for unknown provider, got nil")
		}
		if !errors.Is(err, ErrUnknownProvider) {
			t.Fatalf("expected error to wrap ErrUnknownProvider, got %v", err)
		}
	})

	t.Run("unknown short id falls back silently to opencode-go", func(t *testing.T) {
		result, ok, err := router.resolveRequestedModel(cfg, "totally-unknown-short-id", false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Fatal("expected resolveRequestedModel to match")
		}
		if result.Primary.Provider != "opencode-go" {
			t.Errorf("expected provider opencode-go, got %q", result.Primary.Provider)
		}
		if result.Primary.ModelID != "totally-unknown-short-id" {
			t.Errorf("expected model_id totally-unknown-short-id, got %q", result.Primary.ModelID)
		}
	})
}

func TestResolveRequestedModel(t *testing.T) {
	catalogPath := writeTestCatalog(t)
	// Verify the fixture loads so the test failures are not misleading.
	if _, err := catalog.Load(catalogPath); err != nil {
		t.Fatalf("test catalog fixture is invalid: %v", err)
	}

	cfg := &config.Config{
		RespectRequestedModel: boolPtr(true),
		Models: map[string]config.ModelConfig{
			"default": {
				Provider:    "opencode-go",
				ModelID:     "kimi-k2.6",
				Temperature: 0.5,
				MaxTokens:   8192,
			},
			"custom-model": {
				Provider:    "opencode-go",
				ModelID:     "custom-model",
				Temperature: 0.3,
				MaxTokens:   2048,
			},
			"deepseek-v4-pro": {
				Provider:    "opencode-go",
				ModelID:     "deepseek-v4-pro",
				Temperature: 0.7,
				MaxTokens:   8192,
			},
		},
		Fallbacks: map[string][]config.ModelConfig{
			"default": {{Provider: "opencode-go", ModelID: "qwen3.5-plus"}},
		},
	}
	atomic := newTestAtomicConfig(cfg)

	tests := []struct {
		name           string
		requestedModel string
		needsVision    bool
		catalogPath    string
		wantProvider   string
		wantModelID    string
		wantModelRef   string
		wantErr        bool
	}{
		{
			name:           "lab/model@provider resolves through catalog",
			requestedModel: "deepseek/deepseek-v4-flash@opencode-go",
			catalogPath:    catalogPath,
			wantProvider:   "opencode-go",
			wantModelID:    "deepseek-v4-flash",
			wantModelRef:   "deepseek/deepseek-v4-flash@opencode-go",
		},
		{
			name:           "short id resolves through catalog",
			requestedModel: "deepseek-v4-flash",
			catalogPath:    catalogPath,
			wantProvider:   "opencode-go",
			wantModelID:    "deepseek-v4-flash",
			wantModelRef:   "deepseek-v4-flash",
		},
		{
			name:           "mixed-case known model resolves to configured canonical ID",
			requestedModel: "DeepSeek-V4-Pro",
			wantProvider:   "opencode-go",
			wantModelID:    "deepseek-v4-pro",
			wantModelRef:   "",
		},
		{
			name:           "config model takes precedence over catalog",
			requestedModel: "custom-model",
			catalogPath:    catalogPath,
			wantProvider:   "opencode-go",
			wantModelID:    "custom-model",
			wantModelRef:   "",
		},
		{
			name:           "unknown model without catalog uses legacy fallback",
			requestedModel: "some-unknown-model",
			catalogPath:    "",
			wantProvider:   "opencode-go",
			wantModelID:    "some-unknown-model",
			wantModelRef:   "",
		},
		{
			name:           "vision request for non-vision catalog model returns error",
			requestedModel: "glm-5",
			needsVision:    true,
			catalogPath:    catalogPath,
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := NewModelRouterWithCatalog(atomic, tt.catalogPath)
			result, ok, err := router.resolveRequestedModel(cfg, tt.requestedModel, tt.needsVision)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !ok {
				t.Fatalf("expected resolveRequestedModel to match")
			}
			if result.Primary.Provider != tt.wantProvider {
				t.Errorf("expected provider %q, got %q", tt.wantProvider, result.Primary.Provider)
			}
			if result.Primary.ModelID != tt.wantModelID {
				t.Errorf("expected model_id %q, got %q", tt.wantModelID, result.Primary.ModelID)
			}
			if result.Primary.ModelRef != tt.wantModelRef {
				t.Errorf("expected model_ref %q, got %q", tt.wantModelRef, result.Primary.ModelRef)
			}
		})
	}
}

func TestRoute_CanonicalAndShortRefs(t *testing.T) {
	catalogPath := writeTestCatalog(t)

	cfg := &config.Config{
		RespectRequestedModel: boolPtr(true),
		Models: map[string]config.ModelConfig{
			"default": {
				Provider:    "opencode-go",
				ModelID:     "kimi-k2.6",
				Temperature: 0.5,
				MaxTokens:   8192,
			},
		},
		Fallbacks: map[string][]config.ModelConfig{
			"default": {
				{Provider: "opencode-go", ModelID: "qwen3.5-plus"},
			},
		},
	}
	atomic := newTestAtomicConfig(cfg)

	tests := []struct {
		name         string
		requested    string
		wantProvider string
		wantModelID  string
		wantModelRef string
	}{
		{
			name:         "canonical lab/model@provider",
			requested:    "deepseek/deepseek-v4-flash@opencode-go",
			wantProvider: "opencode-go",
			wantModelID:  "deepseek-v4-flash",
			wantModelRef: "deepseek/deepseek-v4-flash@opencode-go",
		},
		{
			name:         "short id resolves to first enabled provider",
			requested:    "deepseek-v4-flash",
			wantProvider: "opencode-go",
			wantModelID:  "deepseek-v4-flash",
			wantModelRef: "deepseek-v4-flash",
		},
		{
			name:         "short id with explicit provider",
			requested:    "kimi-k2.6@openrouter",
			wantProvider: "openrouter",
			wantModelID:  "kimi-k2.6",
			wantModelRef: "kimi-k2.6@openrouter",
		},
		{
			name:         "provider-qualified short id",
			requested:    "glm-5@openrouter",
			wantProvider: "openrouter",
			wantModelID:  "glm-5",
			wantModelRef: "glm-5@openrouter",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := NewModelRouterWithCatalog(atomic, catalogPath)
			result, err := router.Route([]MessageContent{{Role: "user", Content: "Hello"}}, 100, tt.requested)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Scenario != ScenarioDefault {
				t.Errorf("expected scenario %q, got %q", ScenarioDefault, result.Scenario)
			}
			if result.Primary.Provider != tt.wantProvider {
				t.Errorf("expected provider %q, got %q", tt.wantProvider, result.Primary.Provider)
			}
			if result.Primary.ModelID != tt.wantModelID {
				t.Errorf("expected model_id %q, got %q", tt.wantModelID, result.Primary.ModelID)
			}
			if result.Primary.ModelRef != tt.wantModelRef {
				t.Errorf("expected model_ref %q, got %q", tt.wantModelRef, result.Primary.ModelRef)
			}
		})
	}
}

func TestRoute_ModelOverridesPrecedence(t *testing.T) {
	catalogPath := writeTestCatalog(t)

	cfg := &config.Config{
		ModelOverrides: map[string]config.ModelConfig{
			"deepseek/deepseek-v4-flash@opencode-go": {
				Provider:    "opencode-zen",
				ModelID:     "claude-sonnet-4.5",
				Temperature: 0.2,
				MaxTokens:   4096,
			},
			"kimi-k2.6": {
				Provider:    "openrouter",
				ModelID:     "kimi-k2.6-or",
				Temperature: 0.1,
				MaxTokens:   1024,
			},
		},
		Fallbacks: map[string][]config.ModelConfig{
			"default": {
				{Provider: "opencode-go", ModelID: "qwen3.5-plus"},
			},
		},
	}
	atomic := newTestAtomicConfig(cfg)
	router := NewModelRouterWithCatalog(atomic, catalogPath)

	tests := []struct {
		name         string
		requested    string
		wantMatch    bool
		wantModelID  string
		wantProvider string
	}{
		{
			name:         "canonical override key matches",
			requested:    "deepseek/deepseek-v4-flash@opencode-go",
			wantMatch:    true,
			wantModelID:  "claude-sonnet-4.5",
			wantProvider: "opencode-zen",
		},
		{
			name:         "short override key matches",
			requested:    "kimi-k2.6",
			wantMatch:    true,
			wantModelID:  "kimi-k2.6-or",
			wantProvider: "openrouter",
		},
		{
			name:      "unrelated canonical ref does not match",
			requested: "kimi-k2.6@openrouter",
			wantMatch: false,
		},
		{
			name:      "unrelated short id does not match",
			requested: "glm-5",
			wantMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, ok := router.RouteWithOverride(tt.requested)
			if ok != tt.wantMatch {
				t.Fatalf("expected match=%v, got %v", tt.wantMatch, ok)
			}
			if !tt.wantMatch {
				return
			}
			if result.Scenario != ScenarioOverride {
				t.Errorf("expected scenario %q, got %q", ScenarioOverride, result.Scenario)
			}
			if result.Primary.ModelID != tt.wantModelID {
				t.Errorf("expected model_id %q, got %q", tt.wantModelID, result.Primary.ModelID)
			}
			if result.Primary.Provider != tt.wantProvider {
				t.Errorf("expected provider %q, got %q", tt.wantProvider, result.Primary.Provider)
			}
		})
	}
}

func TestRouteWithFamilyOverride_MatchesSubstring(t *testing.T) {
	cfg := &config.Config{
		ModelFamilyOverrides: map[string]config.ModelConfig{
			"opus":   {Provider: "opencode-go", ModelID: "glm-5.1", Temperature: 0.4, MaxTokens: 8192},
			"sonnet": {Provider: "opencode-go", ModelID: "kimi-k2.6"},
			"haiku":  {Provider: "opencode-go", ModelID: "qwen3.7-plus"},
		},
		Fallbacks: map[string][]config.ModelConfig{
			"opus": {
				{Provider: "opencode-go", ModelID: "kimi-k2.6"},
			},
		},
	}
	router := NewModelRouter(newTestAtomicConfig(cfg))

	tests := []struct {
		name        string
		requested   string
		wantModelID string
	}{
		{"versioned opus", "claude-opus-4-20250514", "glm-5.1"},
		{"versioned sonnet", "claude-sonnet-4-5-20250929", "kimi-k2.6"},
		{"versioned haiku", "claude-haiku-4-5-20251001", "qwen3.7-plus"},
		{"mixed case", "Claude-OPUS-4", "glm-5.1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, ok := router.RouteWithFamilyOverride(tt.requested)
			if !ok {
				t.Fatalf("expected family override to match %q", tt.requested)
			}
			if result.Primary.ModelID != tt.wantModelID {
				t.Errorf("expected model_id %q, got %q", tt.wantModelID, result.Primary.ModelID)
			}
			if result.Scenario != ScenarioOverride {
				t.Errorf("expected ScenarioOverride, got %s", result.Scenario)
			}
		})
	}
}

func TestRouteWithFamilyOverride_FallbacksByFamilyThenDefault(t *testing.T) {
	cfg := &config.Config{
		ModelFamilyOverrides: map[string]config.ModelConfig{
			"opus":   {Provider: "opencode-go", ModelID: "glm-5.1"},
			"sonnet": {Provider: "opencode-go", ModelID: "kimi-k2.6"},
		},
		Fallbacks: map[string][]config.ModelConfig{
			"opus":    {{Provider: "opencode-go", ModelID: "glm-5"}},
			"default": {{Provider: "opencode-go", ModelID: "qwen3.5-plus"}},
		},
	}
	router := NewModelRouter(newTestAtomicConfig(cfg))

	opus, ok := router.RouteWithFamilyOverride("claude-opus-4")
	if !ok {
		t.Fatal("expected opus match")
	}
	if len(opus.Fallbacks) != 1 || opus.Fallbacks[0].ModelID != "glm-5" {
		t.Errorf("expected family fallback [glm-5], got %+v", opus.Fallbacks)
	}

	sonnet, ok := router.RouteWithFamilyOverride("claude-sonnet-4")
	if !ok {
		t.Fatal("expected sonnet match")
	}
	if len(sonnet.Fallbacks) != 1 || sonnet.Fallbacks[0].ModelID != "qwen3.5-plus" {
		t.Errorf("expected default fallback [qwen3.5-plus], got %+v", sonnet.Fallbacks)
	}
}

func TestRouteWithFamilyOverride_NoMatch(t *testing.T) {
	cfg := &config.Config{
		ModelFamilyOverrides: map[string]config.ModelConfig{
			"opus": {Provider: "opencode-go", ModelID: "glm-5.1"},
		},
	}
	router := NewModelRouter(newTestAtomicConfig(cfg))

	if _, ok := router.RouteWithFamilyOverride("gpt-4o"); ok {
		t.Error("expected no family match for gpt-4o")
	}
}

func TestRouteWithFamilyOverride_NilMap(t *testing.T) {
	router := NewModelRouter(newTestAtomicConfig(&config.Config{}))
	if _, ok := router.RouteWithFamilyOverride("claude-opus-4"); ok {
		t.Error("expected no match for nil ModelFamilyOverrides map (must not panic)")
	}
}

func TestRouteWithFamilyOverride_LongestKeyWins(t *testing.T) {
	// Overlapping keys: a request containing both should match the longer key
	// for deterministic behavior.
	cfg := &config.Config{
		ModelFamilyOverrides: map[string]config.ModelConfig{
			"opus":      {Provider: "opencode-go", ModelID: "short-match"},
			"opus-mini": {Provider: "opencode-go", ModelID: "long-match"},
		},
	}
	router := NewModelRouter(newTestAtomicConfig(cfg))

	result, ok := router.RouteWithFamilyOverride("claude-opus-mini-4")
	if !ok {
		t.Fatal("expected a match")
	}
	if result.Primary.ModelID != "long-match" {
		t.Errorf("expected longest key to win (long-match), got %q", result.Primary.ModelID)
	}
}

func TestCostBasedRouting_SelectsCheapest(t *testing.T) {
	catalogPath := filepath.Join("testdata", "selector_catalog.json")
	cfg := &config.Config{
		APIKey:                 "global-key",
		EnableCostBasedRouting: true,
		Models: map[string]config.ModelConfig{
			"default": {Provider: "opencode-go", ModelID: "legacy-default"},
			"complex": {Provider: "opencode-go", ModelID: "legacy-complex"},
		},
		CostRouting: &config.CostRoutingConfig{Scenarios: selectorTestScenarios()},
	}
	atomic := config.NewAtomicConfig(cfg, "/tmp/test-config.json")
	router := NewModelRouterWithCatalog(atomic, catalogPath)

	result, err := router.Route([]MessageContent{{Role: "user", Content: "Hello"}}, 100, "")
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}
	if result.Primary.ModelID != "cheap-no-tools" {
		t.Errorf("default scenario: expected cheap-no-tools, got %s", result.Primary.ModelID)
	}

	complex, err := router.Route([]MessageContent{{Role: "user", Content: "Architect a new microservice"}}, 100, "")
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}
	if complex.Primary.ModelID != "large-context" {
		t.Errorf("complex scenario: expected large-context, got %s", complex.Primary.ModelID)
	}
}

func TestCostBasedRouting_DisabledUsesLegacy(t *testing.T) {
	catalogPath := filepath.Join("testdata", "selector_catalog.json")
	cfg := &config.Config{
		APIKey:                 "global-key",
		EnableCostBasedRouting: false,
		Models: map[string]config.ModelConfig{
			"default": {Provider: "opencode-go", ModelID: "legacy-default"},
		},
	}
	atomic := config.NewAtomicConfig(cfg, "/tmp/test-config.json")
	router := NewModelRouterWithCatalog(atomic, catalogPath)

	result, err := router.Route([]MessageContent{{Role: "user", Content: "Hello"}}, 100, "")
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}
	if result.Primary.ModelID != "legacy-default" {
		t.Errorf("expected legacy-default, got %s", result.Primary.ModelID)
	}
}

func TestCostBasedRouting_FallsBackWhenNoMatch(t *testing.T) {
	catalogPath := filepath.Join("testdata", "selector_catalog.json")
	cfg := &config.Config{
		APIKey:                 "global-key",
		EnableCostBasedRouting: true,
		Models: map[string]config.ModelConfig{
			"background": {Provider: "opencode-go", ModelID: "legacy-background"},
		},
		// No catalog model can satisfy this floor, so the selector must find no
		// candidate and routing must fall back to the legacy scenario model.
		CostRouting: &config.CostRoutingConfig{Scenarios: map[string]config.CostScenario{
			"background": {MinContextWindow: 99_000_000},
		}},
	}
	atomic := config.NewAtomicConfig(cfg, "/tmp/test-config.json")
	router := NewModelRouterWithCatalog(atomic, catalogPath)

	result, err := router.Route([]MessageContent{{Role: "user", Content: "what is the time"}}, 100, "")
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}
	if result.Primary.ModelID != "legacy-background" {
		t.Errorf("expected fallback legacy-background, got %s", result.Primary.ModelID)
	}
}

func TestCostBasedRouting_RouteForStreaming(t *testing.T) {
	catalogPath := filepath.Join("testdata", "selector_catalog.json")
	cfg := &config.Config{
		APIKey:                 "global-key",
		EnableCostBasedRouting: true,
		Models: map[string]config.ModelConfig{
			"fast": {Provider: "opencode-go", ModelID: "legacy-fast"},
		},
	}
	atomic := config.NewAtomicConfig(cfg, "/tmp/test-config.json")
	router := NewModelRouterWithCatalog(atomic, catalogPath)

	result, err := router.RouteForStreaming([]MessageContent{{Role: "user", Content: "Hello"}}, 100, "")
	if err != nil {
		t.Fatalf("RouteForStreaming failed: %v", err)
	}
	if result.Primary.ModelID != "cheap-no-tools" {
		t.Errorf("expected cheap-no-tools, got %s", result.Primary.ModelID)
	}
}

func TestRoute_LegacyConfigFixtures(t *testing.T) {
	t.Run("example config fixture", func(t *testing.T) {
		t.Setenv("ROUTATIC_PROXY_API_KEY", "test-key")

		cfgPath := "../../configs/config.example.json"
		cfg, err := config.LoadFromPath(cfgPath)
		if err != nil {
			t.Fatalf("failed to load example config: %v", err)
		}
		atomic := config.NewAtomicConfig(cfg, cfgPath)
		router := NewModelRouter(atomic)

		messages := []MessageContent{{Role: "user", Content: "Hello"}}

		result, err := router.Route(messages, 100, "")
		if err != nil {
			t.Fatalf("Route failed: %v", err)
		}
		if result.Primary.ModelID != "deepseek-v4-pro" {
			t.Errorf("expected primary deepseek-v4-pro, got %s", result.Primary.ModelID)
		}

		streamResult, err := router.RouteForStreaming(messages, 100, "")
		if err != nil {
			t.Fatalf("RouteForStreaming failed: %v", err)
		}
		if streamResult.Primary.ModelID != "deepseek-v4-flash" {
			t.Errorf("expected streaming primary deepseek-v4-flash, got %s", streamResult.Primary.ModelID)
		}
	})

	t.Run("inline legacy fixture", func(t *testing.T) {
		cfg := &config.Config{
			RespectRequestedModel: boolPtr(false),
			Models: map[string]config.ModelConfig{
				"default": {Provider: "opencode-go", ModelID: "kimi-k2.6"},
				"fast":    {Provider: "opencode-go", ModelID: "qwen3.5-plus"},
			},
			Fallbacks: map[string][]config.ModelConfig{
				"default": {{Provider: "opencode-go", ModelID: "glm-5.1"}},
				"fast":    {{Provider: "opencode-go", ModelID: "deepseek-v4-flash"}},
			},
		}
		atomic := newTestAtomicConfig(cfg)
		router := NewModelRouter(atomic)

		messages := []MessageContent{{Role: "user", Content: "Hello"}}

		result, err := router.Route(messages, 100, "")
		if err != nil {
			t.Fatalf("Route failed: %v", err)
		}
		if result.Primary.ModelID != "kimi-k2.6" {
			t.Errorf("expected primary kimi-k2.6, got %s", result.Primary.ModelID)
		}

		streamResult, err := router.RouteForStreaming(messages, 100, "")
		if err != nil {
			t.Fatalf("RouteForStreaming failed: %v", err)
		}
		if streamResult.Primary.ModelID != "qwen3.5-plus" {
			t.Errorf("expected streaming primary qwen3.5-plus, got %s", streamResult.Primary.ModelID)
		}
	})
}

func TestListModels_MergesConfigAndOverrides(t *testing.T) {
	cfg := &config.Config{
		Models: map[string]config.ModelConfig{
			"default":   {Provider: "opencode-go", ModelID: "kimi-k2.6"},
			"kimi-k2.6": {Provider: "opencode-go", ModelID: "kimi-k2.6"},
		},
		ModelOverrides: map[string]config.ModelConfig{
			"claude-sonnet-4-5-20250929": {Provider: "opencode-zen", ModelID: "minimax"},
		},
	}

	router := NewModelRouter(newTestAtomicConfig(cfg))
	models := router.ListModels(context.Background())

	byID := make(map[string]ModelInfo, len(models))
	for _, m := range models {
		byID[m.ID] = m
	}

	for _, want := range []string{"default", "kimi-k2.6", "claude-sonnet-4-5-20250929"} {
		if _, ok := byID[want]; !ok {
			t.Errorf("expected model %q in listing, got %+v", want, models)
		}
	}
	if got := byID["claude-sonnet-4-5-20250929"].Provider; got != "opencode-zen" {
		t.Errorf("expected override provider opencode-zen, got %q", got)
	}

	// Sorted ascending by ID.
	for i := 1; i < len(models); i++ {
		if models[i-1].ID > models[i].ID {
			t.Errorf("models not sorted: %q before %q", models[i-1].ID, models[i].ID)
		}
	}
}

func TestListModels_OverrideProviderWinsOnCollision(t *testing.T) {
	// A key present in both models and model_overrides must surface the
	// override's provider, matching routing precedence (model_overrides wins).
	cfg := &config.Config{
		Models: map[string]config.ModelConfig{
			"claude-sonnet-4-5-20250929": {Provider: "opencode-go", ModelID: "kimi-k2.6"},
		},
		ModelOverrides: map[string]config.ModelConfig{
			"claude-sonnet-4-5-20250929": {Provider: "opencode-zen", ModelID: "minimax"},
		},
	}

	router := NewModelRouter(newTestAtomicConfig(cfg))
	models := router.ListModels(context.Background())

	var got string
	for _, m := range models {
		if m.ID == "claude-sonnet-4-5-20250929" {
			got = m.Provider
		}
	}
	if got != "opencode-zen" {
		t.Errorf("expected override provider opencode-zen to win, got %q", got)
	}
}

func TestListModels_Empty(t *testing.T) {
	router := NewModelRouter(newTestAtomicConfig(&config.Config{}))
	if models := router.ListModels(context.Background()); len(models) != 0 {
		t.Errorf("expected no models, got %+v", models)
	}
}

// A scoped listing must not offer an id the routing chain would drop: the
// picker entry would fail the moment it is used. This covers the alias sources,
// whose names say nothing about the platform that serves them, and the catalog.
func TestListModels_ScopedToActiveSite(t *testing.T) {
	cfg := &config.Config{
		ActiveSite: "cline-pass",
		Models: map[string]config.ModelConfig{
			"default":     {Provider: "commandcode", ModelID: "deepseek/deepseek-v4-pro"},
			"local-alias": {Provider: "cline-pass", ModelID: "cline-pass/glm-5.3"},
		},
		ModelOverrides: map[string]config.ModelConfig{
			"claude-opus-4-8":  {Provider: "commandcode", ModelID: "deepseek/deepseek-v4-pro"},
			"claude-haiku-4-5": {Provider: "cline-pass", ModelID: "cline-pass/kimi-k3"},
		},
	}

	router := NewModelRouter(newTestAtomicConfig(cfg))
	models := router.ListModels(context.Background())

	byID := make(map[string]ModelInfo, len(models))
	for _, m := range models {
		byID[m.ID] = m
	}
	// The active site's targets stay, whatever the alias is called.
	for _, want := range []string{"local-alias", "claude-haiku-4-5"} {
		if _, ok := byID[want]; !ok {
			t.Errorf("%q is served by the active site but was not listed", want)
		}
	}
	// Another platform's targets go, even under a Claude alias and even as the
	// default model.
	for _, unwanted := range []string{"default", "claude-opus-4-8"} {
		if _, ok := byID[unwanted]; ok {
			t.Errorf("%q belongs to another platform and must not be listed while cline-pass is active", unwanted)
		}
	}
	for id, info := range byID {
		if info.Provider != "cline-pass" {
			t.Errorf("%q was listed with provider %q", id, info.Provider)
		}
	}

	// The same config unscoped still lists everything: the filter is the scope,
	// not a permanent narrowing.
	unscoped := NewModelRouter(newTestAtomicConfig(&config.Config{
		Models:         cfg.Models,
		ModelOverrides: cfg.ModelOverrides,
	}))
	if got := len(unscoped.ListModels(context.Background())); got != 4 {
		t.Errorf("unscoped listing = %d entries, want all 4", got)
	}
}

// An underscore spelling is the accepted legacy form of a platform id, and the
// scope must normalize it the same way routing does.
func TestListModels_NormalizesActiveSiteSpelling(t *testing.T) {
	cfg := &config.Config{
		ActiveSite: "opencode_go",
		Models: map[string]config.ModelConfig{
			"go-model":  {Provider: "opencode-go", ModelID: "kimi-k2.6"},
			"zen-model": {Provider: "opencode-zen", ModelID: "minimax"},
		},
	}
	models := NewModelRouter(newTestAtomicConfig(cfg)).ListModels(context.Background())
	if len(models) != 1 || models[0].ID != "go-model" {
		t.Errorf("models = %+v, want only the opencode-go target", models)
	}
}

// A platform's own roster is the authority on which of its models still exist.
//
// models.dev is a third-party mirror and lags: it still carried four cline-pass
// models the platform had retired. Both sources reached the listing, so the
// picker kept offering models that answer an error when called.
//
// The catalog here holds a live model and a retired one; the roster lists only
// the live one.
func TestListModels_DropsCatalogModelsThePlatformNoLongerPublishes(t *testing.T) {
	const (
		live    = "cline-pass/deepseek-v4.1-flash"
		retired = "cline-pass/kimi-k2.6"
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/ai/cline/recommended-models" {
			t.Errorf("roster requested from %q", r.URL.Path)
			return
		}
		_, _ = io.WriteString(w, `{"clinePass":[{"id":"`+live+`","name":"DeepSeek V4.1 Flash"}]}`)
	}))
	defer srv.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "catalog.json")
	body := `{
	  "providers": {"cline-pass": {"name": "cline-pass"}},
	  "models": {
	    "cline-pass/deepseek-v4.1-flash": {"name": "DeepSeek V4.1 Flash"},
	    "cline-pass/kimi-k2.6": {"name": "Kimi K2.6"}
	  }
	}`
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		ClinePass: config.ClinePassConfig{
			APIKey:  "synthetic-key",
			BaseURL: srv.URL + "/api/v1/chat/completions",
		},
	}
	router := NewModelRouterWithCatalog(newTestAtomicConfig(cfg), path)

	byID := map[string]bool{}
	for _, m := range router.ListModels(context.Background()) {
		byID[m.ID] = true
	}
	if !byID[live] {
		t.Errorf("the model the platform still publishes must be listed; got %v", byID)
	}
	if byID[retired] {
		t.Error("a model the platform retired must not reach the picker, even though the catalog still carries it")
	}
}

// A roster that could not be fetched must not restrict the catalog.
//
// An unreachable platform reports no models, and reading that as "the platform
// has none" would empty its section of the picker on every blip - the same
// distinction FetchSiteModels draws between an empty list and a failed fetch.
func TestListModels_KeepsCatalogModelsWhenTheRosterIsUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	base := srv.URL + "/api/v1/chat/completions"
	srv.Close() // now unreachable

	dir := t.TempDir()
	path := filepath.Join(dir, "catalog.json")
	body := `{
	  "providers": {"cline-pass": {"name": "cline-pass"}},
	  "models": {"cline-pass/kimi-k2.6": {"name": "Kimi K2.6"}}
	}`
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{ClinePass: config.ClinePassConfig{APIKey: "synthetic-key", BaseURL: base}}
	router := NewModelRouterWithCatalog(newTestAtomicConfig(cfg), path)

	byID := map[string]bool{}
	for _, m := range router.ListModels(context.Background()) {
		byID[m.ID] = true
	}
	if !byID["cline-pass/kimi-k2.6"] {
		t.Error("an unreachable roster must leave the catalog's models alone, not empty the picker")
	}
}

// A model whose capabilities live under another namespace must still resolve.
//
// Cline publishes "cline-pass/deepseek-v4.1-flash-1017"; models.dev files the
// same model as "deepseek/deepseek-v4.1-flash-1017". Neither catalog lookup
// strips anything, so the id the listing just handed us matched nothing, the
// hardcoded metadata table (which does not know this model) answered instead,
// and every request carrying a new image died on "no eligible model for
// request capacity" - a 500 for a vision-capable model.
func TestPublishedByActiveSite_ResolvesCapabilitiesAcrossNamespaces(t *testing.T) {
	const listingID = "cline-pass/deepseek-v4.1-flash-1017"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/ai/cline/recommended-models" {
			t.Errorf("roster requested from %q", r.URL.Path)
			return
		}
		_, _ = io.WriteString(w, `{"clinePass":[{"id":"`+listingID+`","name":"`+listingID+`"}]}`)
	}))
	defer srv.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "catalog.json")
	// Several providers carry this model, exactly as the real catalog does - it
	// lists the id under 38 of them. One row would not reproduce the defect:
	// ResolveShort answers a single match and only refuses a tie.
	body := `{
	  "providers": {
	    "cline-pass":   {"name": "cline-pass",   "enabled": true},
	    "deepseek":     {"name": "deepseek",     "enabled": true},
	    "nearai":       {"name": "nearai",       "enabled": true},
	    "fireworks-ai": {"name": "fireworks-ai", "enabled": true}
	  },
	  "models": {
	    "deepseek/deepseek-v4.1-flash-1017": {
	      "id": "deepseek/deepseek-v4.1-flash-1017",
	      "name": "DeepSeek V4.1 Flash 1017",
	      "limit": {"context": 1000000, "output": 384000},
	      "tool_call": true,
	      "modalities": {"input": ["text", "image"], "output": ["text"]}
	    },
	    "nearai/deepseek-v4.1-flash-1017": {
	      "id": "nearai/deepseek-v4.1-flash-1017",
	      "name": "DeepSeek V4.1 Flash 1017",
	      "limit": {"context": 1000000, "output": 384000},
	      "tool_call": true,
	      "modalities": {"input": ["text", "image"], "output": ["text"]}
	    },
	    "fireworks-ai/deepseek-v4.1-flash-1017": {
	      "id": "fireworks-ai/deepseek-v4.1-flash-1017",
	      "name": "DeepSeek V4.1 Flash 1017",
	      "limit": {"context": 1000000, "output": 384000},
	      "tool_call": true,
	      "modalities": {"input": ["text", "image"], "output": ["text"]}
	    }
	  }
	}`
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		ActiveSite: "cline-pass",
		ClinePass:  config.ClinePassConfig{APIKey: "synthetic-key", BaseURL: srv.URL + "/api/v1/chat/completions"},
	}
	router := NewModelRouterWithCatalog(newTestAtomicConfig(cfg), path)

	target, ok := router.PublishedByActiveSite(context.Background(), listingID)
	if !ok {
		t.Fatalf("%s is in the platform's own roster and must resolve", listingID)
	}
	if !target.Vision {
		t.Error("the catalog records this model as accepting images; a false here makes every image request unfilterable")
	}
	if target.ContextWindow != 1000000 {
		t.Errorf("ContextWindow = %d, want the catalog's 1000000", target.ContextWindow)
	}

	// The platform and the id are the listing's, not the catalog's: adopting the
	// catalog's would move the request to another platform's credential, which on
	// ClinePass is a different billing pool rather than another spelling.
	if target.Provider != "cline-pass" {
		t.Errorf("Provider = %q, want the active site", target.Provider)
	}
	if target.ModelID != listingID {
		t.Errorf("ModelID = %q, want the id the platform published", target.ModelID)
	}
	if target.ModelRef == "" {
		t.Error("ModelRef must be set, or ResolveModelConfig overwrites the catalog's capabilities " +
			"with the built-in family table, which does not know this model")
	}
}

// A vendor-qualified id on a platform that does not prefix its own must not be
// stripped: that would resolve the model on some other platform entirely.
func TestCatalogResolved_LeavesVendorNamespacesAlone(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "catalog.json")
	body := `{
	  "providers": {"opencode-go": {"name": "opencode-go", "enabled": true}},
	  "models": {"opencode-go/deepseek-v4-flash": {"name": "DeepSeek V4 Flash", "modalities": {"input": ["text"], "output": ["text"]}}}
	}`
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	router := NewModelRouterWithCatalog(newTestAtomicConfig(&config.Config{}), path)

	// "deepseek" is not this platform, so nothing is stripped and the vendor
	// namespace resolves through the catalog's own rules instead.
	resolved, err := router.catalogResolved(context.Background(), "opencode-go", "opencode-go/deepseek-v4-flash")
	if err != nil {
		t.Fatalf("expected the platform's own prefix to resolve: %v", err)
	}
	if resolved.Provider != "opencode-go" {
		t.Errorf("Provider = %q, want opencode-go", resolved.Provider)
	}
}

// The same namespace mismatch reaches the client path too. A client that names
// the model the way the platform lists it must get the catalog's capabilities
// rather than the bare legacy fallback, or a vision request is refused here
// before the platform is ever asked.
func TestResolveRequestedModel_ResolvesPlatformPrefixedId(t *testing.T) {
	const requested = "cline-pass/deepseek-v4.1-flash-1017"

	dir := t.TempDir()
	path := filepath.Join(dir, "catalog.json")
	body := `{
	  "providers": {
	    "cline-pass": {"name": "cline-pass", "enabled": true},
	    "deepseek":   {"name": "deepseek",   "enabled": true},
	    "nearai":     {"name": "nearai",     "enabled": true}
	  },
	  "models": {
	    "deepseek/deepseek-v4.1-flash-1017": {
	      "id": "deepseek/deepseek-v4.1-flash-1017",
	      "name": "DeepSeek V4.1 Flash 1017",
	      "limit": {"context": 1000000, "output": 384000},
	      "tool_call": true,
	      "modalities": {"input": ["text", "image"], "output": ["text"]}
	    },
	    "nearai/deepseek-v4.1-flash-1017": {
	      "id": "nearai/deepseek-v4.1-flash-1017",
	      "name": "DeepSeek V4.1 Flash 1017",
	      "limit": {"context": 1000000, "output": 384000},
	      "tool_call": true,
	      "modalities": {"input": ["text", "image"], "output": ["text"]}
	    }
	  }
	}`
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{RespectRequestedModel: boolPtr(true)}
	router := NewModelRouterWithCatalog(newTestAtomicConfig(cfg), path)

	// needsVision=true is the case that used to fail: without catalog
	// capabilities the model looks text-only and is rejected outright.
	result, ok, err := router.resolveRequestedModel(cfg, requested, true)
	if err != nil {
		t.Fatalf("a vision request against a vision-capable model must not be refused: %v", err)
	}
	if !ok {
		t.Fatal("expected the requested model to be honoured")
	}
	if !result.Primary.Vision {
		t.Error("Vision = false; the catalog says this model accepts images")
	}
	if result.Primary.ContextWindow != 1000000 {
		t.Errorf("ContextWindow = %d, want 1000000", result.Primary.ContextWindow)
	}
	// The client named this platform, so the catalog's vendor namespace must not
	// become the target - on ClinePass the prefix is the billing pool selector.
	if result.Primary.Provider != "cline-pass" {
		t.Errorf("Provider = %q, want cline-pass", result.Primary.Provider)
	}
	if result.Primary.ModelID != requested {
		t.Errorf("ModelID = %q, want the id the client sent (%q)", result.Primary.ModelID, requested)
	}
}

// A vendor namespace is not a platform prefix. Stripping one would look for a
// model named after its vendor and resolve to nothing, or worse, to whatever
// that name happens to match.
func TestSplitPlatformPrefix_IgnoresVendorNamespaces(t *testing.T) {
	for _, tc := range []struct{ in, wantPlatform, wantShort string }{
		{"cline-pass/deepseek-v4.1-flash-1017", "cline-pass", "deepseek-v4.1-flash-1017"},
		{"deepseek/deepseek-v4.1-flash-1017", "", ""},
		{"opencode-go/kimi-k3", "opencode-go", "kimi-k3"},
		{"kimi-k3", "", ""},
		{"cline-pass/", "", ""},
		{"/kimi-k3", "", ""},
	} {
		platform, short := splitPlatformPrefix(tc.in)
		if platform != tc.wantPlatform || short != tc.wantShort {
			t.Errorf("splitPlatformPrefix(%q) = (%q, %q), want (%q, %q)", tc.in, platform, short, tc.wantPlatform, tc.wantShort)
		}
	}
}

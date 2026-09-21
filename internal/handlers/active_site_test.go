package handlers

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/metrics"
	"github.com/routatic/proxy/internal/router"
	"github.com/routatic/proxy/internal/site"
	"github.com/routatic/proxy/internal/token"
)

// activeSiteHandler builds a handler whose only upstream is a stub publishing
// CommandCode's model list. Routing decisions are made before any inference
// request, so they can be observed through buildModelChain alone.
func activeSiteHandler(t *testing.T, cfg *config.Config) *MessagesHandler {
	t.Helper()
	atomicCfg := config.NewAtomicConfig(cfg, "/tmp/test-config.json")
	tokenCounter, err := token.NewCounter()
	if err != nil {
		t.Fatalf("NewCounter: %v", err)
	}
	handler := NewMessagesHandler(
		client.NewOpenCodeClient(atomicCfg, nil),
		newTestProviderRegistry(t, atomicCfg),
		router.NewModelRouter(atomicCfg),
		nil, tokenCounter, metrics.New(), nil, nil,
	)
	handler.logger = slog.Default()
	return handler
}

func siteConfig(t *testing.T, active string) *config.Config {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/provider/v1/models" {
			t.Errorf("unexpected upstream path %q", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"data":[{"id":"deepseek/deepseek-v4-flash","name":"DeepSeek V4 Flash"},{"id":"moonshotai/Kimi-K2.7-Code","name":"Kimi K2.7 Code"}]}`)
	}))
	t.Cleanup(upstream.Close)
	return &config.Config{
		ActiveSite: active,
		Models: map[string]config.ModelConfig{
			"default":  {Provider: "opencode-go", ModelID: "kimi-k2.6"},
			"alias-cc": {Provider: "commandcode", ModelID: "zai-org/GLM-5.2"},
		},
		ModelOverrides: map[string]config.ModelConfig{
			// A deliberate re-map: this id is named after a model CommandCode
			// also publishes, but the operator routed it to OpenCode Go.
			"deepseek/deepseek-v4-flash": {Provider: "opencode-go", ModelID: "kimi-k2.6"},
			// A configured name the active site cannot serve.
			"legacy-alias": {Provider: "opencode-go", ModelID: "kimi-k2.6"},
		},
		CommandCode: config.CommandCodeConfig{
			APIKey:  "synthetic-key",
			BaseURL: upstream.URL + "/provider/v1/chat/completions",
		},
	}
}

func chainFor(t *testing.T, handler *MessagesHandler, requested string) ([]config.ModelConfig, error) {
	t.Helper()
	chain, _, err := handler.buildModelChain(context.Background(), requested,
		[]router.MessageContent{{Role: "user", Content: "hi"}}, 100, false, 256, false, false)
	return chain, err
}

func TestActiveSitePublishedModelWinsOverConfiguredRemap(t *testing.T) {
	handler := activeSiteHandler(t, siteConfig(t, site.CommandCode))
	chain, err := chainFor(t, handler, "deepseek/deepseek-v4-flash")
	if err != nil {
		t.Fatal(err)
	}
	if len(chain) == 0 {
		t.Fatal("no chain")
	}
	// The listing is what the client chose from, so its choice is honoured
	// rather than re-mapped through the operator's alias.
	if chain[0].Provider != site.CommandCode || chain[0].ModelID != "deepseek/deepseek-v4-flash" {
		t.Fatalf("primary = %s/%s, want the published model on the active site", chain[0].Provider, chain[0].ModelID)
	}
	for _, target := range chain {
		if target.Provider != site.CommandCode {
			t.Fatalf("chain kept a target from another platform: %s/%s", target.Provider, target.ModelID)
		}
	}
}

func TestActiveSitePublishedModelsRespectCapacity(t *testing.T) {
	for _, tc := range []struct {
		name          string
		requested     string
		vision        bool
		tools         bool
		inputTokens   int
		wantPrimary   string
		wantNoTargets bool
	}{
		{"vision fallback", "deepseek/deepseek-v4-flash", true, false, 100, "kimi-k2.6", false},
		{"output limit", "deepseek/deepseek-v4-flash", false, false, 100, "deepseek/deepseek-v4-flash", false},
		{"exhausted context", "deepseek/deepseek-v4-flash", false, false, 1000000, "", true},
		{"tool-less fallback", "deepseek/deepseek-v4-flash", false, true, 100, "deepseek/deepseek-v4-flash", false},
		{"native vision ID", "moonshotai/Kimi-K2.7-Code", true, false, 100, "moonshotai/Kimi-K2.7-Code", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := siteConfig(t, site.CommandCode)
			cfg.Models["default"] = config.ModelConfig{
				Provider: site.CommandCode, ModelID: "kimi-k2.6", MaxTokens: 8192, SupportsTools: boolPtr(false),
			}
			h := activeSiteHandler(t, cfg)
			chain, _, err := h.buildModelChain(context.Background(), tc.requested,
				[]router.MessageContent{{Role: "user", Content: "hi"}}, tc.inputTokens, false, 64, tc.vision, tc.tools)
			if tc.wantNoTargets {
				if err == nil || len(chain) != 0 {
					t.Fatalf("exhausted models were accepted: chain=%+v err=%v", chain, err)
				}
				return
			}
			if err != nil || len(chain) == 0 {
				t.Fatalf("no eligible chain: %+v err=%v", chain, err)
			}
			if chain[0].ModelID != tc.wantPrimary {
				t.Fatalf("primary=%s, want %s", chain[0].ModelID, tc.wantPrimary)
			}
			for _, model := range chain {
				if model.Provider != site.CommandCode || model.MaxTokens != 64 ||
					(tc.vision && !model.Vision) || (tc.tools && !config.SupportsTools(model)) {
					t.Fatalf("request constraints bypassed: %+v", model)
				}
			}
		})
	}
}

func TestActiveSiteFiltersConfiguredTargets(t *testing.T) {
	handler := activeSiteHandler(t, siteConfig(t, site.CommandCode))
	chain, err := chainFor(t, handler, "alias-cc")
	if err != nil {
		t.Fatal(err)
	}
	if len(chain) == 0 || chain[0].Provider != site.CommandCode || chain[0].ModelID != "zai-org/GLM-5.2" {
		t.Fatalf("chain = %+v, want the site's own target", chain)
	}
	// models.default points at OpenCode Go and must not survive the scope.
	for _, target := range chain {
		if target.Provider != site.CommandCode {
			t.Fatalf("a target from another platform survived: %s/%s", target.Provider, target.ModelID)
		}
	}
}

func TestPublishedModelRetainsSameTargetConfiguration(t *testing.T) {
	for _, source := range []string{"models", "model_overrides", "default"} {
		t.Run(source, func(t *testing.T) {
			cfg := siteConfig(t, site.CommandCode)
			requested := "deepseek/deepseek-v4-flash"
			model := config.ModelConfig{Provider: site.CommandCode, ModelID: requested, Vision: true, MaxTokens: 32, ContextWindow: 32000}
			delete(cfg.ModelOverrides, requested)
			if source == "models" {
				cfg.Models[requested] = model
			} else if source == "default" {
				cfg.Models["default"] = model
			} else {
				cfg.ModelOverrides[requested] = model
			}
			h := activeSiteHandler(t, cfg)
			chain, _, err := h.buildModelChain(context.Background(), requested,
				[]router.MessageContent{{Role: "user", Content: "hi"}}, 100, false, 64, true, false)
			if err != nil || len(chain) == 0 {
				t.Fatalf("configured vision capability was lost: chain=%+v err=%v", chain, err)
			}
			if chain[0].ModelID != requested || chain[0].MaxTokens != 32 || chain[0].ContextWindow != 32000 {
				t.Fatalf("same-target configuration was lost: %+v", chain[0])
			}
		})
	}
}

// A request the operator's own configuration routes to a platform the active
// site excludes is their configuration not covering that site. Saying so beats
// quietly routing to a platform they did not choose.
func TestActiveSiteWithoutATargetFailsExplicitly(t *testing.T) {
	handler := activeSiteHandler(t, siteConfig(t, site.CommandCode))
	_, err := chainFor(t, handler, "legacy-alias")
	if err == nil {
		t.Fatal("a request with no target on the active site was routed anyway")
	}
	if !strings.Contains(err.Error(), site.CommandCode) {
		t.Fatalf("error does not name the active site: %v", err)
	}
}

// A model name no source knows is still the client's choice of model, and this
// proxy does not manage models. It goes to the active site and lets the platform
// answer for itself, rather than being sent to a platform the operator excluded
// or refused here on the platform's behalf.
func TestActiveSiteCarriesAnUnknownModelName(t *testing.T) {
	handler := activeSiteHandler(t, siteConfig(t, site.CommandCode))
	chain, err := chainFor(t, handler, "some-model-nobody-publishes")
	if err != nil {
		t.Fatal(err)
	}
	if len(chain) == 0 || chain[0].Provider != site.CommandCode || chain[0].ModelID != "some-model-nobody-publishes" {
		t.Fatalf("chain = %+v, want the name carried to the active site", chain)
	}
}

// An unset active site leaves routing exactly as it was: the field is an
// opt-in, and an existing config must not start filtering.
func TestUnsetActiveSiteKeepsEveryTarget(t *testing.T) {
	handler := activeSiteHandler(t, siteConfig(t, ""))
	chain, err := chainFor(t, handler, "deepseek/deepseek-v4-flash")
	if err != nil {
		t.Fatal(err)
	}
	// Without a scope the configured re-map is what wins.
	if len(chain) == 0 || chain[0].Provider != site.OpenCodeGo {
		t.Fatalf("chain = %+v, want the configured re-map to OpenCode Go", chain)
	}
	// And the OpenCode Go fallback stays available.
	var sawOpenCodeGo bool
	for _, target := range chain {
		if target.Provider == site.OpenCodeGo {
			sawOpenCodeGo = true
		}
	}
	if !sawOpenCodeGo {
		t.Fatal("an unset active site changed which targets are candidates")
	}
}

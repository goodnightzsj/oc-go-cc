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
		_, _ = io.WriteString(w, `{"data":[{"id":"deepseek/deepseek-v4-flash","name":"DeepSeek V4 Flash"}]}`)
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

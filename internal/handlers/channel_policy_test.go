package handlers

import (
	"errors"
	"net/http"
	"testing"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/internal/provider"
)

func TestRequiredChannelBeforeCapacityFiltering(t *testing.T) {
	for _, pin := range []bool{true, false} {
		cfg := &config.Config{
			ClinePass: config.ClinePassConfig{ChannelPinEnabled: pin, ChannelPin: "private"},
			Models: map[string]config.ModelConfig{
				"default": {Provider: "cline-pass", ModelID: "synthetic-limited", ContextWindow: 1},
			},
			Fallbacks: map[string][]config.ModelConfig{
				"default": {{Provider: "openrouter", ModelID: "synthetic-capable", ContextWindow: 65536}},
			},
		}
		h := newTestMessagesHandler(t, cfg)
		h.providerRegistry = core.NewProviderRegistry()
		if err := h.providerRegistry.Register(provider.NewClinePassProvider(config.NewAtomicConfig(cfg, ""), nil)); err != nil {
			t.Fatal(err)
		}
		chain, _, err := h.buildModelChain(t.Context(), "", nil, 512, false, 32, false, false)
		if pin {
			var apiErr *client.APIError
			if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusUnprocessableEntity || len(chain) != 0 {
				t.Fatalf("required route lost to capacity filter: chain=%v err=%v", chain, err)
			}
		} else if err != nil || len(chain) != 1 || chain[0].Provider != "openrouter" {
			t.Fatalf("capacity control did not replace insufficient primary: chain=%v err=%v", chain, err)
		}
	}
}

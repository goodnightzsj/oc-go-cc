package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/internal/metrics"
	"github.com/routatic/proxy/internal/provider"
	"github.com/routatic/proxy/internal/router"
	"github.com/routatic/proxy/pkg/types"
)

func TestStreamingPolicy(t *testing.T) {
	for _, tc := range []struct {
		name                             string
		status, keys, repeats, wantCalls int
	}{
		{"single-key-auth", 401, 1, 1, 1},
		{"multi-key-auth", 401, 2, 1, 2},
		{"single-key-forbidden", 403, 1, 1, 1},
		{"server-errors-open-circuit", 500, 1, 4, 6},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{APIKeys: []string{"synthetic-a"}}
			if tc.keys == 2 {
				cfg.APIKeys = append(cfg.APIKeys, "synthetic-b")
			}
			atomicCfg := config.NewAtomicConfig(cfg, "")
			calls, otherCalls := 0, 0
			reg := core.NewProviderRegistry()
			if err := reg.Register(&usageLimitStreamProvider{name: "opencode-go", calls: &calls,
				err: &client.APIError{StatusCode: tc.status, Body: "synthetic refusal"}}); err != nil {
				t.Fatal(err)
			}
			if err := reg.Register(&usageLimitStreamProvider{name: "opencode-zen", calls: &otherCalls,
				body: "data: {\"type\":\"message_stop\"}\n\n"}); err != nil {
				t.Fatal(err)
			}
			fallback := router.NewFallbackHandler(slog.Default(), 3, time.Minute)
			fallback.SetAtomicConfig(atomicCfg)
			h := &MessagesHandler{client: client.NewOpenCodeClient(atomicCfg), providerRegistry: reg,
				fallbackHandler: fallback, streamProxy: NewStreamProxy(), logger: slog.Default(), metrics: metrics.New()}
			chain := []config.ModelConfig{{Provider: "opencode-go", ModelID: "a"}, {Provider: "opencode-go", ModelID: "b"}, {Provider: "opencode-zen", ModelID: "c"}}
			for range tc.repeats {
				h.handleStreaming(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/messages", nil),
					&types.MessageRequest{Stream: boolPtr(true)}, chain, nil, router.ScenarioDefault, "synthetic")
			}
			if calls != tc.wantCalls || otherCalls != tc.repeats {
				t.Fatalf("calls=%d other=%d; want %d/%d", calls, otherCalls, tc.wantCalls, tc.repeats)
			}
			if tc.status == 500 && fallback.GetCircuitStates()["opencode-go/a"] != "open" {
				t.Fatal("stream failures did not open circuit")
			}
			if tc.status == 500 {
				_, _, err := fallback.ExecuteWithFallback(context.Background(), chain[:2], func(context.Context, config.ModelConfig) ([]byte, error) {
					t.Error("buffered request retried a model whose streaming circuit is open")
					return nil, nil
				})
				if err == nil {
					t.Fatal("expected shared circuits to reject buffered request")
				}
			}
		})
	}
}

func TestOpenRouterRetainsEmptyCompletedStreamSemantics(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("data: {\"id\":\"c\",\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"completion_tokens\":1}}\n\ndata: [DONE]\n\n"))
	}))
	defer upstream.Close()
	h := newStreamingTestHandler(t, upstream.URL)
	cfg := config.NewAtomicConfig(&config.Config{OpenRouter: config.OpenRouterConfig{BaseURL: upstream.URL, APIKey: "synthetic"}}, "")
	h.providerRegistry = core.NewProviderRegistry()
	if err := h.providerRegistry.Register(provider.NewOpenRouterProvider(cfg, nil)); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h.handleStreaming(w, httptest.NewRequest(http.MethodPost, "/v1/messages", nil), &types.MessageRequest{Stream: boolPtr(true)},
		[]config.ModelConfig{{Provider: "openrouter", ModelID: "synthetic"}}, nil, router.ScenarioLongContext, "synthetic")
	if strings.Contains(w.Body.String(), "event: error") || h.metrics.GetSnapshot().RequestsSuccess != 1 {
		t.Fatalf("completed OpenRouter reply became failure: %s", w.Body.String())
	}
}

package handlers

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/metrics"
	"github.com/routatic/proxy/internal/router"
	"github.com/routatic/proxy/internal/token"
)

func TestNonStreamingAttemptTimeout(t *testing.T) {
	for _, endpoint := range []string{"/v1/messages", "/v1/responses"} {
		for _, scenario := range []struct {
			name     string
			fallback bool
			succeed  bool
		}{{"single timeout", false, false}, {"chain exhausted", true, false}, {"fallback succeeds", true, true}} {
			t.Run(endpoint+"/"+scenario.name, func(t *testing.T) {
				var attempts atomic.Int32
				release := make(chan struct{})
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					var body struct {
						Model string `json:"model"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
						return
					}
					attempts.Add(1)
					if scenario.succeed && body.Model == "synthetic-fallback" {
						w.Header().Set("Content-Type", "application/json")
						_, _ = io.WriteString(w, `{"id":"chat_test","choices":[{"message":{"role":"assistant","content":"answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1}}`)
						return
					}
					select {
					case <-r.Context().Done():
					case <-release:
					}
				}))
				defer upstream.Close()
				defer close(release)
				cfg := &config.Config{
					APIKey:                "synthetic-test-key",
					OpenCodeGo:            config.OpenCodeGoConfig{BaseURL: upstream.URL, TimeoutMs: 150},
					Models:                map[string]config.ModelConfig{"default": {Provider: "opencode-go", ModelID: "synthetic-primary"}},
					RespectRequestedModel: boolPtr(false),
				}
				if scenario.fallback {
					cfg.Fallbacks = map[string][]config.ModelConfig{"default": {{Provider: "opencode-go", ModelID: "synthetic-fallback"}}}
				}
				atomicCfg := config.NewAtomicConfig(cfg, "")
				counter, err := token.NewCounter()
				if err != nil {
					t.Fatal(err)
				}
				m := metrics.New()
				h := NewMessagesHandler(client.NewOpenCodeClient(atomicCfg), newTestProviderRegistry(t, atomicCfg),
					router.NewModelRouter(atomicCfg), router.NewFallbackHandler(slog.Default(), 1, time.Minute), counter, m, nil, nil)
				parentErr := make(chan error, 1)
				gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if endpoint == "/v1/messages" {
						h.HandleMessages(w, r)
					} else {
						h.HandleResponses(w, r)
					}
					parentErr <- r.Context().Err()
				}))
				defer gateway.Close()
				payload := `{"model":"synthetic","stream":false,"max_tokens":8,"messages":[{"role":"user","content":"hello"}]}`
				if endpoint == "/v1/responses" {
					payload = `{"model":"synthetic","stream":false,"input":"hello"}`
				}
				response, err := (&http.Client{Timeout: 5 * time.Second}).Post(gateway.URL+endpoint, "application/json", strings.NewReader(payload))
				if err != nil {
					t.Fatal(err)
				}
				defer response.Body.Close()
				body, err := io.ReadAll(response.Body)
				if err != nil {
					t.Fatal(err)
				}
				if err := <-parentErr; err != nil {
					t.Fatalf("parent request ended: %v", err)
				}
				wantStatus, wantFailures := http.StatusBadGateway, int64(1)
				if scenario.succeed {
					wantStatus, wantFailures = http.StatusOK, 0
				}
				if response.StatusCode != wantStatus || len(body) == 0 || m.GetSnapshot().RequestsFailed != wantFailures {
					t.Fatalf("status=%d body=%s failures=%d", response.StatusCode, body, m.GetSnapshot().RequestsFailed)
				}
				wantAttempts := int32(1)
				if scenario.fallback {
					wantAttempts = 2
				}
				if attempts.Load() != wantAttempts {
					t.Fatalf("attempts=%d want=%d", attempts.Load(), wantAttempts)
				}
				if state := h.fallbackHandler.GetCircuitStates()["opencode-go/synthetic-primary"]; state != "open" {
					t.Fatalf("timed-out model circuit=%s", state)
				}
			})
		}
	}
}

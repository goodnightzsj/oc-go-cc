package handlers

import (
	"bufio"
	"context"
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
	"github.com/routatic/proxy/pkg/types"
)

func TestStreamingBodyTimeoutAttribution(t *testing.T) {
	for _, protocol := range []struct{ name, frame string }{
		{"openai", `data: {"choices":[{"delta":{"content":"partial"}}]}` + "\n\n"},
		{"responses", "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n"},
		{"gemini", `data: {"candidates":[{"content":{"parts":[{"text":"partial"}]}}]}` + "\n\n"},
		{"anthropic", "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"partial\"}}\n\n"},
	} {
		for _, parentCancel := range []bool{false, true} {
			name := protocol.name + "/deadline"
			if parentCancel {
				name = protocol.name + "/parent_cancel"
			}
			t.Run(name, func(t *testing.T) {
				var attempts atomic.Int32
				release := make(chan struct{})
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					attempts.Add(1)
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = io.WriteString(w, protocol.frame)
					w.(http.Flusher).Flush()
					select {
					case <-r.Context().Done():
					case <-release:
					}
				}))
				defer upstream.Close()
				defer close(release)
				timeoutMs := 150
				if parentCancel {
					timeoutMs = 5000
				}
				cfg := config.NewAtomicConfig(&config.Config{OpenCodeZen: config.OpenCodeZenConfig{
					BaseURL: upstream.URL, AnthropicBaseURL: upstream.URL,
					ResponsesBaseURL: upstream.URL, GeminiBaseURL: upstream.URL,
					StreamingTimeoutMs: timeoutMs,
				}}, "")
				h := &MessagesHandler{
					fallbackHandler: router.NewFallbackHandler(nil, 1, time.Minute),
					client:          client.NewOpenCodeClient(cfg), providerRegistry: newTestProviderRegistry(t, cfg),
					streamProxy: NewStreamProxy(), logger: slog.Default(), metrics: metrics.New(),
				}
				raw := json.RawMessage(`{"model":"synthetic","stream":true,"max_tokens":8,"messages":[{"role":"user","content":"hello"}]}`)
				var message types.MessageRequest
				if err := json.Unmarshal(raw, &message); err != nil {
					t.Fatal(err)
				}
				chain := []config.ModelConfig{
					{Provider: "opencode-zen", ModelID: "synthetic", WireFormat: protocol.name},
					{Provider: "opencode-zen", ModelID: "fallback", WireFormat: protocol.name},
				}
				done := make(chan error, 1)
				gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					h.handleStreaming(w, r, &message, chain, raw, "", "")
					done <- r.Context().Err()
				}))
				defer gateway.Close()
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				req, err := http.NewRequestWithContext(ctx, http.MethodPost, gateway.URL, nil)
				if err != nil {
					t.Fatal(err)
				}
				response, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
				if err != nil {
					t.Fatal(err)
				}
				defer response.Body.Close()
				reader := bufio.NewReader(response.Body)
				// Wait for actual body delivery before either canceling or waiting for timeout.
				for {
					line, err := reader.ReadString('\n')
					if err != nil {
						t.Fatalf("missing partial output: %v", err)
					}
					if strings.Contains(line, "partial") {
						break
					}
				}
				if parentCancel {
					cancel()
				} else {
					body, err := io.ReadAll(reader)
					if err != nil || !strings.Contains(string(body), "event: error") {
						t.Fatalf("timeout response=%s error=%v", body, err)
					}
				}
				select {
				case err := <-done:
					if (err != nil) != parentCancel {
						t.Fatalf("parent error=%v, canceled=%v", err, parentCancel)
					}
				case <-time.After(3 * time.Second):
					t.Fatal("handler did not finish")
				}
				if attempts.Load() != 1 {
					t.Fatalf("attempts=%d; must not fallback after output", attempts.Load())
				}
				wantFailures := int64(1)
				if parentCancel {
					wantFailures = 0
				}
				snapshot := h.metrics.GetSnapshot()
				if snapshot.RequestsFailed != wantFailures || snapshot.RequestsSuccess != 0 || snapshot.UpstreamCalls != 1 {
					t.Errorf("stream outcome counters=%+v want failures=%d and one upstream call", snapshot, wantFailures)
				}
				want := "open"
				if parentCancel {
					want = "closed"
				}
				if got := h.fallbackHandler.GetCircuitStates()["opencode-zen/synthetic"]; got != want {
					t.Fatalf("circuit=%s want=%s", got, want)
				}
			})
		}
	}
}

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
				if got := m.GetSnapshot().UpstreamCalls; got != int64(wantAttempts) {
					t.Errorf("upstream_calls=%d want actual sends=%d", got, wantAttempts)
				}
				if state := h.fallbackHandler.GetCircuitStates()["opencode-go/synthetic-primary"]; state != "open" {
					t.Fatalf("timed-out model circuit=%s", state)
				}
			})
		}
	}
}

package handlers

import (
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
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/internal/metrics"
	"github.com/routatic/proxy/internal/provider"
	"github.com/routatic/proxy/internal/router"
	"github.com/routatic/proxy/pkg/types"
)

func TestClinePeekFailureFallback(t *testing.T) {
	for _, failure := range []string{"healthy", "idle", "unexpected_eof", "total_deadline", "client_cancel"} {
		t.Run(failure, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var secondaryCalls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req struct {
					Model string `json:"model"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				if strings.Contains(req.Model, "secondary") {
					secondaryCalls.Add(1)
					_, _ = io.WriteString(w, "data: {\"id\":\"test\",\"model\":\"secondary\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"fallback answer\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":10}}\n\ndata: [DONE]\n\n")
					return
				}
				if failure == "unexpected_eof" {
					w.Header().Set("Content-Length", "1000")
				}
				_, _ = io.WriteString(w, ": opened\n\n")
				w.(http.Flusher).Flush()
				switch failure {
				case "unexpected_eof":
					return
				case "client_cancel":
					cancel()
				case "total_deadline":
					ticker := time.NewTicker(5 * time.Millisecond)
					defer ticker.Stop()
					for {
						select {
						case <-r.Context().Done():
							return
						case <-ticker.C:
							_, _ = io.WriteString(w, ": heartbeat\n\n")
							w.(http.Flusher).Flush()
						}
					}
				}
				<-r.Context().Done()
			}))
			defer upstream.Close()
			cfg := config.NewAtomicConfig(&config.Config{ClinePass: config.ClinePassConfig{
				APIKey: "synthetic-key", BaseURL: upstream.URL, StreamTimeoutMs: 100, StreamingTimeoutMs: 300,
			}}, "")
			reg := core.NewProviderRegistry()
			if err := reg.Register(provider.NewClinePassProvider(cfg, nil)); err != nil {
				t.Fatal(err)
			}
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			fallback := router.NewFallbackHandler(logger, 3, time.Minute)
			fallback.SetAtomicConfig(cfg)
			h := &MessagesHandler{client: client.NewOpenCodeClient(cfg), providerRegistry: reg,
				fallbackHandler: fallback, streamProxy: NewStreamProxy(), logger: logger, metrics: metrics.New()}
			chain := []config.ModelConfig{{Provider: "cline-pass", ModelID: "cline-pass/primary"}, {Provider: "cline-pass", ModelID: "cline-pass/secondary"}}
			if failure == "healthy" {
				chain = chain[1:]
			}
			w := httptest.NewRecorder()
			h.handleStreaming(w, httptest.NewRequest(http.MethodPost, "/v1/messages", nil).WithContext(ctx),
				&types.MessageRequest{Stream: boolPtr(true), MaxTokens: 64, Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"hi"`)}}},
				chain, nil, router.ScenarioDefault, "synthetic")
			if failure == "client_cancel" {
				if secondaryCalls.Load() != 0 || w.Body.Len() != 0 {
					t.Fatalf("client canceled but stream continued: calls=%d response=%s", secondaryCalls.Load(), w.Body.String())
				}
				return
			}
			if secondaryCalls.Load() != 1 || !strings.Contains(w.Body.String(), "fallback answer") || strings.Contains(w.Body.String(), "event: error") || strings.Count(w.Body.String(), "event: message_start") != 1 {
				t.Fatalf("secondary calls=%d; response=%s", secondaryCalls.Load(), w.Body.String())
			}
		})
	}
}

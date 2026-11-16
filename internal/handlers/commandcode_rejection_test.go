package handlers

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/metrics"
	"github.com/routatic/proxy/internal/provider"
	"github.com/routatic/proxy/internal/router"
	"github.com/routatic/proxy/pkg/types"
)

func TestProviderTruncatedAuthBodyPreservesStatus(t *testing.T) {
	for _, platform := range []string{"commandcode", "cline-pass"} {
		t.Run(platform, func(t *testing.T) {
			for _, truncated := range []bool{false, true} {
				t.Run(map[bool]string{false: "complete", true: "truncated"}[truncated], func(t *testing.T) {
					calls := 0
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls++
						_, _ = io.Copy(io.Discard, r.Body)
						if truncated {
							w.Header().Set("Content-Length", "1000")
						}
						w.WriteHeader(http.StatusUnauthorized)
						_, _ = io.WriteString(w, "synthetic credential rejected")
					}))
					defer upstream.Close()
					live := &config.Config{}
					if platform == "cline-pass" {
						live.ClinePass = config.ClinePassConfig{APIKeys: []string{"synthetic-a", "synthetic-b"}, BaseURL: upstream.URL}
					} else {
						live.CommandCode = config.CommandCodeConfig{APIKeys: []string{"synthetic-a", "synthetic-b"}, BaseURL: upstream.URL}
					}
					cfg := config.NewAtomicConfig(live, "")
					h := &MessagesHandler{client: client.NewOpenCodeClient(cfg), providerRegistry: newTestProviderRegistry(t, cfg), fallbackHandler: router.NewFallbackHandler(nil, 3, time.Minute), metrics: metrics.New(), logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
					h.fallbackHandler.SetKeyAvailability(provider.NewKeyAvailability(cfg))
					out := httptest.NewRecorder()
					h.handleNonStreaming(out, httptest.NewRequest(http.MethodPost, "/v1/messages", nil), &types.MessageRequest{Model: "m", MaxTokens: 16}, []config.ModelConfig{{Provider: platform, ModelID: "m1"}, {Provider: platform, ModelID: "m2"}, {Provider: platform, ModelID: "m3"}}, nil, router.ScenarioDefault, "synthetic")
					if calls != 2 || out.Code != http.StatusUnauthorized {
						t.Fatalf("known source 401 replaced: requests=%d status=%d body=%s", calls, out.Code, out.Body.String())
					}
				})
			}
		})
	}
}

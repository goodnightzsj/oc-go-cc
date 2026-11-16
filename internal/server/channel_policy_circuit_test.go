package server

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
)

func TestRequiredChannelBeforeAttemptFilters(t *testing.T) {
	for _, filter := range []string{"circuit", "capacity"} {
		for _, path := range []string{"/v1/messages", "/v1/responses"} {
			for _, stream := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/stream=%t", filter, path, stream), func(t *testing.T) {
					var calls atomic.Int32
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls.Add(1)
						if stream {
							_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"unpinned answer\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
							return
						}
						_, _ = io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"unpinned answer"},"finish_reason":"stop"}]}`)
					}))
					t.Cleanup(upstream.Close)
					respect := false
					model := config.ModelConfig{Provider: "cline-pass", ModelID: "synthetic", MaxTokens: 100}
					if filter == "capacity" {
						model.ContextWindow = 1
					}
					cfg := &config.Config{
						ActiveSite: "cline-pass", EnableCrossPlatformFallback: true, EnableStreamingScenarioRouting: true,
						ClinePass:             config.ClinePassConfig{APIKeys: []string{"synthetic"}, BaseURL: upstream.URL, ChannelPinEnabled: true, ChannelPin: "private"},
						OpenRouter:            config.OpenRouterConfig{APIKeys: []string{"synthetic"}, BaseURL: upstream.URL},
						Models:                map[string]config.ModelConfig{"default": model},
						Fallbacks:             map[string][]config.ModelConfig{"default": {{Provider: "openrouter", ModelID: "fallback", MaxTokens: 100}}},
						RespectRequestedModel: &respect,
						Storage:               &config.StorageConfig{DatabasePath: filepath.Join(t.TempDir(), "history.db"), RetentionDays: -1},
					}
					srv, err := NewServer(config.NewAtomicConfig(cfg, ""), nil)
					if err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { _ = srv.Close() })
					if filter == "circuit" {
						for range 3 {
							settle := srv.Fallback().AllowAttempt(model)
							if settle == nil {
								t.Fatal("fixture circuit opened too early")
							}
							settle(t.Context(), &client.APIError{StatusCode: 500, Body: "synthetic automatic-mode failure"})
						}
					}
					payload := fmt.Sprintf(`{"model":"client-alias","messages":[{"role":"user","content":"hi"}],"max_tokens":100,"stream":%t}`, stream)
					if path == "/v1/responses" {
						payload = fmt.Sprintf(`{"model":"client-alias","input":"hi","stream":%t}`, stream)
					}
					w := httptest.NewRecorder()
					srv.httpSrv.Handler.ServeHTTP(w, httptest.NewRequest("POST", path, strings.NewReader(payload)))
					if w.Code != http.StatusUnprocessableEntity || calls.Load() != 0 || !strings.Contains(w.Body.String(), "channel_pin_unsupported") {
						t.Fatalf("required channel bypassed by %s filter: calls=%d status=%d body=%s", filter, calls.Load(), w.Code, w.Body.String())
					}
				})
			}
		}
	}
}

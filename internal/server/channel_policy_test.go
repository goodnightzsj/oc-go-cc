package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/routatic/proxy/internal/config"
)

func TestRequiredChannelNeverFallsBackAcrossProtocols(t *testing.T) {
	for _, path := range []string{"/v1/messages", "/v1/responses"} {
		for _, stream := range []bool{false, true} {
			for _, pin := range []bool{true, false} {
				t.Run(fmt.Sprintf("%s/stream=%t/pin=%t", path, stream, pin), func(t *testing.T) {
					var calls atomic.Int32
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls.Add(1)
						if strings.HasPrefix(r.URL.Path, "/cline") {
							w.WriteHeader(http.StatusInternalServerError)
							return
						}
						var req struct {
							Stream bool `json:"stream"`
						}
						if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
							t.Error(err)
							w.WriteHeader(http.StatusBadRequest)
							return
						}
						if !req.Stream {
							_, _ = io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"unpinned answer"},"finish_reason":"stop"}]}`)
							return
						}
						_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"unpinned answer\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
					}))
					t.Cleanup(upstream.Close)
					respect := false
					srv, err := NewServer(config.NewAtomicConfig(&config.Config{
						ActiveSite: "cline-pass", EnableCrossPlatformFallback: true, EnableStreamingScenarioRouting: true,
						ClinePass:             config.ClinePassConfig{APIKeys: []string{"synthetic"}, BaseURL: upstream.URL + "/cline", ChannelPinEnabled: pin, ChannelPin: "private"},
						OpenRouter:            config.OpenRouterConfig{APIKeys: []string{"synthetic"}, BaseURL: upstream.URL},
						Models:                map[string]config.ModelConfig{"default": {Provider: "cline-pass", ModelID: "synthetic", MaxTokens: 100}},
						Fallbacks:             map[string][]config.ModelConfig{"default": {{Provider: "openrouter", ModelID: "fallback", MaxTokens: 100}}},
						RespectRequestedModel: &respect,
						Storage:               &config.StorageConfig{DatabasePath: filepath.Join(t.TempDir(), "history.db"), RetentionDays: -1},
					}, ""), nil)
					if err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { _ = srv.Close() })
					payload := fmt.Sprintf(`{"model":"client-alias","messages":[{"role":"user","content":"hi"}],"max_tokens":100,"stream":%t}`, stream)
					if path == "/v1/responses" {
						payload = fmt.Sprintf(`{"model":"client-alias","input":"hi","stream":%t}`, stream)
					}
					w := httptest.NewRecorder()
					srv.httpSrv.Handler.ServeHTTP(w, httptest.NewRequest("POST", path, strings.NewReader(payload)))
					body := w.Body.String()
					if !pin {
						if w.Code != http.StatusOK || calls.Load() != 2 || !strings.Contains(body, "unpinned answer") {
							t.Fatalf("fallback control is unreachable: calls=%d status=%d body=%s", calls.Load(), w.Code, body)
						}
						return
					}
					if !strings.Contains(body, "channel_pin_unsupported") || calls.Load() != 0 {
						t.Fatalf("required channel silently downgraded: calls=%d status=%d body=%s", calls.Load(), w.Code, body)
					}
					if stream && w.Code == http.StatusOK {
						if !strings.Contains(body, "event: error") && !strings.Contains(body, "event: response.failed") {
							t.Fatalf("missing failed stream: %s", body)
						}
						if strings.Contains(body, "event: response.completed") || strings.Contains(body, "event: message_stop") {
							t.Fatal("policy refusal ended as success")
						}
					} else if w.Code != http.StatusUnprocessableEntity {
						t.Fatalf("status=%d body=%s", w.Code, body)
					}
					for _, state := range srv.Fallback().GetCircuitStates() {
						if state != "closed" {
							t.Fatalf("policy refusal damaged model health: %s", state)
						}
					}
				})
			}
		}
	}
}

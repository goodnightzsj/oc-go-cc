package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/routatic/proxy/internal/config"
)

func TestChatToolReferenceRejectionReportsClientCause(t *testing.T) {
	for _, platform := range []string{"cline-pass", "workbuddy", "openrouter"} {
		t.Run(platform, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					_, _ = w.Write([]byte(`{"data":[{"id":"synthetic"}]}`))
					return
				}
				calls.Add(1)
				http.Error(w, "unexpected upstream request", http.StatusInternalServerError)
			}))
			defer upstream.Close()
			respect := false
			model := config.ModelConfig{Provider: platform, ModelID: "synthetic", Vision: config.BoolPtr(true)}
			srv, err := NewServer(config.NewAtomicConfig(&config.Config{
				ActiveSite:            platform,
				RespectRequestedModel: &respect,
				ClinePass:             config.ClinePassConfig{BaseURL: upstream.URL, APIKey: "synthetic-cline"},
				WorkBuddy:             config.WorkBuddyConfig{BaseURL: upstream.URL, APIKey: "synthetic-buddy"},
				OpenRouter:            config.OpenRouterConfig{BaseURL: upstream.URL, APIKey: "synthetic-router"},
				Models: map[string]config.ModelConfig{
					"default": model,
					"vision":  model,
				},
				Fallbacks: map[string][]config.ModelConfig{"vision": {model}},
				Storage:   &config.StorageConfig{DatabasePath: filepath.Join(t.TempDir(), "test.db"), RetentionDays: -1},
			}, ""), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = srv.Close() }()
			for _, path := range []string{"/v1/messages", "/v1/responses"} {
				for _, stream := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/stream=%t", path, stream), func(t *testing.T) {
						payload := fmt.Sprintf(`{"model":"synthetic","max_tokens":100,"stream":%t,"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"call-1","name":"lookup","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"call-1","content":[{"type":"tool_reference","tool_name":"lookup"}]}]}]}`, stream)
						if path == "/v1/responses" {
							payload = fmt.Sprintf(`{"model":"synthetic","stream":%t,"input":[{"type":"function_call","call_id":"call-1","name":"lookup","arguments":"{}"},{"type":"function_call_output","call_id":"call-1","output":[{"type":"tool_reference"}]}]}`, stream)
						}
						w := httptest.NewRecorder()
						srv.httpSrv.Handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, strings.NewReader(payload)))
						wantStatus := http.StatusBadRequest
						if stream && path == "/v1/messages" {
							// This handler commits SSE headers before calling the provider.
							wantStatus = http.StatusOK
							failureEvent := "event: error"
							if !strings.Contains(w.Body.String(), failureEvent) {
								t.Errorf("missing terminal failure: %s", w.Body.String())
							}
						}
						if w.Code != wantStatus || !strings.Contains(w.Body.String(), "tool_reference") {
							t.Errorf("status=%d body=%s", w.Code, w.Body.String())
						}
					})
				}
			}
			if calls.Load() != 0 {
				t.Fatalf("local conversion rejection made %d upstream calls", calls.Load())
			}
		})
	}
}

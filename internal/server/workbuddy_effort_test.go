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
	"github.com/routatic/proxy/pkg/types"
)

func TestWorkBuddyPublishedEffortReachesUpstream(t *testing.T) {
	for _, tc := range []struct {
		name, target, intent, want string
		levels                     []string
	}{
		{"max", "global:deepseek-v4.1-flash", "max", "max", nil},
		{"phantom xhigh", "global:deepseek-v4.1-flash", "xhigh", "high", nil},
		{"single rung", "global:kimi-k3", "max", "medium", nil},
		{"operator ladder", "global:deepseek-v4.1-flash", "max", "low", []string{"low"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v1/models" {
					_, _ = io.WriteString(w, `{"data":[{"id":"global:deepseek-v4.1-flash","reasoning_supported_efforts":["low","high","max","off"],"reasoning_default_effort":"max"},{"id":"global:kimi-k3","reasoning_supported_efforts":["medium"]}]}`)
					return
				}
				calls.Add(1)
				var request types.ChatCompletionRequest
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					return
				}
				if request.Model != tc.target || request.ReasoningEffort == nil || *request.ReasoningEffort != tc.want {
					t.Errorf("model=%s effort=%v want=%s", request.Model, request.ReasoningEffort, tc.want)
				}
				if request.Stream != nil && *request.Stream {
					_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":1}}\n\ndata: [DONE]\n\n")
				} else {
					_, _ = io.WriteString(w, `{"id":"synthetic","choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
				}
			}))
			t.Cleanup(up.Close)
			target := config.ModelConfig{Provider: "workbuddy", ModelID: tc.target}
			if tc.levels != nil {
				target.Reasoning = &config.ReasoningConfig{Levels: tc.levels}
			}
			cfg := &config.Config{ActiveSite: "workbuddy", RespectRequestedModel: config.BoolPtr(false), WorkBuddy: config.WorkBuddyConfig{BaseURL: up.URL, APIKey: "synthetic"}, Models: map[string]config.ModelConfig{"default": target}, ModelOverrides: map[string]config.ModelConfig{"alias": target}, ModelFamilyOverrides: map[string]config.ModelConfig{"opus": target}, Storage: &config.StorageConfig{DatabasePath: filepath.Join(t.TempDir(), "test.db"), RetentionDays: -1}}
			srv, err := NewServer(config.NewAtomicConfig(cfg, ""), nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = srv.Close() })
			for _, model := range []string{"scenario", "alias", "claude-opus-test"} {
				for _, path := range []string{"/v1/messages", "/v1/responses"} {
					for _, stream := range []bool{false, true} {
						payload := fmt.Sprintf(`{"model":%q,"max_tokens":100,"stream":%t,"messages":[{"role":"user","content":"hi"}],"output_config":{"effort":%q}}`, model, stream, tc.intent)
						if path == "/v1/responses" {
							payload = fmt.Sprintf(`{"model":%q,"stream":%t,"input":"hi","reasoning":{"effort":%q}}`, model, stream, tc.intent)
						}
						w := httptest.NewRecorder()
						srv.httpSrv.Handler.ServeHTTP(w, httptest.NewRequest("POST", path, strings.NewReader(payload)))
						if w.Code != 200 || !strings.Contains(w.Body.String(), "ok") || strings.Contains(w.Body.String(), "response.failed") {
							t.Fatalf("%s %s stream=%t: %d %s", model, path, stream, w.Code, w.Body.String())
						}
					}
				}
			}
			if calls.Load() != 12 {
				t.Fatalf("inference calls=%d want12", calls.Load())
			}
		})
	}
}

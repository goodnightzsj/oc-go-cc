package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/pkg/types"
)

func TestChatToolImagesRouteAndReachProviders(t *testing.T) {
	for _, platform := range []string{"cline-pass", "workbuddy", "openrouter"} {
		for _, path := range []string{"/v1/messages", "/v1/responses"} {
			for _, stream := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s%s/stream=%t", platform, path, stream), func(t *testing.T) {
					var calls atomic.Int32
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.Method == http.MethodGet {
							_, _ = io.WriteString(w, `{"data":[{"id":"vision"}]}`)
							return
						}
						calls.Add(1)
						var req types.ChatCompletionRequest
						if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
							t.Error(err)
							http.Error(w, "invalid request", http.StatusBadRequest)
							return
						}
						if req.Model != "vision" || len(req.Messages) < 4 || len(req.Messages[0].ToolCalls) != 2 || req.Messages[1].ToolCallID != "b" || req.Messages[2].ToolCallID != "a" {
							t.Errorf("wrong model or tool batch: %+v", req)
							http.Error(w, "wrong tool batch", http.StatusBadRequest)
							return
						}
						for _, msg := range req.Messages[1:3] {
							var ref string
							if msg.Role != "tool" || json.Unmarshal(msg.Content, &ref) != nil || !strings.Contains(ref, msg.ToolCallID) {
								t.Errorf("tool result is not linked text: %+v", msg)
							}
						}
						var content []string
						for _, msg := range req.Messages[3:] {
							var text string
							if msg.Role == "user" && json.Unmarshal(msg.Content, &text) == nil {
								content = append(content, text)
								continue
							}
							var parts []types.ChatContentPart
							if msg.Role != "user" || json.Unmarshal(msg.Content, &parts) != nil {
								t.Errorf("invalid multimodal projection: %+v", msg)
								continue
							}
							for _, part := range parts {
								if part.Type == "image_url" && part.ImageURL != nil {
									content = append(content, part.ImageURL.URL)
								} else if !strings.Contains(part.Text, "tool_call_id") {
									content = append(content, part.Text)
								}
							}
						}
						want := []string{"data:image/png;base64,aGk=", "before", "https://example.invalid/shot.png", "after", "continue"}
						if !reflect.DeepEqual(content, want) {
							t.Errorf("tool output lost/reordered/duplicated: got %q want %q", content, want)
						}
						if req.Stream != nil && *req.Stream {
							w.Header().Set("Content-Type", "text/event-stream")
							_, _ = io.WriteString(w, "data: {\"id\":\"chatcmpl-test\",\"model\":\"vision\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"seen\"},\"finish_reason\":null}]}\n\ndata: {\"id\":\"chatcmpl-test\",\"model\":\"vision\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":1,\"total_tokens\":11}}\n\ndata: [DONE]\n\n")
						} else {
							_, _ = io.WriteString(w, `{"id":"chatcmpl-test","model":"vision","choices":[{"index":0,"message":{"role":"assistant","content":"seen"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":1,"total_tokens":11}}`)
						}
					}))
					t.Cleanup(upstream.Close)
					respect := false
					vision := config.ModelConfig{Provider: platform, ModelID: "vision", Vision: config.BoolPtr(true)}
					srv, err := NewServer(config.NewAtomicConfig(&config.Config{
						ActiveSite: platform, RespectRequestedModel: &respect,
						ClinePass:  config.ClinePassConfig{BaseURL: upstream.URL, APIKey: "synthetic"},
						WorkBuddy:  config.WorkBuddyConfig{BaseURL: upstream.URL, APIKey: "synthetic"},
						OpenRouter: config.OpenRouterConfig{BaseURL: upstream.URL, APIKey: "synthetic"},
						Models: map[string]config.ModelConfig{
							"default": {Provider: platform, ModelID: "text-only", Vision: config.BoolPtr(false)}, "vision": vision,
						},
						Fallbacks: map[string][]config.ModelConfig{"vision": {vision}},
						Storage:   &config.StorageConfig{DatabasePath: filepath.Join(t.TempDir(), "history.db"), RetentionDays: -1},
					}, ""), nil)
					if err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { _ = srv.Close() })
					payload := fmt.Sprintf(`{"model":"m","max_tokens":100,"stream":%t,"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"a","name":"shot","input":{}},{"type":"tool_use","id":"b","name":"shot","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"b","content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"aGk="}}]}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"a","content":[{"type":"text","text":"before"},{"type":"image","source":{"type":"url","url":"https://example.invalid/shot.png"}},{"type":"text","text":"after"}]}]}]}`, stream)
					if path == "/v1/responses" {
						payload = fmt.Sprintf(`{"model":"m","stream":%t,"input":[{"type":"function_call","call_id":"a","name":"shot","arguments":"{}"},{"type":"function_call","call_id":"b","name":"shot","arguments":"{}"},{"type":"function_call_output","call_id":"b","output":[{"type":"input_image","image_url":"data:image/png;base64,aGk="}]},{"type":"function_call_output","call_id":"a","output":[{"type":"input_text","text":"before"},{"type":"input_image","image_url":"https://example.invalid/shot.png"},{"type":"input_text","text":"after"}]}]}`, stream)
					}
					// A text-only tail is still part of this user/tool-result turn.
					var request map[string]json.RawMessage
					if err := json.Unmarshal([]byte(payload), &request); err != nil {
						t.Fatal(err)
					}
					field := "messages"
					if path == "/v1/responses" {
						field = "input"
					}
					var messages []json.RawMessage
					if err := json.Unmarshal(request[field], &messages); err != nil {
						t.Fatal(err)
					}
					messages = append(messages, json.RawMessage(`{"role":"user","content":"continue"}`))
					request[field], err = json.Marshal(messages)
					if err != nil {
						t.Fatal(err)
					}
					encoded, err := json.Marshal(request)
					if err != nil {
						t.Fatal(err)
					}
					w := httptest.NewRecorder()
					srv.httpSrv.Handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(encoded))))
					if w.Code != http.StatusOK || calls.Load() != 1 || !strings.Contains(w.Body.String(), "seen") || strings.Contains(w.Body.String(), `"type":"error"`) || strings.Contains(w.Body.String(), "event: response.failed") {
						t.Fatalf("status=%d calls=%d body=%s", w.Code, calls.Load(), w.Body.String())
					}
				})
			}
		}
	}
}

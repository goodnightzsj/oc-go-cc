package server

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/routatic/proxy/internal/config"
)

func TestToolArgumentBudgetTruncationAcrossBothInbounds(t *testing.T) {
	for _, wire := range []string{"openai", "responses"} {
		for _, path := range []string{"/v1/messages", "/v1/responses"} {
			for _, budget := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s%s/budget=%t", wire, path, budget), func(t *testing.T) {
					calls := 0
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls++
						_, _ = io.Copy(io.Discard, r.Body)
						w.Header().Set("Content-Type", "text/event-stream")
						if wire == "openai" {
							finish := "tool_calls"
							if budget {
								finish = "length"
							}
							_, _ = fmt.Fprintf(w, "data:%s\n\ndata:%s\n\n", `{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"a","type":"function","function":{"name":"lookup","arguments":"{"}}]}}]}`, fmt.Sprintf(`{"choices":[{"delta":{},"finish_reason":%q}]}`, finish))
						} else {
							terminal := `{"type":"response.completed","response":{"status":"completed"}}`
							if budget {
								terminal = `{"type":"response.incomplete","response":{"status":"incomplete","incomplete_details":{"reason":"max_output_tokens"}}}`
							}
							for _, event := range []string{
								`{"type":"response.output_item.added","item":{"type":"function_call","id":"f","call_id":"a","name":"lookup"}}`,
								`{"type":"response.function_call_arguments.delta","item_id":"f","delta":"{"}`,
								`{"type":"response.output_item.done","item":{"type":"function_call","status":"incomplete","id":"f","call_id":"a","name":"lookup","arguments":"{"}}`, terminal,
							} {
								_, _ = fmt.Fprintf(w, "data:%s\n\n", event)
							}
						}
					}))
					t.Cleanup(upstream.Close)
					respect := false
					srv, err := NewServer(config.NewAtomicConfig(&config.Config{
						OpenCodeGo: config.OpenCodeGoConfig{BaseURL: upstream.URL, ResponsesBaseURL: upstream.URL, APIKey: "synthetic"},
						Models:     map[string]config.ModelConfig{"default": {Provider: "opencode-go", ModelID: "synthetic", WireFormat: wire}}, RespectRequestedModel: &respect,
						Storage: &config.StorageConfig{DatabasePath: filepath.Join(t.TempDir(), "history.db"), RetentionDays: -1},
					}, ""), nil)
					if err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { _ = srv.Close() })
					payload := `{"model":"m","max_tokens":100,"stream":true,"messages":[{"role":"user","content":"hello"}]}`
					if path == "/v1/responses" {
						payload = `{"model":"m","stream":true,"input":"hello"}`
					}
					w := httptest.NewRecorder()
					srv.httpSrv.Handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, strings.NewReader(payload)))
					body := w.Body.String()
					if calls != 1 {
						t.Fatalf("replayed partial output: calls=%d", calls)
					}
					if !budget {
						native := wire == "responses" && path == "/v1/responses"
						if !strings.Contains(body, `"error"`) || strings.Contains(body, `"type":"message_stop"`) || !native && strings.Contains(body, `"type":"response.completed"`) {
							t.Fatalf("invalid normal tool became successful: %s", body)
						}
						return
					}
					if strings.Contains(body, `"type":"error"`) || strings.Contains(body, `"type":"response.failed"`) {
						t.Fatalf("budget truncation became adapter failure: %s", body)
					}
					want := `"stop_reason":"max_tokens"`
					if path == "/v1/responses" {
						want = `"reason":"max_output_tokens"`
					}
					if !strings.Contains(body, want) {
						t.Fatalf("budget terminal lost: %s", body)
					}
				})
			}
		}
	}
}

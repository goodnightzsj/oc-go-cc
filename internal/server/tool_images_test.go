package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/pkg/types"
)

func TestToolImagesRouteAndReachResponsesWire(t *testing.T) {
	for _, path := range []string{"/v1/messages", "/v1/responses"} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", path, stream), func(t *testing.T) {
				calls := 0
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					var req types.ResponsesRequest
					if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
						t.Error(err)
						http.Error(w, "invalid request", http.StatusBadRequest)
						return
					}
					if req.Model != "vision" || len(req.Input) != 2 || req.Input[1].CallID != "c" {
						t.Errorf("wrong route/tool association: %+v", req)
					} else {
						var parts []map[string]string
						if err := json.Unmarshal(req.Input[1].Output, &parts); err != nil || len(parts) != 3 || parts[0]["text"] != "before" || parts[1]["image_url"] != "https://example.invalid/shot.png" || parts[2]["text"] != "after" {
							t.Errorf("image or order lost: %s err=%v", req.Input[1].Output, err)
						}
					}
					response := `{"status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"seen"}]}]}`
					if stream {
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = fmt.Fprintf(w, "data:{\"type\":\"response.output_text.delta\",\"delta\":\"seen\"}\n\ndata:{\"type\":\"response.completed\",\"response\":%s}\n\n", response)
					} else {
						_, _ = io.WriteString(w, response)
					}
				}))
				t.Cleanup(upstream.Close)
				respect := false
				vision := config.ModelConfig{Provider: "opencode-zen", ModelID: "vision", WireFormat: "responses", Vision: config.BoolPtr(true)}
				srv, err := NewServer(config.NewAtomicConfig(&config.Config{
					ActiveSite: "opencode-zen", OpenCodeZen: config.OpenCodeZenConfig{APIKey: "synthetic", ResponsesBaseURL: upstream.URL},
					Models: map[string]config.ModelConfig{
						"default": {Provider: "opencode-zen", ModelID: "text-only", WireFormat: "responses", Vision: config.BoolPtr(false)}, "vision": vision,
					},
					Fallbacks: map[string][]config.ModelConfig{"vision": {vision}}, RespectRequestedModel: &respect,
					Storage: &config.StorageConfig{DatabasePath: filepath.Join(t.TempDir(), "history.db"), RetentionDays: -1},
				}, ""), nil)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = srv.Close() })
				payload := fmt.Sprintf(`{"model":"m","stream":%t,"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"c","name":"shot","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"c","content":[{"type":"text","text":"before"},{"type":"image","source":{"type":"url","url":"https://example.invalid/shot.png"}},{"type":"text","text":"after"}]}]}]}`, stream)
				if path == "/v1/responses" {
					payload = fmt.Sprintf(`{"model":"m","stream":%t,"input":[{"type":"function_call","call_id":"c","name":"shot","arguments":"{}"},{"type":"function_call_output","call_id":"c","output":[{"type":"input_text","text":"before"},{"type":"input_image","image_url":"https://example.invalid/shot.png"},{"type":"input_text","text":"after"}]}]}`, stream)
				}
				w := httptest.NewRecorder()
				srv.httpSrv.Handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, strings.NewReader(payload)))
				if w.Code != 200 || calls != 1 || !strings.Contains(w.Body.String(), "seen") || strings.Contains(w.Body.String(), `"type":"error"`) {
					t.Fatalf("HTTP=%d calls=%d body=%s", w.Code, calls, w.Body.String())
				}
			})
		}
	}
}

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
	"github.com/routatic/proxy/internal/debug"
	"github.com/routatic/proxy/internal/storage"
)

func TestResponseModelSourceReachesHistory(t *testing.T) {
	for _, tc := range []struct{ platform, wire string }{
		{"opencode-go", "openai"}, {"opencode-zen", "openai"}, {"aws-bedrock", "openai"},
		{"commandcode", "openai"}, {"openrouter", "openai"}, {"workbuddy", "openai"}, {"cline-pass", "openai"},
		{"opencode-zen", "gemini"}, {"opencode-zen", "responses"},
		{"opencode-go", "anthropic"}, {"opencode-zen", "anthropic"}, {"aws-bedrock", "anthropic"}, {"commandcode", "anthropic"},
	} {
		t.Run(tc.platform+"/"+tc.wire, func(t *testing.T) {
			for _, source := range []struct{ model, channel string }{
				{"", "deepseek"}, {"synthetic", "deepseek"}, {"vmc/served", "deepseek"}, {"vmc/served", "private"},
			} {
				if source.channel == "private" && tc.platform != "cline-pass" {
					continue // Only ClinePass reports routing channels.
				}
				declared := source.model
				// Including equality proves an explicit declaration is not treated as absent.
				t.Run("declared="+declared+"/channel="+source.channel, func(t *testing.T) {
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.Method == http.MethodGet && r.URL.Path == "/v1/models" {
							_, _ = io.WriteString(w, `{"data":[{"id":"synthetic"}]}`)
							return
						}
						var request struct{ Stream bool }
						if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
							t.Error(err)
							return
						}
						stream := request.Stream || strings.Contains(r.URL.Path, "streamGenerateContent")
						if stream {
							w.Header().Set("Content-Type", "text/event-stream")
						}
						channel := ""
						if tc.platform == "cline-pass" {
							channel = source.channel
						}
						_, _ = io.WriteString(w, responseModelFixture(tc.wire, declared, stream, channel))
					}))
					t.Cleanup(upstream.Close)
					respect := false
					cfg := &config.Config{
						ActiveSite: tc.platform,
						OpenCodeGo: config.OpenCodeGoConfig{APIKey: "synthetic", BaseURL: upstream.URL, AnthropicBaseURL: upstream.URL},
						OpenCodeZen: config.OpenCodeZenConfig{APIKey: "synthetic", BaseURL: upstream.URL, AnthropicBaseURL: upstream.URL,
							ResponsesBaseURL: upstream.URL, GeminiBaseURL: upstream.URL},
						AWSBedrock:            config.AWSBedrockConfig{APIKey: "synthetic", BaseURL: upstream.URL + "/openai/v1", AnthropicBaseURL: upstream.URL + "/anthropic/v1"},
						CommandCode:           config.CommandCodeConfig{APIKey: "synthetic", BaseURL: upstream.URL, AnthropicBaseURL: upstream.URL},
						OpenRouter:            config.OpenRouterConfig{APIKey: "synthetic", BaseURL: upstream.URL},
						WorkBuddy:             config.WorkBuddyConfig{APIKey: "synthetic", BaseURL: upstream.URL},
						ClinePass:             config.ClinePassConfig{APIKey: "synthetic", BaseURL: upstream.URL},
						Models:                map[string]config.ModelConfig{"default": {Provider: tc.platform, ModelID: "synthetic", WireFormat: tc.wire, MaxTokens: 100}},
						RespectRequestedModel: &respect,
						Storage:               &config.StorageConfig{DatabasePath: filepath.Join(t.TempDir(), "history.db"), RetentionDays: -1},
					}
					var capture *debug.CaptureLogger
					if tc.platform == "cline-pass" {
						store, err := debug.NewStorage(config.DebugCapture{Enabled: true, Directory: filepath.Join(t.TempDir(), "capture"), MaxFiles: 2, MaxFileSize: 1 << 20})
						if err != nil {
							t.Fatal(err)
						}
						t.Cleanup(func() { _ = store.Close() })
						capture = debug.NewCaptureLogger(store, true)
						t.Cleanup(func() { _ = capture.Close() })
					}
					srv, err := NewServer(config.NewAtomicConfig(cfg, ""), capture)
					if err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { _ = srv.Close() })
					for _, path := range []string{"/v1/messages", "/v1/responses"} {
						for _, stream := range []bool{false, true} {
							payload := fmt.Sprintf(`{"model":"client-alias","messages":[{"role":"user","content":"hi"}],"max_tokens":100,"stream":%t}`, stream)
							if path == "/v1/responses" {
								payload = fmt.Sprintf(`{"model":"client-alias","input":"hi","stream":%t}`, stream)
							}
							w := httptest.NewRecorder()
							srv.httpSrv.Handler.ServeHTTP(w, httptest.NewRequest("POST", path, strings.NewReader(payload)))
							if w.Code != 200 || strings.Contains(w.Body.String(), `"type":"error"`) {
								t.Fatalf("%s stream=%t HTTP=%d body=%s", path, stream, w.Code, w.Body.String())
							}
							if tc.platform == "cline-pass" {
								want := declared
								if !stream {
									want = "deepseek/canonical"
								} else if want == "" {
									want = "synthetic"
								}
								if !strings.Contains(w.Body.String(), `"model":"`+want+`"`) {
									t.Fatalf("downstream model changed: %s", w.Body.String())
								}
							}
						}
					}
					rows, total, err := storage.NewRequests(srv.Storage()).Query(t.Context(), storage.RequestQuery{})
					if err != nil || total != 4 {
						t.Fatalf("total=%d err=%v", total, err)
					}
					for _, row := range rows {
						if !row.Success || row.ResponseModel != declared || row.Model != "synthetic" || row.Provider != tc.platform || row.RequestedModel != "client-alias" {
							t.Fatalf("source declaration lost/fabricated: %+v", row)
						}
						if tc.platform == "cline-pass" && (row.UpstreamModel != "deepseek/canonical" || row.UpstreamChannel != source.channel) {
							t.Fatalf("canonical/channel replaced by response model: %+v", row)
						}
					}
				})
			}
		})
	}
}

func responseModelFixture(wire, model string, stream bool, channel string) string {
	declaration := ""
	if model != "" {
		declaration = fmt.Sprintf(`"model":%q,`, model)
	}
	usage := `"usage":{"input_tokens":11,"output_tokens":7}`
	switch wire {
	case "anthropic":
		message := `{` + declaration + `"type":"message","id":"synthetic","role":"assistant","content":[{"type":"text","text":"answer"}],"stop_reason":"end_turn",` + usage + `}`
		if !stream {
			return message
		}
		return "data: {\"type\":\"message_start\",\"message\":" + message + "}\n\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"answer\"}}\n\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"}}\n\ndata: {\"type\":\"message_stop\"}\n\n"
	case "responses":
		response := `{` + declaration + `"id":"resp_fixture","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"answer"}]}],` + usage + `}`
		if !stream {
			return response
		}
		return "data: {\"type\":\"response.completed\",\"response\":" + response + "}\n\n"
	case "gemini":
		response := fmt.Sprintf(`{"modelVersion":%q,"candidates":[{"content":{"role":"model","parts":[{"text":"answer"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":11,"candidatesTokenCount":7}}`, model)
		if !stream {
			return response
		}
		return "data: " + response + "\n\n"
	default:
		usage = `"usage":{"prompt_tokens":11,"completion_tokens":7}`
		if !stream {
			return `{` + declaration + `"id":"synthetic","choices":[{"message":{"role":"assistant","content":"answer"},"finish_reason":"stop"}],` + usage + `}`
		}
		metadata := ""
		if channel != "" {
			metadata = fmt.Sprintf("data: {\"choices\":[],\"provider_metadata\":{\"gateway\":{\"routing\":{\"canonicalSlug\":\"deepseek/canonical\",\"finalProvider\":%q,\"resolvedProvider\":%q}}}}\n\n", channel, channel)
		}
		return "data: {" + declaration + "\"choices\":[{\"delta\":{\"content\":\"answer\"},\"finish_reason\":\"stop\"}]," + usage + "}\n\n" + metadata + "data: [DONE]\n\n"
	}
}

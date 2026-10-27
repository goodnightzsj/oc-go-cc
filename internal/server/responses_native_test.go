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
	"github.com/routatic/proxy/internal/storage"
)

func nativeResponsesServer(t *testing.T, platform string, upstream http.HandlerFunc, configure func(*config.Config)) *Server {
	t.Helper()
	source := httptest.NewServer(upstream)
	t.Cleanup(source.Close)
	respect := false
	cfg := &config.Config{
		ActiveSite:            platform,
		OpenCodeGo:            config.OpenCodeGoConfig{APIKey: "synthetic", ResponsesBaseURL: source.URL + "/responses"},
		OpenCodeZen:           config.OpenCodeZenConfig{APIKey: "synthetic", ResponsesBaseURL: source.URL + "/responses"},
		CommandCode:           config.CommandCodeConfig{APIKey: "synthetic", ResponsesBaseURL: source.URL + "/responses", BaseURL: source.URL + "/chat"},
		AWSBedrock:            config.AWSBedrockConfig{APIKey: "synthetic", BaseURL: source.URL + "/openai/v1"},
		Models:                map[string]config.ModelConfig{"default": {Provider: platform, ModelID: "synthetic", WireFormat: "responses", Vision: config.BoolPtr(true)}},
		RespectRequestedModel: &respect,
		Storage:               &config.StorageConfig{DatabasePath: filepath.Join(t.TempDir(), "history.db"), RetentionDays: -1},
	}
	cfg.Models["vision"] = cfg.Models["default"]
	cfg.Fallbacks = map[string][]config.ModelConfig{"vision": {cfg.Models["vision"]}}
	if configure != nil {
		configure(cfg)
	}
	srv, err := NewServer(config.NewAtomicConfig(cfg, ""), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	return srv
}

func TestNativeResponsesPreservesPayloadAndSourceBytes(t *testing.T) {
	for _, platform := range []string{"opencode-go", "opencode-zen", "commandcode", "aws-bedrock"} {
		for _, stream := range []bool{false, true} {
			for _, failed := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/stream=%t/failed=%t", platform, stream, failed), func(t *testing.T) {
					payload := fmt.Sprintf(`{"model":"alias","stream":%t,"input":[{"role":"user","content":[{"type":"input_text","text":"hi"},{"type":"input_image","image_url":"data:image/png;base64,aGk=","detail":"high"}]}],"tools":[{"type":"function","name":"lookup","strict":true,"parameters":{"type":"object","properties":{"q":{"type":"string"}},"required":["q"],"additionalProperties":false}},{"type":"custom","name":"patch","format":{"type":"text"}},{"type":"namespace","name":"ns","tools":[]}],"reasoning":{"effort":"high","summary":"auto"},"include":["reasoning.encrypted_content"],"text":{"verbosity":"low","format":{"type":"json_schema","name":"answer","strict":true,"schema":{"type":"object","properties":{},"additionalProperties":false}}},"prompt_cache_key":"affinity","metadata":{"fixture":"native"},"native_extension":{"keep":true}}`, stream)
					status := "completed"
					if failed {
						status = "failed"
					}
					response := fmt.Sprintf(`{"id":"resp_native","object":"response","model":"actual-model","status":%q,"output":[{"type":"reasoning","id":"rs_1","encrypted_content":"synthetic-opaque","summary":[{"type":"summary_text","text":"public summary"}]},{"type":"custom_tool_call","id":"ct_1","call_id":"call_1","name":"patch","input":"freeform"}],"usage":{"input_tokens":11,"output_tokens":7,"output_tokens_details":{"reasoning_tokens":3}},"native_extension":{"kept":true}}`, status)
					wire := response
					if stream {
						wire = "event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_native\",\"model\":\"actual-model\",\"status\":\"in_progress\"}}\r\n\r\n" +
							"event: response.custom_tool_call_input.delta\ndata: {\"type\":\"response.custom_tool_call_input.delta\",\"delta\":\"freeform\",\"item_id\":\"ct_1\"}\n\n" +
							fmt.Sprintf("event: response.%s\ndata: {\"type\":\"response.%s\",\"response\":%s}\n\n", status, status, response)
					}
					var calls atomic.Int32
					srv := nativeResponsesServer(t, platform, func(w http.ResponseWriter, r *http.Request) {
						calls.Add(1)
						if !strings.HasSuffix(r.URL.Path, "/responses") {
							t.Errorf("wrong endpoint %s", r.URL.Path)
						}
						var got, want map[string]any
						if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
							t.Error(err)
						}
						if err := json.Unmarshal([]byte(payload), &want); err != nil {
							t.Fatal(err)
						}
						want["model"], want["store"] = "synthetic", false
						if !reflect.DeepEqual(got, want) {
							t.Errorf("native payload changed: got=%v want=%v", got, want)
						}
						if stream {
							w.Header().Set("Content-Type", "text/event-stream")
						}
						_, _ = io.WriteString(w, wire)
					}, nil)
					w := httptest.NewRecorder()
					srv.httpSrv.Handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(payload)))
					if w.Code != 200 || w.Body.String() != wire || calls.Load() != 1 {
						t.Fatalf("raw response changed: calls=%d HTTP %d body=%s", calls.Load(), w.Code, w.Body.String())
					}
					rows, total, err := storage.NewRequests(srv.Storage()).Query(t.Context(), storage.RequestQuery{})
					if err != nil || total != 1 {
						t.Fatalf("history total=%d err=%v", total, err)
					}
					row := rows[0]
					if !row.OutcomeKnown || row.Success == failed || row.Model != "synthetic" || row.UpstreamModel != "actual-model" || row.ResponseModel != "actual-model" || row.Provider != platform || row.InputTokens != 11 || row.OutputTokens != 7 || row.ReasoningTokens != 3 {
						t.Fatalf("native accounting changed: %+v", row)
					}
				})
			}
		}
	}
}

func TestNativeResponsesInvalidStreamsFailWithoutReplay(t *testing.T) {
	for _, tail := range []string{
		"", "data: {bad}\n\n", "data: {\"type\":\"response.completed\"}",
		"data: {\"type\":\"response.completed\",\"response\":{\"status\":\"failed\"}}\n\n",
		"data: {\"type\":\"response.cancelled\"}\n\n",
	} {
		t.Run(tail, func(t *testing.T) {
			var calls atomic.Int32
			prefix := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\",\"usage\":{\"input_tokens\":5,\"output_tokens\":2}}\n\n"
			srv := nativeResponsesServer(t, "commandcode", func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				_, _ = io.Copy(io.Discard, r.Body)
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, prefix+tail)
			}, nil)
			w := httptest.NewRecorder()
			srv.httpSrv.Handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"m","input":"hi","stream":true}`)))
			if !strings.HasPrefix(w.Body.String(), prefix+tail) || calls.Load() != 1 {
				t.Fatalf("raw partial changed: %s", w.Body.String())
			}
			if !strings.Contains(w.Body.String(), `"type":"error"`) && !strings.Contains(tail, "response.cancelled") {
				t.Fatalf("missing failure: %s", w.Body.String())
			}
			rows, total, err := storage.NewRequests(srv.Storage()).Query(t.Context(), storage.RequestQuery{})
			if err != nil || total != 1 || rows[0].Success || rows[0].OutputTokens != 2 || rows[0].InputTokens != 5 {
				t.Fatalf("failed stream accounting: %+v total=%d err=%v", rows, total, err)
			}
		})
	}
}

func TestNativeResponsesRejectsCrossProtocolOnlyFeatures(t *testing.T) {
	var calls atomic.Int32
	srv := nativeResponsesServer(t, "commandcode", func(w http.ResponseWriter, r *http.Request) { calls.Add(1); http.Error(w, "must not execute", 500) }, func(cfg *config.Config) {
		cfg.Models["default"] = config.ModelConfig{Provider: "commandcode", ModelID: "synthetic", WireFormat: "openai"}
	})
	for _, tools := range []string{`[{"type":"custom","name":"patch"}]`, `[{"type":"namespace","name":"ns"}]`, `[{"type":"function","name":"f","strict":true}]`} {
		w := httptest.NewRecorder()
		srv.httpSrv.Handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"m","input":"hi","tools":`+tools+`}`)))
		if w.Code != 400 || !strings.Contains(w.Body.String(), "native Responses") {
			t.Fatalf("lost bridge rejection: HTTP %d %s", w.Code, w.Body.String())
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("native-only request executed on Chat: %d", calls.Load())
	}
}

func TestNativeResponsesParallelToolImagesParticipateInRouting(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			calls := 0
			srv := nativeResponsesServer(t, "commandcode", func(w http.ResponseWriter, r *http.Request) {
				calls++
				var req struct {
					Model string
					Input []json.RawMessage
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
				}
				if req.Model != "vision" || len(req.Input) != 2 {
					t.Errorf("parallel image lost in routing: %+v", req)
				}
				response := `{"status":"completed","output":[]}`
				if stream {
					w.Header().Set("Content-Type", "text/event-stream")
					response = "data: {\"type\":\"response.completed\",\"response\":" + response + "}\n\n"
				}
				_, _ = io.WriteString(w, response)
			}, func(cfg *config.Config) {
				cfg.EnableStreamingScenarioRouting = true
				cfg.Models["default"] = config.ModelConfig{Provider: "commandcode", ModelID: "text-only", WireFormat: "responses", Vision: config.BoolPtr(false)}
				cfg.Models["vision"] = config.ModelConfig{Provider: "commandcode", ModelID: "vision", WireFormat: "responses", Vision: config.BoolPtr(true)}
				cfg.Fallbacks["vision"] = []config.ModelConfig{cfg.Models["vision"]}
			})
			payload := fmt.Sprintf(`{"model":"m","stream":%t,"include":["reasoning.encrypted_content"],"input":[{"type":"function_call_output","call_id":"a","output":[{"type":"input_image","image_url":"https://example.invalid/a.png"}]},{"type":"function_call_output","call_id":"b","output":"text"}]}`, stream)
			w := httptest.NewRecorder()
			srv.httpSrv.Handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(payload)))
			if w.Code != 200 || calls != 1 || strings.Contains(w.Body.String(), `"error"`) {
				t.Fatalf("image path: calls=%d HTTP %d %s", calls, w.Code, w.Body.String())
			}
		})
	}
}

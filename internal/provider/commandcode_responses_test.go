package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/internal/debug"
	"github.com/routatic/proxy/pkg/types"
)

func TestCommandCodeExplicitResponsesEndpoint(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			const response = `{"id":"resp-test","object":"response","status":"completed","model":"reported-model","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":9,"input_tokens_details":{"cached_tokens":4},"output_tokens":7,"output_tokens_details":{"reasoning_tokens":5}}}`
			wire := response
			if stream {
				wire = "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":" + response + "}\n\n"
			}
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/custom/responses" || r.URL.RawQuery != "version=test" {
					t.Errorf("wrong endpoint: %s", r.URL)
				}
				if r.Header.Get("Authorization") != "Bearer synthetic-dedicated" || r.Header.Get("x-cmd-zdr") != "1" || r.Header.Get("anthropic-version") != "" {
					t.Error("Responses lost dedicated identity/ZDR or leaked Anthropic negotiation")
				}
				var body map[string]json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				var sentStream bool
				if raw, ok := body["stream"]; ok {
					if err := json.Unmarshal(raw, &sentStream); err != nil {
						t.Error(err)
					}
				}
				if body["input"] == nil || body["messages"] != nil || sentStream != stream || string(body["model"]) != `"synthetic-model"` {
					t.Errorf("wrong Responses payload: %s", body)
				}
				if stream && r.Header.Get("Accept") != "text/event-stream" {
					t.Error("stream Accept missing")
				}
				_, _ = io.WriteString(w, wire)
			}))
			defer upstream.Close()
			cfg, err := config.LoadJSON([]byte(fmt.Sprintf(`{"commandcode":{"api_key":"synthetic-dedicated","responses_base_url":%q,"zero_data_retention":true},"models":{"default":{"provider":"commandcode","model_id":"synthetic-model","wire_format":"responses"}}}`, upstream.URL+"/custom/responses?version=test")))
			if err != nil {
				t.Fatal(err)
			}
			p := NewCommandCodeProvider(config.NewAtomicConfig(cfg, ""), nil)
			model := config.ModelConfig{Provider: "commandcode", ModelID: "synthetic-model", WireFormat: "responses"}
			if stream {
				body, err := p.Stream(context.Background(), commandCodeRequest(), model)
				if err != nil {
					t.Fatal(err)
				}
				defer body.Close()
				got, err := io.ReadAll(body)
				if err != nil || string(got) != wire || client.KeyHintOf(body) == "" {
					t.Fatalf("stream or key observation lost: %q, %v", got, err)
				}
				return
			}
			result, err := p.Execute(context.Background(), commandCodeRequest(), model)
			if err != nil {
				t.Fatal(err)
			}
			var message types.MessageResponse
			if err := json.Unmarshal(result.Body, &message); err != nil {
				t.Fatal(err)
			}
			if message.Usage == nil || message.Usage.InputTokens != 5 || message.Usage.CacheReadInputTokens != 4 || message.Usage.OutputTokens != 7 || message.Usage.ReasoningTokens != 5 || result.KeyHint == "" {
				t.Fatalf("lost usage/key: %+v", result)
			}
		})
	}
}

func TestCommandCodeCaptureKeepsKeyHintAcrossFormats(t *testing.T) {
	for format, response := range map[string]string{
		"openai":    `{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`,
		"anthropic": `{"type":"message","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn"}`,
		"responses": `{"status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}]}`,
	} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", format, stream), func(t *testing.T) {
				p, _ := commandCodeTestProvider(t, func(w http.ResponseWriter, _ *http.Request) {
					_, _ = io.WriteString(w, response)
				})
				store, err := debug.NewStorage(config.DebugCapture{Enabled: true, Directory: filepath.Join(t.TempDir(), "capture"), MaxFiles: 2, MaxFileSize: 1 << 20})
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = store.Close() })
				p.capture = debug.NewCaptureLogger(store, true)
				t.Cleanup(func() { _ = p.capture.Close() })
				model := config.ModelConfig{Provider: "commandcode", ModelID: "synthetic", WireFormat: format}
				var hint string
				if stream {
					body, err := p.Stream(t.Context(), commandCodeRequest(), model)
					if err != nil {
						t.Fatal(err)
					}
					defer body.Close()
					if _, err := io.Copy(io.Discard, body); err != nil {
						t.Fatal(err)
					}
					hint = client.KeyHintOf(body)
				} else {
					result, err := p.Execute(t.Context(), commandCodeRequest(), model)
					if err != nil {
						t.Fatal(err)
					}
					hint = result.KeyHint
				}
				if hint != "****-key" {
					t.Fatalf("capture erased key hint: %q", hint)
				}
			})
		}
	}
}

func TestCommandCodeResponsesFailureRetainsUsage(t *testing.T) {
	p, _ := commandCodeTestProvider(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"object":"response","status":"failed","usage":{"input_tokens":4,"output_tokens":2}}`)
	})
	_, err := p.Execute(context.Background(), commandCodeRequest(), config.ModelConfig{ModelID: "synthetic", WireFormat: "responses"})
	var failure *core.ResponsesStatusError
	if !errors.As(err, &failure) || failure.Usage == nil || failure.Usage.OutputTokens != 2 {
		t.Fatalf("failure/usage lost: %v", err)
	}
}

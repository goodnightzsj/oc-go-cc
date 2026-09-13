package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/pkg/types"
)

// commandCodeTestProvider wires both upstream endpoints at one test server and
// returns the provider plus the model ids the caller should ask for.
func commandCodeTestProvider(t *testing.T, handler http.HandlerFunc) (*CommandCodeProvider, *httptest.Server) {
	t.Helper()
	upstream := httptest.NewServer(handler)
	t.Cleanup(upstream.Close)
	cfg := &config.Config{CommandCode: config.CommandCodeConfig{
		APIKey:           "dedicated-key",
		BaseURL:          upstream.URL + "/provider/v1/chat/completions",
		AnthropicBaseURL: upstream.URL + "/provider/v1/messages",
	}}
	return NewCommandCodeProvider(config.NewAtomicConfig(cfg, ""), nil), upstream
}

func commandCodeRequest() *types.MessageRequest {
	return &types.MessageRequest{
		Model:     "commandcode",
		MaxTokens: 4096,
		Messages:  []types.Message{{Role: "user", Content: json.RawMessage(`"hi"`)}},
		Tools: []types.Tool{{
			Name:        "Read",
			Description: "read a file",
			InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
		}},
		// Fields the cross-protocol DTO does not model. The native path must
		// forward them rather than rebuild the request from our own structs.
		OutputConfig: json.RawMessage(`{"format":{"type":"json_schema","schema":{"type":"object"}}}`),
	}
}

func TestCommandCodeWireFormatSelection(t *testing.T) {
	p, _ := commandCodeTestProvider(t, func(http.ResponseWriter, *http.Request) {})
	for _, tc := range []struct {
		model config.ModelConfig
		want  string
	}{
		{config.ModelConfig{ModelID: "claude-sonnet-4-6"}, "anthropic"},
		{config.ModelConfig{ModelID: "claude-opus-5"}, "anthropic"},
		{config.ModelConfig{ModelID: "deepseek/deepseek-v4-flash"}, "openai"},
		{config.ModelConfig{ModelID: "moonshotai/Kimi-K2.7-Code"}, "openai"},
		// An explicit choice wins over the prefix, so a mirrored or renamed
		// model can still be pinned to the endpoint it actually speaks.
		{config.ModelConfig{ModelID: "claude-sonnet-4-6", WireFormat: "openai"}, "openai"},
		{config.ModelConfig{ModelID: "vendor/claude-proxy", WireFormat: "anthropic"}, "anthropic"},
	} {
		if got := core.ModelWireFormat(p, tc.model).String(); got != tc.want {
			t.Errorf("%s (wire_format=%q) = %s, want %s", tc.model.ModelID, tc.model.WireFormat, got, tc.want)
		}
	}
}

// The Messages endpoint is native passthrough: what Claude Code sent must arrive
// intact apart from the model id, the stream flag and max_tokens.
func TestCommandCodeAnthropicEndpointPassthrough(t *testing.T) {
	var path, auth, version, beta string
	var body map[string]json.RawMessage
	p, _ := commandCodeTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		path, auth, version, beta = r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("anthropic-version"), r.Header.Get("anthropic-beta")
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode upstream body: %v", err)
		}
		_, _ = io.WriteString(w, `{"id":"msg-1","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[{"type":"text","text":"native-ok"}],"stop_reason":"end_turn","usage":{"input_tokens":5,"output_tokens":2}}`)
	})
	ctx := core.WithRequestMetadata(context.Background(), core.RequestMetadata{
		AnthropicVersion: "2023-06-01", AnthropicBeta: "fine-grained-tool-streaming-2025-05-14",
	})
	model := config.ModelConfig{Provider: "commandcode", ModelID: "claude-sonnet-4-6", MaxTokens: 8192}

	result, err := p.Execute(ctx, commandCodeRequest(), model)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if path != "/provider/v1/messages" || auth != "Bearer dedicated-key" {
		t.Fatalf("addressed %q with auth %q", path, auth)
	}
	if version != "2023-06-01" || beta != "fine-grained-tool-streaming-2025-05-14" {
		t.Fatalf("Anthropic negotiation headers lost: version=%q beta=%q", version, beta)
	}
	if got := string(body["model"]); got != `"claude-sonnet-4-6"` {
		t.Fatalf("model = %s, want the real upstream id (the local alias must not leak)", got)
	}
	if got := string(body["max_tokens"]); got != "4096" {
		t.Fatalf("max_tokens = %s, want the request's own 4096", got)
	}
	if body["stream"] != nil && string(body["stream"]) != "null" {
		t.Fatalf("non-streaming request must not ask for a stream: %s", body["stream"])
	}
	for _, field := range []string{"tools", "output_config", "messages"} {
		if body[field] == nil {
			t.Fatalf("%s was dropped on the native passthrough path", field)
		}
	}
	var parsed types.MessageResponse
	if err := json.Unmarshal(result.Body, &parsed); err != nil || parsed.Type != "message" || len(parsed.Content) == 0 {
		t.Fatalf("response is not a usable Messages payload: %v %s", err, result.Body)
	}
	if parsed.Usage.InputTokens != 5 || parsed.Usage.OutputTokens != 2 {
		t.Fatalf("usage lost: %+v", parsed.Usage)
	}
}

func TestCommandCodeAnthropicStreamPassthrough(t *testing.T) {
	p, _ := commandCodeTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]json.RawMessage
		_ = json.NewDecoder(r.Body).Decode(&body)
		if string(body["stream"]) != "true" || r.Header.Get("Accept") != "text/event-stream" {
			t.Errorf("streaming request was not declared as such: stream=%s accept=%q", body["stream"], r.Header.Get("Accept"))
		}
		_, _ = io.WriteString(w, "event: message_start\ndata: {\"type\":\"message_start\"}\n\n")
		_, _ = io.WriteString(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	})
	body, err := p.Stream(context.Background(), commandCodeRequest(), config.ModelConfig{ModelID: "claude-sonnet-4-6"})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	defer func() { _ = body.Close() }()
	sse, err := io.ReadAll(body)
	// No translation happens on this path, so anything other than verbatim
	// Anthropic SSE means the handler downstream would parse the wrong protocol.
	if err != nil || !strings.Contains(string(sse), "event: message_start") || !strings.Contains(string(sse), "event: message_stop") {
		t.Fatalf("stream is not Anthropic SSE: %v %q", err, sse)
	}
}

func TestCommandCodeChatCompletionsEndpoint(t *testing.T) {
	var path string
	var body map[string]json.RawMessage
	p, _ := commandCodeTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode upstream body: %v", err)
		}
		_, _ = io.WriteString(w, `{"id":"chat-1","object":"chat.completion","model":"deepseek/deepseek-v4-flash","choices":[{"index":0,"message":{"role":"assistant","content":"ok","reasoning_content":"thought"},"finish_reason":"stop"}],"usage":{"prompt_tokens":7,"completion_tokens":3,"prompt_tokens_details":{"cached_tokens":4}}}`)
	})
	model := config.ModelConfig{Provider: "commandcode", ModelID: "deepseek/deepseek-v4-flash"}

	result, err := p.Execute(context.Background(), commandCodeRequest(), model)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if path != "/provider/v1/chat/completions" {
		t.Fatalf("addressed %q, want the Chat Completions endpoint", path)
	}
	if got := string(body["model"]); got != `"deepseek/deepseek-v4-flash"` {
		t.Fatalf("model = %s", got)
	}
	if body["messages"] == nil {
		t.Fatal("messages missing from the converted payload")
	}
	var parsed types.MessageResponse
	if err := json.Unmarshal(result.Body, &parsed); err != nil || parsed.Type != "message" {
		t.Fatalf("response was not converted to a Messages payload: %v %s", err, result.Body)
	}
	// The caller reads accounting straight off this payload, so a conversion
	// that drops usage would silently record zero tokens.
	if parsed.Usage.InputTokens != 3 || parsed.Usage.OutputTokens != 3 || parsed.Usage.CacheReadInputTokens != 4 {
		t.Fatalf("usage not carried into the Anthropic payload: %+v", parsed.Usage)
	}
	var sawThinking bool
	for _, block := range parsed.Content {
		if block.Type == "thinking" && block.Thinking == "thought" {
			sawThinking = true
		}
	}
	if !sawThinking {
		t.Fatalf("reasoning_content was dropped, so the next turn cannot round-trip it: %+v", parsed.Content)
	}
}

func TestCommandCodeFailuresAreExplicit(t *testing.T) {
	t.Run("missing max_tokens", func(t *testing.T) {
		p, _ := commandCodeTestProvider(t, func(http.ResponseWriter, *http.Request) {
			t.Error("an unsatisfiable request reached upstream")
		})
		req := &types.MessageRequest{Model: "commandcode", Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"hi"`)}}}
		_, err := p.Execute(context.Background(), req, config.ModelConfig{ModelID: "claude-sonnet-4-6"})
		if err == nil || !strings.Contains(err.Error(), "max_tokens") {
			t.Fatalf("native Messages without max_tokens must fail loudly, got %v", err)
		}
	})

	t.Run("upstream error status", func(t *testing.T) {
		p, _ := commandCodeTestProvider(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"error":{"message":"bad key"}}`)
		})
		_, err := p.Execute(context.Background(), commandCodeRequest(), config.ModelConfig{ModelID: "claude-sonnet-4-6"})
		if err == nil {
			t.Fatal("a 401 was reported as success")
		}
	})

	t.Run("failure body is not an Anthropic message", func(t *testing.T) {
		p, _ := commandCodeTestProvider(t, func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, `{"error":{"message":"quota"}}`)
		})
		_, err := p.Execute(context.Background(), commandCodeRequest(), config.ModelConfig{ModelID: "claude-sonnet-4-6"})
		if err == nil {
			t.Fatal("an error envelope was passed through as a successful response")
		}
	})

	t.Run("redirects are not followed", func(t *testing.T) {
		var redirected atomic.Int32
		target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { redirected.Add(1) }))
		defer target.Close()
		p, _ := commandCodeTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
		})
		if _, err := p.Execute(context.Background(), commandCodeRequest(), config.ModelConfig{ModelID: "claude-sonnet-4-6"}); err == nil {
			t.Fatal("a redirect was treated as a successful upstream response")
		}
		if redirected.Load() != 0 {
			t.Fatal("the dedicated credential was carried to the redirect target")
		}
	})
}

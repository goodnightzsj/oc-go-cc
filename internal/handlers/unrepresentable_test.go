package handlers

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/internal/metrics"
	"github.com/routatic/proxy/internal/provider"
	"github.com/routatic/proxy/internal/router"
	"github.com/routatic/proxy/internal/token"
)

// Claude Code's ToolSearch returns a tool_result carrying a tool_reference
// block, which Chat Completions cannot express. Every CommandCode model except
// the Claude ones is served over Chat Completions, so this arrives on the
// ordinary path.
//
// Unsupported tool content must fail before sending a lossy request upstream.
func TestHandleMessages_ToolReferenceIsNotSilentlyDropped(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"c1","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":1,"total_tokens":6}}`))
	}))
	defer upstream.Close()

	atomicCfg := config.NewAtomicConfig(&config.Config{
		CommandCode: config.CommandCodeConfig{
			APIKey:            "test-key",
			BaseURL:           upstream.URL + "/chat/completions",
			AnthropicBaseURL:  upstream.URL + "/messages",
			ZeroDataRetention: false,
		},
		Models: map[string]config.ModelConfig{
			"default": {Provider: "commandcode", ModelID: "deepseek-v4.1-flash"},
		},
		RespectRequestedModel: boolPtr(false),
	}, "/tmp/test-config.json")

	modelRouter := router.NewModelRouter(atomicCfg)
	tokenCounter, err := token.NewCounter()
	if err != nil {
		t.Fatalf("NewCounter: %v", err)
	}
	registry := core.NewProviderRegistry()
	if err := registry.Register(provider.NewCommandCodeProvider(atomicCfg, nil)); err != nil {
		t.Fatal(err)
	}
	handler := NewMessagesHandler(
		client.NewOpenCodeClient(atomicCfg), registry, modelRouter,
		router.NewFallbackHandler(slog.Default(), 3, 0),
		tokenCounter, metrics.New(), nil, nil,
	)
	handler.logger = slog.Default()

	body := `{"model":"claude-opus-4","max_tokens":256,"messages":[
		{"role":"assistant","content":[{"type":"tool_use","id":"call_1","name":"Monitor","input":{}}]},
		{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_1","content":[{"type":"tool_reference","tool_name":"Monitor"}]}]}
	]}`
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	handler.HandleMessages(recorder, req)

	if recorder.Code != http.StatusBadRequest || calls.Load() != 0 || !strings.Contains(recorder.Body.String(), `"type":"invalid_request_error"`) || !strings.Contains(recorder.Body.String(), "tool_reference") {
		t.Fatalf("unsupported tool content was sent or became success: status=%d calls=%d body=%s", recorder.Code, calls.Load(), recorder.Body.String())
	}
}

package handlers

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
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
// The block is dropped rather than failing the request, matching the reference
// CommandCode proxy, which maps the same block to an empty string. The trade is
// deliberate: the request succeeds and the model proceeds without the tool's
// content. This test pins that the request is NOT rejected - if that changes
// back to a 4xx, this fails and the reason should be reconsidered on purpose.
func TestHandleMessages_ToolReferenceIsDroppedNotRejected(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		client.NewOpenCodeClient(atomicCfg, nil), registry, modelRouter,
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

	if recorder.Code == http.StatusBadRequest {
		t.Fatalf("a tool_reference block must not fail the request; got 400: %s", recorder.Body.String())
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
}

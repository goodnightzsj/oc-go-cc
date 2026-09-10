package handlers

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/internal/metrics"
	"github.com/routatic/proxy/internal/middleware"
	"github.com/routatic/proxy/internal/router"
	"github.com/routatic/proxy/internal/storage"
	"github.com/routatic/proxy/pkg/types"
)

type fragmentedAnthropicProvider struct{ usageLimitStreamProvider }

func (p *fragmentedAnthropicProvider) Stream(context.Context, *types.MessageRequest, config.ModelConfig) (io.ReadCloser, error) {
	return io.NopCloser(iotest.OneByteReader(strings.NewReader(p.body))), nil
}

func TestNativeAnthropicStreamPreservesInitialAndTerminalUsage(t *testing.T) {
	const body = "event: message_start\r\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":11,\"output_tokens\":0,\"cache_read_input_tokens\":90,\"cache_creation_input_tokens\":3}}}\r\n\r\n" +
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"usage input_tokens: 999\"}}\n\n" +
		"event: message_delta\ndata: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":7}}\n\n" +
		"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
	cfg := config.NewAtomicConfig(&config.Config{}, "")
	reg := core.NewProviderRegistry()
	if err := reg.Register(&fragmentedAnthropicProvider{usageLimitStreamProvider: usageLimitStreamProvider{name: "opencode-zen", body: body}}); err != nil {
		t.Fatal(err)
	}
	db, err := storage.Open(storage.Config{DatabasePath: filepath.Join(t.TempDir(), "usage.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &MessagesHandler{
		client: client.NewOpenCodeClient(cfg, nil), providerRegistry: reg,
		streamProxy: NewStreamProxy(), logger: slog.Default(), metrics: metrics.New(),
		storage: NewStorageAdapter(db),
	}
	w := httptest.NewRecorder()
	h.handleStreaming(w, httptest.NewRequest(http.MethodPost, "/v1/messages", nil),
		&types.MessageRequest{Stream: boolPtr(true)}, []config.ModelConfig{{Provider: "opencode-zen", ModelID: "synthetic"}},
		nil, router.ScenarioDefault, "fragmented-usage")
	if got := w.Body.String(); got != body {
		t.Fatalf("native SSE changed: %q", got)
	}
	rows, total, err := storage.NewRequests(db).Query(storage.RequestQuery{Page: 1, PageSize: 10})
	if err != nil || total != 1 {
		t.Fatalf("stored rows=%d err=%v", total, err)
	}
	r := rows[0]
	if r.InputTokens != 11 || r.OutputTokens != 7 || r.CacheReadTokens != 90 || r.CacheCreationTokens != 3 {
		t.Fatalf("usage = input %d output %d cache %d/%d; want 11/7/90/3", r.InputTokens, r.OutputTokens, r.CacheReadTokens, r.CacheCreationTokens)
	}
}

func routeNativeCommandCode(h *MessagesHandler) {
	h.modelRouter = router.NewModelRouter(config.NewAtomicConfig(&config.Config{
		Models:                map[string]config.ModelConfig{"default": {Provider: "commandcode", ModelID: "claude-test", MaxTokens: 1024}},
		RespectRequestedModel: boolPtr(false),
	}, ""))
}

func TestNativeMessagesPreserveUnmodeledFieldsAndProtocolHeaders(t *testing.T) {
	var received map[string]json.RawMessage
	h, _ := newResponsesTestHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/messages" || r.Header.Get("Authorization") != "Bearer synthetic-test-key" {
			t.Errorf("wrong native destination/auth: %s", r.URL.Path)
		}
		if r.Header.Get("anthropic-version") != "2023-06-01" || r.Header.Get("anthropic-beta") != "synthetic-beta-a,synthetic-beta-b" {
			t.Errorf("native protocol headers were lost")
		}
		if r.Header.Get("x-opencode-session") != "" || r.Header.Get("x-api-key") != "" {
			t.Error("another platform's identity was forwarded to CommandCode")
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Error(err)
		}
		_, _ = io.WriteString(w, `{"type":"message","model":"claude-test","content":[{"type":"text","text":"answer"}],"stop_reason":"end_turn","usage":{"input_tokens":11,"output_tokens":7}}`)
	}))
	routeNativeCommandCode(h)
	h.rateLimiter = middleware.NewRateLimiter(1, time.Minute)
	r := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{
		"model":"client-model","max_tokens":200,"messages":[{"role":"user","content":"hi"}],
		"context_management":{"edits":[]},"stop_sequences":["END"],"service_tier":"auto"
	}`))
	r.Header.Set("anthropic-version", "2023-06-01")
	r.Header.Add("anthropic-beta", "synthetic-beta-a")
	r.Header.Add("anthropic-beta", "synthetic-beta-b")
	r.Header.Set("x-claude-code-session-id", "synthetic-session")
	r.Header.Set("x-api-key", "synthetic-client-credential")
	w := httptest.NewRecorder()
	h.HandleMessages(w, r)
	if w.Code != http.StatusOK || string(received["model"]) != `"claude-test"` ||
		string(received["context_management"]) != `{"edits":[]}` || string(received["stop_sequences"]) != `["END"]` ||
		string(received["service_tier"]) != `"auto"` || string(received["max_tokens"]) != "200" {
		t.Fatalf("native fields changed: status=%d body=%s received=%s", w.Code, w.Body.String(), received)
	}
}

type failedDeliveryWriter struct{ *httptest.ResponseRecorder }

func (w failedDeliveryWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestNonStreamingDeliveryFailurePreservesUsageOnce(t *testing.T) {
	for _, mode := range []string{"conversion", "write"} {
		t.Run(mode, func(t *testing.T) {
			blockType := "text"
			if mode == "conversion" {
				blockType = "unsupported"
			}
			h, db := newResponsesTestHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(w, `{"type":"message","model":"claude-test","content":[{"type":"`+blockType+`","text":"answer"}],"stop_reason":"end_turn","usage":{"input_tokens":11,"output_tokens":7,"cache_read_input_tokens":90}}`)
			}))
			routeNativeCommandCode(h)
			w := httptest.NewRecorder()
			var output http.ResponseWriter = w
			if mode == "write" {
				output = failedDeliveryWriter{w}
			}
			h.HandleResponses(output, httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"m","input":"hi"}`)))
			rows, total, err := storage.NewRequests(db).Query(storage.RequestQuery{Page: 1, PageSize: 10})
			if err != nil || total != 1 || rows[0].Success || rows[0].InputTokens != 11 || rows[0].OutputTokens != 7 || rows[0].CacheReadTokens != 90 {
				t.Fatalf("delivery failure accounting=%+v total=%d err=%v", rows, total, err)
			}
			if mode == "conversion" && (w.Code != http.StatusBadGateway || strings.Count(w.Body.String(), `"error"`) != 1) {
				t.Fatalf("conversion failure was not emitted exactly once: %d %s", w.Code, w.Body.String())
			}
		})
	}
}

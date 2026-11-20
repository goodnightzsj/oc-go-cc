package handlers

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/metrics"
	"github.com/routatic/proxy/internal/router"
	"github.com/routatic/proxy/internal/token"
)

// A client that sends an oversized model id must not write it to SQLite, the
// journal or the breaker map: the id is echoed, logged and stored verbatim, so
// its length is a storage and memory bound the client would otherwise control.
// The direct path already bounded its own copy.
func TestOversizedModelIDIsBoundedAtIngestion(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"c1","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`)
	}))
	defer upstream.Close()

	store := &captureRequestStorage{}
	atomicCfg := config.NewAtomicConfig(&config.Config{
		OpenCodeGo: config.OpenCodeGoConfig{BaseURL: upstream.URL, APIKey: "k"},
		Models:     map[string]config.ModelConfig{"default": {Provider: "opencode-go", ModelID: "kimi-k2.6"}},
	}, "")
	counter, err := token.NewCounter()
	if err != nil {
		t.Fatal(err)
	}
	h := NewMessagesHandler(client.NewOpenCodeClient(atomicCfg), newTestProviderRegistry(t, atomicCfg),
		router.NewModelRouter(atomicCfg), router.NewFallbackHandler(nil, 3, time.Minute), counter, metrics.New(), nil, store)
	h.logger = slog.New(slog.NewTextHandler(io.Discard, nil))

	rec := httptest.NewRecorder()
	h.HandleMessages(rec, httptest.NewRequest(http.MethodPost, "/v1/messages",
		strings.NewReader(`{"model":"`+strings.Repeat("m", 4096)+`","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`)))
	rows := store.records()
	if len(rows) != 1 {
		t.Fatalf("status %d, recorded %d rows: %s", rec.Code, len(rows), rec.Body.String())
	}
	if got := len(rows[0].RequestedModel); got != 256 {
		t.Fatalf("requested_model is %d bytes, want 256", got)
	}
}

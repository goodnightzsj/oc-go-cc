package handlers

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/routatic/proxy/internal/metrics"
	"github.com/routatic/proxy/internal/middleware"
)

// A refusal that happens before routing is still an answer this API documents.
// These used http.Error, so a Claude Code client got a plain-text body, no
// Retry-After on the 429 and no Allow on the 405 - and parsed none of it.
func TestAdmissionErrorsUseTheDocumentedShape(t *testing.T) {
	h := &MessagesHandler{rateLimiter: middleware.NewRateLimiter(1, rateLimitWindow), requestIDGen: middleware.NewRequestIDGenerator(),
		logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		metrics: metrics.New()}

	method := httptest.NewRecorder()
	h.HandleMessages(method, httptest.NewRequest(http.MethodGet, "/v1/messages", nil))
	if method.Code != http.StatusMethodNotAllowed || method.Header().Get("Allow") != http.MethodPost {
		t.Fatalf("method: %d Allow=%q", method.Code, method.Header().Get("Allow"))
	}

	first := httptest.NewRecorder()
	h.HandleMessages(first, httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader("{}")))
	second := httptest.NewRecorder()
	h.HandleMessages(second, httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader("{}")))
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second request: %d, want 429", second.Code)
	}
	if second.Header().Get("Retry-After") == "" {
		t.Fatal("429 carries no Retry-After")
	}
	var body struct {
		Type  string `json:"type"`
		Error struct {
			Type string `json:"type"`
		} `json:"error"`
	}
	if err := json.Unmarshal(second.Body.Bytes(), &body); err != nil || body.Type != "error" || body.Error.Type != "rate_limit_error" {
		t.Fatalf("429 body is not the API error shape: %q (%v)", second.Body.String(), err)
	}
}

package handlers

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/core"
)

// An Anthropic error event is terminal. When the upstream's own event already
// reached the caller, a second synthesized one both violates that and hides the
// real cause from a client that shows the last error it saw.
func TestUpstreamErrorEventIsNotFollowedByASecondOne(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: rec}
	upstream := "event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\",\"message\":\"Overloaded\"}}\n\n"
	if _, err := rw.Write([]byte(upstream)); err != nil {
		t.Fatal(err)
	}
	h := &MessagesHandler{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	h.sendStreamError(rw, "upstream reported an SSE error", nil)
	if got := rec.Body.String(); got != upstream {
		t.Fatalf("a second error event was appended: %q", got)
	}
}

// A client that branches on the documented error type could not tell a rate
// limit from a server fault: every event said api_error.
func TestStreamErrorTypeFollowsTheCause(t *testing.T) {
	for _, tc := range []struct {
		cause  error
		status string
	}{
		{&client.APIError{StatusCode: http.StatusTooManyRequests}, "rate_limit_error"},
		{&client.APIError{StatusCode: http.StatusServiceUnavailable}, "overloaded_error"},
		{&client.APIError{StatusCode: http.StatusBadRequest}, "invalid_request_error"},
		{&client.APIError{StatusCode: http.StatusBadGateway}, "api_error"},
		{core.ErrUpstreamStreamFailed, "api_error"},
	} {
		// StatusCode is only populated when an upstream status was involved;
		// a bare sentinel keeps the generic type.
		details := streamErrorDetails("x", tc.cause)
		if details.Type != tc.status {
			t.Fatalf("%v: type %q, want %q", tc.cause, details.Type, tc.status)
		}
	}
}

// Every refusal route answers in the shape this API documents, with Allow on a
// 405 - Claude Code parses bodies, and a plain-text one is reported generically.
func TestRefusalRoutesUseTheDocumentedShape(t *testing.T) {
	for _, tc := range []struct {
		name   string
		method string
		serve  func(w http.ResponseWriter, r *http.Request)
	}{
		{"models", http.MethodPost, (&ModelsHandler{}).HandleListModels},
		{"count_tokens", http.MethodGet, (&HealthHandler{}).HandleCountTokens},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			serve := tc.serve
			func() {
				defer func() { _ = recover() }()
				serve(rec, httptest.NewRequest(tc.method, "/", nil))
			}()
			if rec.Code != http.StatusMethodNotAllowed {
				t.Fatalf("status %d", rec.Code)
			}
			if rec.Header().Get("Allow") == "" {
				t.Fatal("405 carries no Allow")
			}
			var body struct {
				Error struct {
					Type string `json:"type"`
				} `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Error.Type != "invalid_request_error" {
				t.Fatalf("405 body is not the API error shape: %q", rec.Body.String())
			}
		})
	}
}

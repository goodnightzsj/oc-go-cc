package handlers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A caller's own 4xx is the upstream's answer, not a sign the path is down. The
// terminal-event check only applies to a 2xx stream: a JSON error body never
// contains message_stop, and judging it by that marker cooled the shared gate -
// one expired key diverted every other caller to the platform chain.
func TestDirectClientErrorDoesNotCoolTheGate(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		stream bool
	}{
		{"401 json body", http.StatusUnauthorized, `{"type":"error","error":{"message":"invalid x-api-key"}}`, true},
		{"400 json body", http.StatusBadRequest, `{"type":"error","error":{"message":"bad request"}}`, true},
		{"401 empty body", http.StatusUnauthorized, "", true},
		{"200 empty body", http.StatusOK, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer upstream.Close()

			h := newAnthropicFirstTestHandler(upstream.URL, true, http.NotFoundHandler())
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"stream":true}`)))

			h.gate.mu.Lock()
			unavailable := h.gate.unavailable
			h.gate.mu.Unlock()
			// A 200 with nothing behind it is a real failure and may cool the
			// path; a non-2xx is the caller's answer and must not.
			wantUnavailable := tc.status == http.StatusOK
			if unavailable != wantUnavailable {
				t.Fatalf("status %d: unavailable=%t want %t", tc.status, unavailable, wantUnavailable)
			}
		})
	}
}

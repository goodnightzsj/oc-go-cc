package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/core"
)

func TestStreamErrorMetadataPreservesKnownCause(t *testing.T) {
	for _, tc := range []struct {
		name, retry string
		cause       error
		status      int
	}{
		{"rate limit", "Thu, 19 Nov 2026 00:00:00 GMT", &client.APIError{StatusCode: 429, RetryAfter: "Thu, 19 Nov 2026 00:00:00 GMT"}, 429},
		{"entitlement", "", &client.APIError{StatusCode: 403}, 403},
		{"missing retry header", "", &client.APIError{StatusCode: 429}, 429},
		{"unknown transport failure", "", errors.New("synthetic reset"), 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			details := streamErrorDetails("original message", fmt.Errorf("wrapped: %w", tc.cause))
			if details.StatusCode != tc.status || details.RetryAfter != tc.retry || details.RetryAt != "" || details.Message != "original message" {
				t.Fatalf("cause metadata changed: %+v", details)
			}
			// Native Responses must preserve the same metadata and error envelope.
			out := httptest.NewRecorder()
			rw := &responseWriter{ResponseWriter: out}
			rw.setSourceFormat(core.WireFormatOpenAIResponses, true)
			h := &MessagesHandler{logger: slog.Default()}
			h.sendStreamError(rw, details.Message, tc.cause)
			_, data, ok := strings.Cut(out.Body.String(), "data: ")
			if !ok {
				t.Fatalf("native error event missing: %s", out.Body.String())
			}
			raw := strings.TrimSpace(data)
			var event map[string]any
			if err := json.Unmarshal([]byte(raw), &event); err != nil {
				t.Fatal(err)
			}
			if event["type"] != "error" || event["code"] != "upstream_error" || event["message"] != details.Message {
				t.Fatalf("native envelope changed: %s", raw)
			}
			if tc.status != 0 && event["status_code"] != float64(tc.status) || tc.status == 0 && event["status_code"] != nil {
				t.Fatalf("native status changed: %s", raw)
			}
			if tc.retry != "" && event["retry_after"] != tc.retry || tc.retry == "" && event["retry_after"] != nil {
				t.Fatalf("native retry changed: %s", raw)
			}
		})
	}
}

package transformer

import (
	"testing"

	"github.com/routatic/proxy/pkg/types"
)

// A usage object that carries no recognised figure is unreported, not a
// measured zero. Committing it as measured stored the request with
// details_known = 1 and a cost of 0.0, which entered every total and the Quota
// tab's per-model ledger as a genuinely free request.
func TestUnrecognisedUsageIsNotAMeasuredZero(t *testing.T) {
	for _, tc := range []struct {
		name     string
		usage    string
		measured bool
	}{
		{"empty-object", `,"usage":{}`, false},
		{"zeroes", `,"usage":{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0}`, false},
		// A total cannot be split into input and output, so recording it would
		// claim a measured request whose cost is zero.
		{"total-only", `,"usage":{"total_tokens":42}`, false},
		{"details-empty", `,"usage":{"prompt_tokens_details":{}}`, false},
		{"counts", `,"usage":{"prompt_tokens":11,"completion_tokens":7}`, true},
		{"cache-only", `,"usage":{"prompt_cache_hit_tokens":9}`, true},
		{"absent", ``, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newMockResponseWriter()
			terminal := `{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]` + tc.usage + `}`
			body := sseLines(`{"choices":[{"index":0,"delta":{"content":"ok"}}]}`, terminal, "[DONE]")
			if err := NewStreamHandler().ProxyStream(w, body, "synthetic", t.Context(), 0, func() {}); err != nil {
				t.Fatal(err)
			}
			var seen *types.Usage
			for _, event := range parseSSEEvents(t, w.buf.String()) {
				if event.Type == "message_delta" {
					seen = event.Usage
				}
			}
			if (seen != nil) != tc.measured {
				t.Fatalf("usage = %+v, want measured=%t", seen, tc.measured)
			}
		})
	}
}

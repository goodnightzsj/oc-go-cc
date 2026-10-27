package transformer

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/iotest"
)

func TestUpstreamSSEEventFraming(t *testing.T) {
	h := NewStreamHandler()
	for _, tc := range []struct {
		name, content, terminal string
		proxy                   func(http.ResponseWriter, io.ReadCloser) error
	}{
		{"chat", `{"choices":[{"delta":{"content":"sentinel"}}]}`, `{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
			func(w http.ResponseWriter, r io.ReadCloser) error {
				return h.ProxyStream(w, r, "synthetic", t.Context(), 0, func() {})
			}},
		{"responses", `{"type":"response.output_text.delta","delta":"sentinel"}`, `{"type":"response.completed","response":{"status":"completed"}}`,
			func(w http.ResponseWriter, r io.ReadCloser) error {
				return h.ProxyResponsesStream(w, r, "synthetic", t.Context(), 0, func() {})
			}},
		{"gemini", `{"candidates":[{"content":{"parts":[{"text":"sentinel"}]}}]}`, `{"candidates":[{"finishReason":"STOP"}]}`,
			func(w http.ResponseWriter, r io.ReadCloser) error {
				return h.ProxyGeminiStream(w, r, "synthetic", t.Context(), 0, func() {})
			}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, newline := range []string{"\n", "\r\n", "\r"} {
				for _, space := range []string{"", " "} {
					// A newline between JSON members is part of one SSE event, not two JSON documents.
					multiline := tc.content[:1] + newline + "data:" + space + tc.content[1:]
					raw := "\xef\xbb\xbf: heartbeat" + newline + newline + "data:" + space + multiline + newline + newline + "data:" + space + tc.terminal + newline + newline
					w := httptest.NewRecorder()
					if err := tc.proxy(w, io.NopCloser(iotest.OneByteReader(strings.NewReader(raw)))); err != nil || !strings.Contains(w.Body.String(), "sentinel") || !strings.Contains(w.Body.String(), "message_stop") {
						t.Fatalf("newline=%q space=%q err=%v body=%s", newline, space, err, w.Body.String())
					}
				}
			}
			for _, suffix := range []string{"", "\n"} {
				w := httptest.NewRecorder()
				raw := "data: " + tc.content + "\n\ndata: " + tc.terminal + suffix
				if err := tc.proxy(w, io.NopCloser(strings.NewReader(raw))); err == nil || strings.Contains(w.Body.String(), "message_stop") {
					t.Fatalf("unterminated event became success: suffix=%q err=%v body=%s", suffix, err, w.Body.String())
				}
			}
		})
	}
}

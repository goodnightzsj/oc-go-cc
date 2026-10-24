package handlers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

func TestStreamTimingSeparatesEnvelopeContentAndEnd(t *testing.T) {
	for _, content := range []struct{ name, event string }{
		{"text", `{"type":"content_block_delta","delta":{"type":"text_delta","text":" "}}`},
		{"thinking", `{"type":"content_block_delta","delta":{"type":"thinking_delta","thinking":"reason"}}`},
		{"initial text", `{"type":"content_block_start","content_block":{"type":"text","text":"hello"}}`},
		{"initial thinking", `{"type":"content_block_start","content_block":{"type":"thinking","thinking":"reason"}}`},
		{"tool", `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"call_1","name":"read","input":{}}}`},
	} {
		t.Run(content.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				out := httptest.NewRecorder()
				w := &responseWriter{ResponseWriter: out}
				start := time.Now()
				var sent strings.Builder
				write := func(s string) {
					t.Helper()
					sent.WriteString(s)
					if _, err := io.WriteString(w, s); err != nil {
						t.Fatal(err)
					}
				}
				write(": keepalive\n\ndata: {\"type\":\"ping\"}\n\n")
				if !w.timing.firstEvent.IsZero() {
					t.Fatal("heartbeat counted as an event")
				}
				time.Sleep(time.Second)
				write("data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":5}}}\n\n")
				time.Sleep(time.Second)
				for _, empty := range []string{
					`{"type":"content_block_start","content_block":{"type":"text","text":""}}`,
					`{"type":"content_block_delta","delta":{"type":"text_delta","text":""}}`,
					`{"type":"content_block_delta","delta":{"type":"signature_delta","signature":"signed"}}`,
					`{"type":"content_block_start","content_block":{"type":"tool_use","id":"call_1"}}`,
					`{"type":"content_block_delta","delta":{"type":"input_json_delta","partial_json":"{"}}`,
				} {
					write("data: " + empty + "\n\n")
				}
				if !w.timing.firstContent.IsZero() {
					t.Fatal("empty or unestablished content started the clock")
				}
				time.Sleep(time.Second)
				frame := "data: " + content.event + "\r\n\r\n"
				write(frame[:len(frame)-1])
				if !w.timing.firstContent.IsZero() {
					t.Fatal("incomplete SSE frame started the clock")
				}
				time.Sleep(time.Second)
				write(frame[len(frame)-1:])
				time.Sleep(10 * time.Second)
				write("data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":600,\"reasoning_tokens\":100}}\n\ndata: {\"type\":\"message_stop\"}\n\n")
				time.Sleep(2 * time.Second)
				if err := w.finish(); err != nil {
					t.Fatal(err)
				}
				for _, point := range []struct {
					name string
					at   time.Time
					want time.Duration
				}{
					{"event", w.timing.firstEvent, time.Second},
					{"content", w.timing.firstContent, 4 * time.Second},
					{"end", w.timing.messageEnd, 14 * time.Second},
				} {
					if got := point.at.Sub(start); got != point.want {
						t.Errorf("%s offset=%v want %v", point.name, got, point.want)
					}
				}
				if out.Body.String() != sent.String() || w.usage.outputTokens != 600 || w.usage.reasoningTokens != 100 {
					t.Fatal("observation changed wire bytes or usage")
				}
			})
		})
	}
}

func TestStreamTimingOnlyConfirmsSuccessfulWrites(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		out := &timingFailWriter{ResponseRecorder: httptest.NewRecorder()}
		w := &responseWriter{ResponseWriter: out}
		start := time.Now()
		_, _ = io.WriteString(w, "data: {\"type\":\"message_start\"}\n\n")
		if !w.timing.firstEvent.Equal(start) {
			t.Fatal("a measured zero offset was lost")
		}
		out.fail = true
		_, err := io.WriteString(w, "data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"hello\"}}\n\ndata: {\"type\":\"message_stop\"}\n\n")
		if err == nil || !w.timing.firstContent.IsZero() || !w.timing.messageEnd.IsZero() {
			t.Fatal("failed delivery confirmed a timing point")
		}
	})
}

type timingFailWriter struct {
	*httptest.ResponseRecorder
	fail  bool
	short bool
}

func (w *timingFailWriter) Write(p []byte) (int, error) {
	if w.fail {
		return 0, io.ErrClosedPipe
	}
	if w.short {
		return len(p) - 1, nil
	}
	return w.ResponseRecorder.Write(p)
}

func TestStreamTimingShortWriteDoesNotConfirmBatch(t *testing.T) {
	w := &responseWriter{ResponseWriter: &timingFailWriter{ResponseRecorder: httptest.NewRecorder(), short: true}}
	_, _ = io.WriteString(w, "data: {\"type\":\"message_start\"}\n\ndata: {\"type\":\"message_stop\"}\n\n")
	if !w.timing.firstEvent.IsZero() || !w.timing.messageEnd.IsZero() {
		t.Fatal("short write confirmed a complete batch")
	}
}

func TestStreamTimingBatchWaitsForOutputWrite(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		out := &timingDelayedWriter{ResponseRecorder: httptest.NewRecorder()}
		w := &responseWriter{ResponseWriter: out}
		start := time.Now()
		// Optional content metadata was never part of the forwarding validator.
		malformedContent := "data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":123}}\n\n"
		if _, err := io.WriteString(w, malformedContent); err != nil || out.Body.String() != malformedContent {
			t.Fatalf("timing metadata changed forwarding: %v", err)
		}
		if !w.timing.firstContent.IsZero() {
			t.Fatal("invalid optional content claimed a content start")
		}
		_, err := io.WriteString(w, "data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"ok\"}}\n\ndata: {\"type\":\"message_stop\"}\n\n")
		if err != nil {
			t.Fatal(err)
		}
		if w.timing.firstEvent.Sub(start) != time.Second || w.timing.firstContent.Sub(start) != 2*time.Second || !w.timing.firstContent.Equal(w.timing.messageEnd) {
			t.Fatal("batch timestamps did not follow completed output writes")
		}
	})
}

type timingDelayedWriter struct{ *httptest.ResponseRecorder }

func (w *timingDelayedWriter) Write(p []byte) (int, error) {
	time.Sleep(time.Second)
	return w.ResponseRecorder.Write(p)
}

func BenchmarkStreamObservation(b *testing.B) {
	frame := []byte("data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"a synthetic output fragment\"}}\n\n")
	body := []byte("data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":10}}}\n\n" +
		strings.Repeat(string(frame), 500) + "data: {\"type\":\"message_stop\"}\n\n")
	for _, size := range []struct {
		name string
		n    int
	}{{"event", len(frame)}, {"fragmented", 37}} {
		b.Run(size.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				w := &responseWriter{ResponseWriter: timingDiscardWriter{}}
				for start := 0; start < len(body); start += size.n {
					if _, err := w.Write(body[start:min(start+size.n, len(body))]); err != nil {
						b.Fatal(err)
					}
				}
				if err := w.finish(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

type timingDiscardWriter struct{}

func (timingDiscardWriter) Header() http.Header         { return make(http.Header) }
func (timingDiscardWriter) WriteHeader(int)             {}
func (timingDiscardWriter) Write(p []byte) (int, error) { return io.Discard.Write(p) }

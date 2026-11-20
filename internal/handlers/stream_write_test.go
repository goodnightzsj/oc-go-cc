package handlers

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/core"
)

type deadlineCheckWriter struct {
	*httptest.ResponseRecorder
	deadline        time.Time
	writeErr        error
	flushErr        error
	writes, flushes int
}

func (w *deadlineCheckWriter) SetWriteDeadline(deadline time.Time) error {
	w.deadline = deadline
	return nil
}

func (w *deadlineCheckWriter) Write(b []byte) (int, error) {
	w.writes++
	if w.deadline.IsZero() {
		return 0, errors.New("write without a deadline")
	}
	if w.writeErr != nil {
		return 0, w.writeErr
	}
	return w.ResponseRecorder.Write(b)
}

func (w *deadlineCheckWriter) FlushError() error {
	w.flushes++
	if w.deadline.IsZero() {
		return errors.New("flush without a deadline")
	}
	return w.flushErr
}

func TestStreamWriteDeadlineAcrossOutputAdapters(t *testing.T) {
	for _, kind := range []string{"messages", "responses", "native-responses"} {
		t.Run(kind, func(t *testing.T) {
			out := &deadlineCheckWriter{ResponseRecorder: httptest.NewRecorder()}
			var target http.ResponseWriter = out
			if kind != "messages" {
				target = &responsesHTTPWriter{ResponseWriter: out, headers: make(http.Header), streaming: true, native: kind == "native-responses"}
			}
			rw := &responseWriter{ResponseWriter: target}
			target.Header().Set("Content-Type", "text/event-stream")
			rw.WriteHeader(200)
			wire := nativeMessageTerminal
			if kind == "responses" {
				wire = "data: {\"type\":\"message_start\",\"message\":{\"id\":\"synthetic\",\"model\":\"synthetic\",\"usage\":{}}}\n\n" +
					"data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"}}\n\n" +
					"data: {\"type\":\"message_stop\"}\n\n"
			}
			if kind == "native-responses" {
				rw.setSourceFormat(core.WireFormatOpenAIResponses, true)
				wire = nativeResponseTerminal
			}
			if _, err := rw.Write([]byte(wire)); err != nil {
				t.Fatalf("write: %v", err)
			}
			rw.Flush()
			if err := rw.finish(); err != nil {
				t.Fatalf("finish: %v", err)
			}
			if out.writes == 0 || out.flushes == 0 || !out.deadline.IsZero() {
				t.Fatalf("deadline/flush lifecycle: writes=%d flushes=%d deadline=%v", out.writes, out.flushes, out.deadline)
			}
		})
	}
}

type stalledStreamWriter struct {
	*httptest.ResponseRecorder
	conn       net.Conn
	stallFlush bool
}

func (w *stalledStreamWriter) SetWriteDeadline(at time.Time) error {
	return w.conn.SetWriteDeadline(at)
}
func (w *stalledStreamWriter) Write(b []byte) (int, error) {
	if w.stallFlush {
		return w.ResponseRecorder.Write(b)
	}
	return w.conn.Write(b)
}
func (w *stalledStreamWriter) FlushError() error {
	if w.stallFlush {
		_, err := w.conn.Write([]byte("buffered data"))
		return err
	}
	return nil
}

func TestStreamWriteStalledClientCancelsAndFinishes(t *testing.T) {
	for _, flush := range []bool{false, true} {
		server, peer := net.Pipe()
		ctx, cancel := context.WithCancel(t.Context())
		out := &stalledStreamWriter{ResponseRecorder: httptest.NewRecorder(), conn: server, stallFlush: flush}
		rw := &responseWriter{ResponseWriter: out, writeTimeout: 20 * time.Millisecond, cancelWrites: cancel}
		finished := make(chan error, 1)
		go func() {
			_, err := rw.Write([]byte(nativeMessageTerminal))
			if err == nil {
				err = rw.FlushError()
			}
			finished <- err
		}()
		select {
		case err := <-finished:
			var timeout net.Error
			if !errors.As(err, &timeout) || !timeout.Timeout() || ctx.Err() == nil || rw.finish() == nil {
				t.Errorf("flush=%t: timeout/cancellation/result lost: %v", flush, err)
			}
		case <-time.After(time.Second):
			t.Error("stalled downstream exceeded write budget")
		}
		_ = server.Close()
		_ = peer.Close()
		cancel()
	}
}

type countingReadCloser struct {
	r     io.Reader
	reads int
}

func (c *countingReadCloser) Read(p []byte) (int, error) {
	c.reads++
	return c.r.Read(p)
}

func (c *countingReadCloser) Close() error { return nil }

func TestStreamWriteDirectCopyAndNativeError(t *testing.T) {
	out := &deadlineCheckWriter{ResponseRecorder: httptest.NewRecorder()}
	body := io.NopCloser(strings.NewReader(nativeMessageTerminal))
	if _, err := copyStreamingResponse(t.Context(), out, body, nil); err != nil {
		t.Fatalf("direct copy returned %v", err)
	}
	if out.Body.String() != nativeMessageTerminal || out.flushes != 1 || !out.deadline.IsZero() {
		t.Fatal("direct copy did not preserve bytes and deadline lifecycle")
	}
	failure := errors.New("synthetic flush failed")
	out.flushErr = failure
	rw := &responseWriter{ResponseWriter: out}
	rw.setSourceFormat(core.WireFormatOpenAIResponses, true)
	if err := rw.writeNativeError("synthetic upstream failure", nil); !errors.Is(err, failure) {
		t.Fatalf("native error delivery failure lost: %v", err)
	}
	if _, err := rw.Write([]byte("ignored")); !errors.Is(err, failure) {
		t.Fatalf("failed writer reused: %v", err)
	}
	// The direct copy stops reading immediately on failed delivery.
	out = &deadlineCheckWriter{ResponseRecorder: httptest.NewRecorder(), flushErr: io.ErrClosedPipe}
	counting := &countingReadCloser{r: strings.NewReader("firstsecond")}
	body = counting
	if _, err := copyStreamingResponse(t.Context(), out, body, nil); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("direct copy error = %v, want the write failure", err)
	}
	if counting.reads != 1 {
		t.Fatalf("read beyond downstream failure: %d", counting.reads)
	}
}

func TestStreamWriteTerminalFlushFailureIsNotSuccess(t *testing.T) {
	failure := errors.New("synthetic downstream flush failure")
	for _, native := range []bool{false, true} {
		out := &deadlineCheckWriter{ResponseRecorder: httptest.NewRecorder(), deadline: time.Now().Add(time.Minute), flushErr: failure}
		rw := &responseWriter{ResponseWriter: out}
		rw.setSourceFormat(core.WireFormatAnthropic, native)
		wire := nativeMessageTerminal
		if native {
			wire = nativeResponseTerminal
		}
		if _, err := rw.Write([]byte(wire)); err != nil {
			t.Fatal(err)
		}
		rw.Flush()
		if err := rw.finish(); !errors.Is(err, failure) {
			t.Fatalf("native=%t flush failure lost: %v", native, err)
		}
	}
}

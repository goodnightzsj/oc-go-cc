package handlers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/routatic/proxy/internal/core"
)

const nativeResponseTerminal = "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":5,\"output_tokens\":2}}}\n\n"
const nativeMessageTerminal = "data: {\"type\":\"message_delta\",\"usage\":{\"input_tokens\":5,\"output_tokens\":2}}\n\ndata: {\"type\":\"message_stop\"}\n\n"

type nativeTerminalBody struct {
	chunks        []string
	sameReadError bool
	reads         int
	closed        bool
}

func (b *nativeTerminalBody) Read(p []byte) (int, error) {
	b.reads++
	if len(b.chunks) == 0 {
		return 0, io.ErrUnexpectedEOF
	}
	n := copy(p, b.chunks[0])
	b.chunks[0] = b.chunks[0][n:]
	if b.chunks[0] == "" {
		b.chunks = b.chunks[1:]
	}
	if len(b.chunks) == 0 && b.sameReadError {
		return n, io.ErrUnexpectedEOF
	}
	return n, nil
}
func (b *nativeTerminalBody) Close() error { b.closed = true; return nil }

func TestNativeTerminalStopsBeforeTransportCleanup(t *testing.T) {
	for _, native := range []bool{false, true} {
		for _, sameRead := range []bool{false, true} {
			for _, split := range []bool{false, true} {
				for _, crlf := range []bool{false, true} {
					t.Run(fmt.Sprintf("responses=%t/same-read=%t/split=%t/crlf=%t", native, sameRead, split, crlf), func(t *testing.T) {
						wire := nativeMessageTerminal
						if native {
							wire = nativeResponseTerminal
						}
						if crlf {
							wire = strings.ReplaceAll(wire, "\n", "\r\n")
						}
						chunks := []string{wire}
						if split {
							chunks = []string{wire[:len(wire)-2], wire[len(wire)-2:]}
						}
						wantReads := len(chunks)
						body := &nativeTerminalBody{chunks: chunks, sameReadError: sameRead}
						out := httptest.NewRecorder()
						w := &responseWriter{ResponseWriter: out}
						w.setSourceFormat(core.WireFormatAnthropic, native)
						ctx, cancel := context.WithCancel(t.Context())
						err := NewStreamProxy().proxyAnthropicPassthroughStream(w, body, 0, ctx, cancel)
						if err != nil || w.finish() != nil {
							t.Fatalf("delivered terminal failed: %v", err)
						}
						if body.reads != wantReads || !body.closed || out.Body.String() != wire || w.usage.inputTokens != 5 || w.usage.outputTokens != 2 {
							t.Fatalf("lifecycle/bytes/usage changed: reads=%d closed=%t body=%q usage=%+v", body.reads, body.closed, out.Body.String(), w.usage)
						}
					})
				}
			}
		}
	}
}

func TestNativeTerminalDoesNotHideObservedFailure(t *testing.T) {
	for _, tc := range []struct {
		name, wire string
		native     bool
	}{
		{"missing-message-stop", `data: {"type":"message_start"}` + "\n\n", false},
		{"incomplete-message-stop", strings.TrimSuffix(nativeMessageTerminal, "\n"), false},
		{"message-error-same-batch", nativeMessageTerminal + "data: {\"type\":\"error\"}\n\n", false},
		{"invalid-message-same-batch", nativeMessageTerminal + "data: {bad}\n\n", false},
		{"partial-message-same-batch", nativeMessageTerminal + "data: {", false},
		{"incomplete-responses", strings.TrimSuffix(nativeResponseTerminal, "\n"), true},
		{"responses-error-same-batch", nativeResponseTerminal + "data: {\"type\":\"error\"}\n\n", true},
		{"invalid-responses-same-batch", nativeResponseTerminal + "data: {bad}\n\n", true},
		{"partial-responses-same-batch", nativeResponseTerminal + "data: {", true},
		{"conflicting-status", "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"failed\"}}\n\n", true},
		{"invalid-function", "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output\":[{\"type\":\"function_call\",\"arguments\":\"{\"}]}}\n\n", true},
		{"cancelled-response", "data: {\"type\":\"response.cancelled\"}\n\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := &responseWriter{ResponseWriter: httptest.NewRecorder()}
			w.setSourceFormat(core.WireFormatAnthropic, tc.native)
			body := &nativeTerminalBody{chunks: []string{tc.wire}}
			ctx, cancel := context.WithCancel(t.Context())
			err := NewStreamProxy().proxyAnthropicPassthroughStream(w, body, 0, ctx, cancel)
			if err == nil && w.finish() == nil {
				t.Fatal("observed failure became success")
			}
			if !body.closed {
				t.Fatal("body not closed")
			}
		})
	}
}

func TestNativeTerminalFinishesAlreadyStartedTrailer(t *testing.T) {
	for _, native := range []bool{false, true} {
		wire, trailer := nativeMessageTerminal, ": trailing comment\n\n"
		if native {
			wire, trailer = nativeResponseTerminal, "data: [DONE]\n\n"
		}
		body := &nativeTerminalBody{chunks: []string{wire + trailer[:8], trailer[8:]}}
		out := httptest.NewRecorder()
		w := &responseWriter{ResponseWriter: out}
		w.setSourceFormat(core.WireFormatAnthropic, native)
		ctx, cancel := context.WithCancel(t.Context())
		err := NewStreamProxy().proxyAnthropicPassthroughStream(w, body, 0, ctx, cancel)
		if err != nil || w.finish() != nil || body.reads != 2 || out.Body.String() != wire+trailer {
			t.Fatalf("fragmented trailer failed: responses=%t reads=%d err=%v finish=%v", native, body.reads, err, w.finish())
		}
	}
}

func TestNativeTerminalRequiresSuccessfulDelivery(t *testing.T) {
	for _, native := range []bool{false, true} {
		for _, kind := range []string{"write-error", "short-write", "cancelled"} {
			t.Run(fmt.Sprintf("responses=%t/%s", native, kind), func(t *testing.T) {
				out := &timingFailWriter{ResponseRecorder: httptest.NewRecorder(), fail: kind == "write-error", short: kind == "short-write"}
				w := &responseWriter{ResponseWriter: out}
				w.setSourceFormat(core.WireFormatAnthropic, native)
				wire := nativeMessageTerminal
				if native {
					wire = nativeResponseTerminal
				}
				body := &nativeTerminalBody{chunks: []string{wire}}
				ctx, cancel := context.WithCancel(t.Context())
				if kind == "cancelled" {
					cancel()
				}
				err := NewStreamProxy().proxyAnthropicPassthroughStream(w, body, 0, ctx, cancel)
				if err == nil || !body.closed || !w.timing.messageEnd.IsZero() {
					t.Fatalf("delivery failure lost: %v", err)
				}
				if kind == "short-write" && !errors.Is(err, io.ErrShortWrite) {
					t.Fatalf("short write: %v", err)
				}
			})
		}
	}
}

package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/routatic/proxy/internal/sse"
	"github.com/routatic/proxy/internal/transformer"
)

func TestResponsesAnthropicExtendedUsage(t *testing.T) {
	for _, tc := range []struct {
		name, usage string
		invalid     bool
	}{
		{"plain", `{"output_tokens":7}`, false},
		{"cache metadata", `{"output_tokens":7,"cache_creation":{"ephemeral_5m_input_tokens":3}}`, false},
		{"tier", `{"output_tokens":7,"service_tier":"standard"}`, false},
		{"server tools", `{"output_tokens":7,"server_tool_use":{"web_search_requests":1}}`, false},
		{"output details", `{"output_tokens":7,"output_tokens_details":{"reasoning_tokens":3}}`, false},
		{"invalid count", `{"output_tokens":"seven"}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wire := "data: {\"type\":\"message_start\",\"message\":{\"model\":\"claude-test\",\"usage\":{\"input_tokens\":11}}}\n\n" +
				"data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":" + tc.usage + "}\n\n" +
				"data: {\"type\":\"message_stop\"}\n\n"
			h, _ := newResponsesTestHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, wire)
			}))
			routeNativeCommandCode(h)
			w := httptest.NewRecorder()
			h.HandleResponses(w, httptest.NewRequest("POST", "/v1/responses", strings.NewReader(`{"model":"m","input":"hi","store":false,"stream":true}`)))
			failed := strings.Contains(w.Body.String(), "event: response.failed")
			completed := strings.Contains(w.Body.String(), "event: response.completed")
			if failed != tc.invalid || completed == tc.invalid {
				t.Fatalf("unexpected Responses terminal: %s", w.Body.String())
			}
			if !tc.invalid && (!strings.Contains(w.Body.String(), `"output_tokens":7`) || !strings.Contains(w.Body.String(), `"input_tokens":11`)) {
				t.Fatalf("usage was lost: %s", w.Body.String())
			}
		})
	}
}

func TestAnthropicUsageAfterSemanticError(t *testing.T) {
	out := httptest.NewRecorder()
	w := &responseWriter{ResponseWriter: out}
	for _, chunk := range []string{
		"data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":11}}}\n\n",
		"data: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\",\"message\":\"synthetic\"}}\n\n",
		"data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":7}}\n\ndata: {\"type\":\"message_stop\"}\n\n",
	} {
		if _, err := io.WriteString(w, chunk); err != nil {
			t.Fatal(err)
		}
	}
	if w.finish() == nil || w.usage.outputTokens != 7 || !w.sourceTerminalObserved() {
		t.Fatalf("source failure, usage or terminal lost: usage=%+v finish=%v", w.usage, w.finish())
	}
}

func TestAnthropicLocalErrorSeparatesPartialFrame(t *testing.T) {
	out := httptest.NewRecorder()
	w := &responseWriter{ResponseWriter: out}
	if _, err := io.WriteString(w, "data: {bad"); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(w, "}\n\n"); err == nil {
		t.Fatal("fixture must fail parsing")
	}
	h := &MessagesHandler{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	h.sendStreamError(w, "synthetic terminal failure", nil)
	var decoder sse.Decoder
	validErrors := 0
	err := decoder.Feed(out.Body.Bytes(), func(name string, data []byte) error {
		var e struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(data, &e) == nil && name == "error" && e.Type == "error" {
			validErrors++
		}
		return nil
	})
	if err != nil || validErrors != 1 || w.finish() == nil {
		t.Fatalf("missing separate terminal failure: count=%d err=%v body=%q", validErrors, err, out.Body.String())
	}
}

func TestLocalStreamErrorHonorsShortWrite(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	w := &responseWriter{ResponseWriter: &timingFailWriter{ResponseRecorder: httptest.NewRecorder(), short: true}, cancelWrites: cancel}
	h := &MessagesHandler{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	h.sendStreamError(w, "synthetic terminal failure", nil)
	if !errors.Is(w.finish(), io.ErrShortWrite) || ctx.Err() == nil {
		t.Fatalf("short error delivery was accepted: finish=%v context=%v", w.finish(), ctx.Err())
	}
}

func TestAnthropicSharedSSEFraming(t *testing.T) {
	wire := "data: {\"type\":\"message_start\",\"message\":{\"model\":\"actual\",\"usage\":{\"input_tokens\":11}}}\n\n" +
		": keepalive\n\n" +
		"event: message_delta\ndata:{\ndata: \"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":7}}\n\n" +
		"data: {\"type\":\"message_stop\"}\n\n"
	for _, ending := range []string{"\n", "\r\n", "\r"} {
		for _, prefix := range []string{"", "\xef\xbb\xbf"} {
			for _, adapt := range []bool{false, true} {
				t.Run(fmt.Sprintf("ending=%q/bom=%t/responses=%t", ending, prefix != "", adapt), func(t *testing.T) {
					body := prefix + strings.ReplaceAll(wire, "\n", ending)
					out := httptest.NewRecorder()
					rw := &responseWriter{ResponseWriter: out}
					var writer io.Writer = rw
					finish := rw.finish
					if adapt {
						adapter := transformer.NewResponsesStreamWriter(out, "requested")
						writer, finish = adapter, adapter.Finish
					}
					for _, b := range []byte(body) {
						if _, err := writer.Write([]byte{b}); err != nil {
							t.Fatal(err)
						}
					}
					if err := finish(); err != nil {
						t.Fatal(err)
					}
					if adapt {
						if !strings.Contains(out.Body.String(), "event: response.completed") || !strings.Contains(out.Body.String(), ": keepalive\n\n") || !strings.Contains(out.Body.String(), `"input_tokens":11`) || !strings.Contains(out.Body.String(), `"output_tokens":7`) {
							t.Fatalf("lost terminal, heartbeat or usage: %s", out.Body.String())
						}
					} else if out.Body.String() != body || rw.usage.inputTokens != 11 || rw.usage.outputTokens != 7 || !rw.sourceTerminalObserved() {
						t.Fatalf("native bytes or usage changed: body=%q usage=%+v", out.Body.String(), rw.usage)
					}
				})
			}
		}
	}
}

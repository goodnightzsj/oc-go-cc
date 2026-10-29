package transformer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/routatic/proxy/pkg/types"
)

func TestChatRefusalContentAndTerminal(t *testing.T) {
	for _, tc := range []struct {
		name, message, text string
		refusal             bool
	}{
		{"refusal-only", `{"content":null,"refusal":"declined"}`, "declined", true},
		{"both", `{"content":"prefix:","refusal":"declined"}`, "prefix:declined", true},
		{"equal-fields", `{"content":"X","refusal":"X"}`, "XX", true},
		{"empty", `{"content":"ok","refusal":""}`, "ok", false},
		{"null", `{"content":"ok","refusal":null}`, "ok", false},
		{"absent", `{"content":"ok"}`, "ok", false},
	} {
		for _, finish := range []string{"stop", "length", "tool_calls", "content_filter", "unknown", ""} {
			for _, usage := range []string{"null", `{"prompt_tokens":0,"completion_tokens":0}`, `{"prompt_tokens":5,"completion_tokens":2}`} {
				for _, stream := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/%s/%s/stream=%t", tc.name, finish, usage, stream), func(t *testing.T) {
						wantStop := map[string]string{"stop": "end_turn", "length": "max_tokens", "tool_calls": "tool_use", "content_filter": "refusal"}[finish]
						if finish == "stop" && tc.refusal {
							wantStop = "refusal"
						}
						var text, stop string
						var gotUsage *types.Usage
						var err error
						if stream {
							w := newMockResponseWriter()
							body := sseLines(fmt.Sprintf(`{"choices":[{"delta":%s,"finish_reason":%q}],"usage":%s}`, tc.message, finish, usage), "[DONE]")
							err = NewStreamHandler().ProxyStream(w, body, "synthetic", context.Background(), 0, func() {})
							for _, event := range parseSSEEvents(t, w.buf.String()) {
								if event.Delta != nil {
									if event.Delta.Type == "text_delta" {
										text += event.Delta.Text
									}
									if event.Delta.StopReason != "" {
										stop = event.Delta.StopReason
									}
								}
								if event.Type == "message_delta" {
									gotUsage = event.Usage
								}
							}
						} else {
							var input types.ChatCompletionResponse
							if err := json.Unmarshal([]byte(fmt.Sprintf(`{"choices":[{"message":%s,"finish_reason":%q}],"usage":%s}`, tc.message, finish, usage)), &input); err != nil {
								t.Fatal(err)
							}
							var output *types.MessageResponse
							output, err = NewResponseTransformer().TransformResponse(&input, "synthetic")
							if output != nil {
								stop, gotUsage = output.StopReason, output.Usage
								for _, block := range output.Content {
									if block.Type == "text" {
										text += block.Text
									}
								}
							}
						}
						if wantStop == "" {
							if err == nil {
								t.Fatal("missing/unknown terminal accepted")
							}
							return
						}
						if err != nil || text != tc.text || stop != wantStop {
							t.Fatalf("text=%q stop=%s err=%v; want %q/%s", text, stop, err, tc.text, wantStop)
						}
						if (gotUsage == nil) != (usage == "null") {
							t.Fatalf("usage presence changed: %+v", gotUsage)
						}
						if gotUsage != nil {
							want := 0
							if strings.Contains(usage, "5") {
								want = 2
							}
							if gotUsage.OutputTokens != want {
								t.Fatalf("usage changed: %+v", gotUsage)
							}
						}
					})
				}
			}
		}
	}
}

func TestChatRefusalStreamingOrderAndFailures(t *testing.T) {
	const first = `{"choices":[{"delta":{"refusal":"R1"}}]}`
	const content = `{"choices":[{"delta":{"content":"C"}}]}`
	const last = `{"choices":[{"delta":{"refusal":"R2"},"finish_reason":"stop"}]}`
	w := newMockResponseWriter()
	err := NewStreamHandler().ProxyStream(w, sseLines(first, content, last, "[DONE]"), "synthetic", t.Context(), 0, func() {})
	var text string
	for _, event := range parseSSEEvents(t, w.buf.String()) {
		if event.Delta != nil && event.Delta.Type == "text_delta" {
			text += event.Delta.Text
		}
	}
	if err != nil || text != "R1CR2" {
		t.Fatalf("refusal order lost: %q %v", text, err)
	}
	for _, tail := range []string{
		"", `{"error":{"message":"source failure"}}`, `{bad}`, `{"choices":[{"delta":{},"finish_reason":"stop"}]}` + "\n\ndata: " + first,
	} {
		w := newMockResponseWriter()
		body := sseLines(first, tail)
		err := NewStreamHandler().ProxyStream(w, body, "synthetic", t.Context(), 0, func() {})
		if err == nil || strings.Contains(w.buf.String(), `"type":"message_stop"`) {
			t.Fatalf("refusal hid invalid tail %q: %v", tail, err)
		}
	}
	w = newMockResponseWriter()
	err = NewStreamHandler().ProxyStream(w, io.NopCloser(io.MultiReader(sseLines(first), iotest.ErrReader(io.ErrUnexpectedEOF))), "synthetic", t.Context(), 0, func() {})
	if err == nil {
		t.Fatal("refusal hid read error")
	}
}

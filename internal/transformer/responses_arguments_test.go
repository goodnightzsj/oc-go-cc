package transformer

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/pkg/types"
)

type responsesArgumentFailureWriter struct{ *mockResponseWriter }

func (w responsesArgumentFailureWriter) Write([]byte) (int, error) {
	return 0, io.ErrClosedPipe
}

func TestResponsesToolArgumentBoundary(t *testing.T) {
	payload := `{"v":"` + strings.Repeat("x", (1<<20)-8) + `"}`
	w := newMockResponseWriter()
	var tool responsesToolState
	for i := 0; i < len(payload); i += 1024 {
		if err := tool.appendArguments(w, payload[i:min(i+1024, len(payload))]); err != nil {
			t.Fatal(err)
		}
	}
	before := w.buf.Len()
	if err := tool.appendArguments(w, "x"); err == nil || tool.arguments.String() != payload || w.buf.Len() != before {
		t.Fatal("oversized append changed arguments or emitted output")
	}
	var delivered strings.Builder
	for _, event := range parseSSEEvents(t, w.buf.String()) {
		delivered.WriteString(event.Delta.PartialJSON)
	}
	if delivered.String() != payload {
		t.Fatal("tool argument bytes changed")
	}
	if err := tool.appendArguments(responsesArgumentFailureWriter{w}, ""); err != nil {
		t.Fatal("empty delta wrote output")
	}
	var failed responsesToolState
	if err := failed.appendArguments(responsesArgumentFailureWriter{w}, "{}"); !errors.Is(err, ErrClientDisconnected) || failed.arguments.Len() != 0 {
		t.Fatal("failed output changed accumulated arguments")
	}
}

func TestResponsesToolArgumentCompletion(t *testing.T) {
	for _, tc := range []struct {
		name, arguments, snapshot, terminal string
		failed                              bool
	}{
		{"complete", `{}`, `{}`, `{"type":"response.completed","response":{"status":"completed"}}`, false},
		{"snapshot-extension", `{`, `{}`, `{"type":"response.completed","response":{"status":"completed"}}`, false},
		{"changed-snapshot", `{}`, `[]`, `{"type":"response.completed","response":{"status":"completed"}}`, true},
		{"budget", `{"q":`, "", `{"type":"response.incomplete","response":{"status":"incomplete","incomplete_details":{"reason":"max_output_tokens"}}}`, false},
		{"invalid-complete", `{"q":`, "", `{"type":"response.completed","response":{"status":"completed"}}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			delta, _ := json.Marshal(map[string]any{"type": "response.function_call_arguments.delta", "item_id": "f", "delta": tc.arguments})
			frames := []string{`{"type":"response.output_item.added","item":{"type":"function_call","id":"f","call_id":"call","name":"lookup"}}`, string(delta)}
			if tc.snapshot != "" {
				snapshot, _ := json.Marshal(map[string]any{"type": "response.output_item.done", "item": map[string]any{"type": "function_call", "id": "f", "call_id": "call", "name": "lookup", "arguments": tc.snapshot}})
				frames = append(frames, string(snapshot))
			}
			frames = append(frames, tc.terminal)
			w := newMockResponseWriter()
			err := NewStreamHandler().ProxyResponsesStream(w, sseLines(frames...), "synthetic", t.Context(), 0, func() {})
			if (err != nil) != tc.failed || strings.Contains(w.buf.String(), `"type":"message_stop"`) == tc.failed {
				t.Fatalf("completion changed: err=%v failed=%t", err, tc.failed)
			}
			if !tc.failed {
				var delivered strings.Builder
				for _, event := range parseSSEEvents(t, w.buf.String()) {
					if event.Delta != nil {
						delivered.WriteString(event.Delta.PartialJSON)
					}
				}
				want := tc.arguments
				if tc.snapshot != "" {
					want = tc.snapshot
				}
				if delivered.String() != want {
					t.Fatal("completion duplicated or changed arguments")
				}
			}
		})
	}
}

func TestResponsesToolArgumentsAtLimitComplete(t *testing.T) {
	payload := `{"v":"` + strings.Repeat("x", (1<<20)-8) + `"}`
	frames := []string{`{"type":"response.output_item.added","item":{"type":"function_call","id":"f","call_id":"call","name":"lookup"}}`}
	for i := 0; i < len(payload); i += 1024 {
		delta, err := json.Marshal(map[string]any{"type": "response.function_call_arguments.delta", "item_id": "f", "delta": payload[i:min(i+1024, len(payload))]})
		if err != nil {
			t.Fatal(err)
		}
		frames = append(frames, string(delta))
	}
	// The per-event size limit is separate; final snapshots are covered above.
	frames = append(frames, `{"type":"response.completed","response":{"status":"completed"}}`)
	w := newMockResponseWriter()
	if err := NewStreamHandler().ProxyResponsesStream(w, sseLines(frames...), "synthetic", t.Context(), 0, func() {}); err != nil {
		t.Fatal(err)
	}
	var delivered strings.Builder
	stops := 0
	for _, event := range parseSSEEvents(t, w.buf.String()) {
		if event.Delta != nil {
			delivered.WriteString(event.Delta.PartialJSON)
		}
		if event.Type == "message_stop" {
			stops++
		}
	}
	if delivered.String() != payload || stops != 1 {
		t.Fatal("1 MiB argument stream did not complete exactly once")
	}
}

func TestResponsesTerminalToolSnapshots(t *testing.T) {
	for _, tc := range []struct {
		name, priorType, priorArgs, arguments, callID, function string
		budget, failed                                          bool
	}{
		{name: "terminal-only", arguments: `{}`},
		{name: "prefix-extension", priorType: "added", priorArgs: `{`, arguments: `{}`},
		{name: "closed-consistent", priorType: "done", priorArgs: `{}`, arguments: `{}`},
		{name: "closed-changed", priorType: "done", priorArgs: `{}`, arguments: `{"q":1}`, failed: true},
		{name: "closed-name-changed", priorType: "done", priorArgs: `{}`, arguments: `{}`, function: "other", failed: true},
		{name: "closed-call-changed", priorType: "done", priorArgs: `{}`, arguments: `{}`, callID: "other", failed: true},
		{name: "open-prefix-changed", priorType: "added", priorArgs: `{"q":`, arguments: `{}`, failed: true},
		{name: "budget-terminal-only", arguments: `{"q":`, budget: true},
		{name: "budget-prefix-extension", priorType: "added", priorArgs: `{"q":`, arguments: `{"q":1`, budget: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			item := types.ResponsesOutput{Type: "function_call", ID: "f", CallID: "call", Name: "lookup", Arguments: tc.priorArgs}
			var frames []string
			encode := func(v any) string {
				b, err := json.Marshal(v)
				if err != nil {
					t.Fatal(err)
				}
				return string(b)
			}
			if tc.priorType != "" {
				frames = append(frames, encode(types.ResponsesChunk{Type: "response.output_item." + tc.priorType, Item: &item}))
			}
			item.Arguments = tc.arguments
			if tc.callID != "" {
				item.CallID = tc.callID
			}
			if tc.function != "" {
				item.Name = tc.function
			}
			terminal := types.ResponsesChunk{Type: "response.completed", Response: &types.ResponsesResponse{Status: "completed", Output: []types.ResponsesOutput{item}}}
			if tc.budget {
				terminal.Type, terminal.Response.Status = "response.incomplete", "incomplete"
				terminal.Response.IncompleteDetails = &types.ResponsesIncompleteDetails{Reason: "max_output_tokens"}
			}
			frames = append(frames, encode(terminal))
			wire, err := io.ReadAll(sseLines(frames...))
			if err != nil {
				t.Fatal(err)
			}
			observation := StreamTermination{Format: core.WireFormatOpenAIResponses}
			sourceErr := observation.Feed(wire)
			if sourceErr == nil {
				sourceErr = observation.Finish()
			}
			if (sourceErr != nil) != tc.failed {
				t.Fatalf("source snapshot validation differs: err=%v failed=%t", sourceErr, tc.failed)
			}
			out := newMockResponseWriter()
			err = NewStreamHandler().ProxyResponsesStream(out, sseLines(frames...), "synthetic", t.Context(), 0, func() {})
			if tc.failed {
				if err == nil || strings.Contains(out.buf.String(), `"type":"message_stop"`) {
					t.Fatalf("contradictory snapshot completed: err=%v output=%s", err, out.buf.String())
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var arguments strings.Builder
			starts, stops, messages := 0, 0, 0
			reason := ""
			for _, event := range parseSSEEvents(t, out.buf.String()) {
				if event.ContentBlock != nil && event.ContentBlock.Type == "tool_use" {
					starts++
				}
				if event.Type == "content_block_stop" {
					stops++
				}
				if event.Type == "message_stop" {
					messages++
				}
				if event.Delta != nil {
					arguments.WriteString(event.Delta.PartialJSON)
					if event.Delta.StopReason != "" {
						reason = event.Delta.StopReason
					}
				}
			}
			wantReason := "tool_use"
			if tc.budget {
				wantReason = "max_tokens"
			}
			if starts != 1 || stops != 1 || messages != 1 || arguments.String() != tc.arguments || reason != wantReason {
				t.Fatalf("snapshot lost/duplicated: starts=%d stops=%d messages=%d args=%q reason=%q", starts, stops, messages, arguments.String(), reason)
			}
		})
	}
}

type responsesToolFailureWriter struct {
	*mockResponseWriter
	event string
}

func (w *responsesToolFailureWriter) Write(p []byte) (int, error) {
	if strings.Contains(string(p), `"type":"`+w.event+`"`) {
		return 0, io.ErrClosedPipe
	}
	return w.mockResponseWriter.Write(p)
}

func TestResponsesToolSnapshotWriteFailure(t *testing.T) {
	for _, event := range []string{"content_block_start", "content_block_delta", "content_block_stop"} {
		t.Run(event, func(t *testing.T) {
			state := responsesStreamState{tools: make(map[string]*responsesToolState)}
			out := &responsesToolFailureWriter{mockResponseWriter: newMockResponseWriter(), event: event}
			item := types.ResponsesOutput{Type: "function_call", ID: "f", CallID: "call", Name: "lookup", Arguments: `{}`}
			if err := state.functionItem(out, item, true); !errors.Is(err, ErrClientDisconnected) {
				t.Fatalf("delivery failure lost: %v", err)
			}
			tool := state.tools[item.ID]
			if event == "content_block_start" {
				if tool != nil || state.nextIndex != 0 {
					t.Fatal("failed start committed tool state")
				}
				return
			}
			if tool == nil || tool.closed || (event == "content_block_delta" && tool.arguments.Len() != 0) {
				t.Fatal("failed write committed arguments or closed state")
			}
		})
	}
}

type responsesArgumentDiscardWriter struct{ writes int }

func (w *responsesArgumentDiscardWriter) Header() http.Header { return nil }
func (w *responsesArgumentDiscardWriter) WriteHeader(int)     {}
func (w *responsesArgumentDiscardWriter) Write(p []byte) (int, error) {
	w.writes++
	return len(p), nil
}

func BenchmarkResponsesToolArguments(b *testing.B) {
	part := strings.Repeat("x", 1024)
	b.ReportAllocs()
	for b.Loop() {
		var tool responsesToolState
		var out responsesArgumentDiscardWriter
		for range 1024 {
			if err := tool.appendArguments(&out, part); err != nil {
				b.Fatal(err)
			}
		}
		if out.writes != 1024 {
			b.Fatal("benchmark omitted SSE output writes")
		}
	}
}

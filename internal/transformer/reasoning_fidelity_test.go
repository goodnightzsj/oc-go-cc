package transformer

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/pkg/types"
)

func TestChatReasoningAliasesInJSONAndStream(t *testing.T) {
	for _, tc := range []struct{ fields, want string }{
		{`"reasoning":"alias"`, "alias"},
		{`"reasoning_content":"canonical"`, "canonical"},
		{`"reasoning_content":"same","reasoning":"same"`, "same"},
		{`"reasoning_content":"canonical","reasoning":"alias"`, "canonical"},
		{`"reasoning_content":"","reasoning":"alias"`, "alias"},
	} {
		var upstream types.ChatCompletionResponse
		if err := json.Unmarshal([]byte(`{"choices":[{"message":{`+tc.fields+`},"finish_reason":"stop"}]}`), &upstream); err != nil {
			t.Fatal(err)
		}
		response, err := NewResponseTransformer().TransformResponse(&upstream, "synthetic")
		if err != nil || len(response.Content) != 1 || response.Content[0].Thinking != tc.want {
			t.Fatalf("JSON reasoning lost or duplicated: %+v %v", response, err)
		}
		w := newMockResponseWriter()
		err = NewStreamHandler().ProxyStream(w, sseLines(`{"choices":[{"delta":{`+tc.fields+`}}]}`, `{"choices":[{"delta":{},"finish_reason":"stop"}]}`), "synthetic", t.Context(), 0, func() {})
		if err != nil {
			t.Fatal(err)
		}
		var thought strings.Builder
		for _, event := range parseSSEEvents(t, w.buf.String()) {
			if event.Delta != nil {
				thought.WriteString(event.Delta.Thinking)
			}
		}
		if thought.String() != tc.want {
			t.Fatalf("stream reasoning=%q want=%q", thought.String(), tc.want)
		}
	}
}

func TestResponsesReasoningSummaryOrderAndDeduplication(t *testing.T) {
	for _, source := range [][]string{
		{
			`{"type":"response.output_item.added","item":{"type":"reasoning","id":"r","summary":[]}}`,
			`{"type":"response.reasoning_summary_part.added","item_id":"r","summary_index":0,"part":{"type":"summary_text","text":""}}`,
			`{"type":"response.reasoning_summary_text.delta","item_id":"r","summary_index":0,"delta":"thi"}`,
			`{"type":"response.reasoning_summary_text.delta","item_id":"r","summary_index":0,"delta":"nk"}`,
			`{"type":"response.reasoning_summary_text.done","item_id":"r","summary_index":0,"text":"think"}`,
			`{"type":"response.reasoning_summary_part.done","item_id":"r","summary_index":0,"part":{"type":"summary_text","text":"think"}}`,
			`{"type":"response.output_item.done","item":{"type":"reasoning","id":"r","summary":[{"type":"summary_text","text":"think"}]}}`,
		},
		{`{"type":"response.output_item.done","item":{"type":"reasoning","id":"r","summary":[{"type":"summary_text","text":"think"}]}}`},
	} {
		w := newMockResponseWriter()
		frames := append(append([]string{}, source...), `{"type":"response.output_text.delta","item_id":"m","delta":"answer"}`, `{"type":"response.completed","response":{"status":"completed","output":[{"type":"reasoning","id":"r","summary":[{"type":"summary_text","text":"think"}]}]}}`)
		if err := NewStreamHandler().ProxyResponsesStream(w, sseLines(frames...), "synthetic", t.Context(), 0, func() {}); err != nil {
			t.Fatal(err)
		}
		var blocks []string
		for _, event := range parseSSEEvents(t, w.buf.String()) {
			if event.Type == "content_block_start" {
				blocks = append(blocks, event.ContentBlock.Type+":")
			}
			if event.Delta != nil && event.Index != nil {
				blocks[*event.Index] += event.Delta.Thinking + event.Delta.Text
			}
		}
		if strings.Join(blocks, "|") != "thinking:think|text:answer" {
			t.Fatalf("summary order or duplication: %v", blocks)
		}
	}
	var response types.ResponsesResponse
	if err := json.Unmarshal([]byte(`{"status":"completed","output":[{"type":"reasoning","summary":[{"type":"summary_text","text":"think"},{"type":"summary_text","text":"more"}]},{"type":"message","role":"assistant","content":[{"type":"output_text","text":"answer"}]}]}`), &response); err != nil {
		t.Fatal(err)
	}
	nr, err := ResponsesToNormalized(&response, "synthetic")
	if err != nil {
		t.Fatal(err)
	}
	content := core.DenormalizeResponse(nr).Content
	if len(content) != 3 || content[0].Thinking != "think" || content[1].Thinking != "more" || content[2].Text != "answer" {
		t.Fatalf("JSON summary order lost: %+v", content)
	}
}

func TestResponsesChangedSummaryCannotBecomeSuccessful(t *testing.T) {
	w := newMockResponseWriter()
	err := NewStreamHandler().ProxyResponsesStream(w, sseLines(
		`{"type":"response.reasoning_summary_text.delta","item_id":"r","delta":"before"}`,
		`{"type":"response.reasoning_summary_text.done","item_id":"r","text":"after"}`,
		`{"type":"response.completed"}`,
	), "synthetic", t.Context(), 0, func() {})
	if err == nil || strings.Contains(w.buf.String(), `"type":"message_stop"`) {
		t.Fatalf("changed summary reported success: %v %s", err, w.buf.String())
	}
}

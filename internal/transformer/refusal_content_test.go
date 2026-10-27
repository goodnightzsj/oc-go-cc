package transformer

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/pkg/types"
)

func TestResponsesRefusalContentSurvivesJSONAndStream(t *testing.T) {
	const response = `{"status":"completed","output":[{"type":"message","id":"m","role":"assistant","content":[{"type":"refusal","refusal":"declined"}]}]}`
	var upstream types.ResponsesResponse
	if err := json.Unmarshal([]byte(response), &upstream); err != nil {
		t.Fatal(err)
	}
	nr, err := ResponsesToNormalized(&upstream, "synthetic")
	if err != nil {
		t.Fatal(err)
	}
	message := core.DenormalizeResponse(nr)
	if message.StopReason != "refusal" || len(message.Content) != 1 || message.Content[0].Text != "declined" {
		t.Fatalf("refusal lost: %+v", message)
	}
	for _, incremental := range []bool{false, true} {
		var frames []string
		if incremental {
			frames = []string{
				`{"type":"response.content_part.added","item_id":"m","part":{"type":"refusal","refusal":""}}`,
				`{"type":"response.refusal.delta","item_id":"m","delta":"decl"}`,
				`{"type":"response.refusal.delta","item_id":"m","delta":"ined"}`,
				`{"type":"response.refusal.done","item_id":"m","refusal":"declined"}`,
				`{"type":"response.content_part.done","item_id":"m","part":{"type":"refusal","refusal":"declined"}}`,
			}
		}
		frames = append(frames, `{"type":"response.output_item.done","item":{"type":"message","id":"m","role":"assistant","content":[{"type":"refusal","refusal":"declined"}]}}`, `{"type":"response.completed","response":`+response+`}`)
		w := newMockResponseWriter()
		if err := NewStreamHandler().ProxyResponsesStream(w, sseLines(frames...), "synthetic", t.Context(), 0, func() {}); err != nil {
			t.Fatal(err)
		}
		var text strings.Builder
		stop := ""
		for _, event := range parseSSEEvents(t, w.buf.String()) {
			if event.Delta != nil {
				text.WriteString(event.Delta.Text)
				if event.Delta.StopReason != "" {
					stop = event.Delta.StopReason
				}
			}
		}
		if text.String() != "declined" || stop != "refusal" {
			t.Fatalf("refusal duplicated/lost: %s stop=%s", text.String(), stop)
		}
	}
}

func TestResponsesToolArgumentsBoundAcrossEvents(t *testing.T) {
	chunk, _ := json.Marshal(map[string]any{"type": "response.function_call_arguments.delta", "item_id": "f", "delta": strings.Repeat("x", 600000)})
	w := newMockResponseWriter()
	err := NewStreamHandler().ProxyResponsesStream(w, sseLines(`{"type":"response.output_item.added","item":{"type":"function_call","id":"f","call_id":"a","name":"lookup"}}`, string(chunk), string(chunk), `{"type":"response.completed"}`), "synthetic", t.Context(), 0, func() {})
	if err == nil || !strings.Contains(err.Error(), "exceed 1 MiB") || strings.Contains(w.buf.String(), `"type":"message_stop"`) {
		t.Fatalf("cross-event arguments unbounded: %v", err)
	}
}

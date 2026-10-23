package transformer

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/routatic/proxy/pkg/types"
)

func TestResponsesPreservesKnownReasoning(t *testing.T) {
	for _, reasoning := range []int{0, 26} {
		usage := types.Usage{InputTokens: 20, OutputTokens: 36, ReasoningTokens: reasoning}
		msg := types.MessageResponse{ID: "test", Type: "message", Role: "assistant", Model: "m", StopReason: "end_turn", Usage: usage}
		for _, mode := range []string{"nonstream", "message_start", "message_delta", "interrupted"} {
			t.Run(mode, func(t *testing.T) {
				var raw []byte
				if mode == "nonstream" {
					body, err := json.Marshal(msg)
					if err != nil {
						t.Fatal(err)
					}
					response, err := AnthropicMessageToResponse(body, "m")
					if err != nil {
						t.Fatal(err)
					}
					raw, err = json.Marshal(response)
					if err != nil {
						t.Fatal(err)
					}
				} else {
					var out bytes.Buffer
					w := NewResponsesStreamWriter(&out, "m")
					startUsage, deltaUsage := &usage, (*types.Usage)(nil)
					if mode != "message_start" {
						startUsage, deltaUsage = nil, &usage
					}
					start := map[string]any{"type": "message_start", "message": map[string]any{"model": "m", "usage": startUsage}}
					delta := map[string]any{"type": "message_delta", "delta": map[string]string{"stop_reason": "end_turn"}, "usage": deltaUsage}
					events := []map[string]any{start, delta}
					if mode != "interrupted" {
						events = append(events, map[string]any{"type": "message_stop"})
					}
					for _, event := range events {
						data, err := json.Marshal(event)
						if err != nil {
							t.Fatal(err)
						}
						if _, err := w.Write(append(append([]byte("data: "), data...), '\n', '\n')); err != nil {
							t.Fatal(err)
						}
					}
					if err := w.Finish(); (err != nil) != (mode == "interrupted") {
						t.Fatalf("Finish = %v", err)
					}
					kind := "response.completed"
					if mode == "interrupted" {
						kind = "response.failed"
					}
					for _, line := range strings.Split(out.String(), "\n") {
						if !strings.HasPrefix(line, "data: ") {
							continue
						}
						var event struct {
							Type     string
							Response json.RawMessage
						}
						if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err != nil {
							t.Fatal(err)
						}
						if event.Type == kind {
							raw = event.Response
						}
					}
				}
				var response struct {
					Usage struct {
						Input   int            `json:"input_tokens"`
						Output  int            `json:"output_tokens"`
						Total   int            `json:"total_tokens"`
						Details map[string]int `json:"output_tokens_details"`
					}
				}
				if err := json.Unmarshal(raw, &response); err != nil {
					t.Fatal(err)
				}
				got := response.Usage
				if got.Input != 20 || got.Output != 36 || got.Total != 56 || got.Details["reasoning_tokens"] != reasoning || (reasoning == 0 && got.Details != nil) {
					t.Fatalf("reasoning=%d: usage=%+v", reasoning, got)
				}
			})
		}
	}
}

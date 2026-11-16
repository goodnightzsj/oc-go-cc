package transformer

import (
	"fmt"
	"testing"

	"github.com/routatic/proxy/internal/core"
)

func TestNativeSourceValidatesConsumedFields(t *testing.T) {
	for _, field := range []string{
		`"usage":{"output_tokens":"oops"}`,
		`"usage":{"cache_read_input_tokens":[]}`,
		`"usage":{"cache_creation_input_tokens":{}}`,
		`"usage":{"reasoning_tokens":1.5}`,
		`"message":{"usage":{"input_tokens":"oops"}}`,
		`"message":{"model":42}`,
		`"delta":{"stop_reason":[]}`,
	} {
		t.Run(field, func(t *testing.T) {
			s := StreamTermination{Format: core.WireFormatAnthropic}
			body := fmt.Sprintf("data: {\"type\":\"message_delta\",%s}\n\ndata: {\"type\":\"message_stop\"}\n\n", field)
			if err := s.Feed([]byte(body)); err == nil {
				t.Fatal("invalid consumer field was accepted as healthy")
			}
			if err := s.Finish(); err == nil {
				t.Fatal("terminal erased source failure")
			}
		})
	}
	s := StreamTermination{Format: core.WireFormatAnthropic}
	if err := s.Feed([]byte("data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":0,\"service_tier\":\"standard\"},\"opaque\":{\"signed\":true}}}\n\ndata: {\"type\":\"message_stop\"}\n\n")); err != nil {
		t.Fatal(err)
	}
	if err := s.Finish(); err != nil {
		t.Fatal(err)
	}
}

func TestStreamTerminationContracts(t *testing.T) {
	for _, tc := range []struct {
		name              string
		format            core.WireFormat
		body              string
		requireDone, fail bool
	}{
		{"chat-no-done", core.WireFormatOpenAIChat, "data: {\"choices\":[{\"finish_reason\":\"length\"}]}\n\n", false, false},
		{"cline-no-done", core.WireFormatOpenAIChat, "data: {\"choices\":[{\"finish_reason\":\"stop\"}]}\n\n", true, true},
		{"half-frame", core.WireFormatAnthropic, "data: {\"type\":\"message_stop\"}\n", false, true},
		{"terminal-half-tail", core.WireFormatAnthropic, "data: {\"type\":\"message_stop\"}\n\ndata: {", false, true},
		{"empty", core.WireFormatAnthropic, "", false, true},
		{"anthropic-late-content", core.WireFormatAnthropic, "data: {\"type\":\"message_stop\"}\n\ndata: {\"type\":\"content_block_delta\"}\n\n", false, true},
		{"anthropic-late-event-name", core.WireFormatAnthropic, "data: {\"type\":\"message_stop\"}\n\nevent: content_block_start\ndata: {}\n\n", false, true},
		{"anthropic-late-usage", core.WireFormatAnthropic, "data: {\"type\":\"message_stop\"}\n\ndata: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":3}}\n\n", false, false},
		{"anthropic-late-extension", core.WireFormatAnthropic, "data: {\"type\":\"message_stop\"}\n\ndata: {\"type\":\"ping\"}\n\ndata: {\"type\":\"future_extension\"}\n\n", false, false},
		{"gemini-safety", core.WireFormatGemini, "data: {\"promptFeedback\":{\"blockReason\":\"SAFETY\"}}\n\n", false, false},
		{"gemini-failure", core.WireFormatGemini, "data: {\"candidates\":[{\"finishReason\":\"OTHER\"}]}\n\n", false, true},
		{"responses-length", core.WireFormatOpenAIResponses, "event: response.incomplete\ndata: {\"type\":\"response.incomplete\",\"response\":{\"status\":\"incomplete\",\"incomplete_details\":{\"reason\":\"max_output_tokens\"}}}\n\n", false, false},
		{"responses-failed", core.WireFormatOpenAIResponses, "event: response.failed\ndata: {\"type\":\"response.failed\"}\n\n", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, size := range []int{1, 7, len(tc.body) + 1} {
				s := StreamTermination{Format: tc.format, RequireDone: tc.requireDone}
				s.Feed([]byte(": keepalive\n\ndata:\n\ndata:   \n\n"))
				for p := []byte(tc.body); len(p) > 0; {
					n := min(size, len(p))
					s.Feed(p[:n])
					p = p[n:]
				}
				if err := s.Finish(); (err != nil) != tc.fail {
					t.Fatalf("size %d: %v", size, err)
				}
			}
		})
	}
}

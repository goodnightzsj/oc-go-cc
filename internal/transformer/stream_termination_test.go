package transformer

import (
	"testing"

	"github.com/routatic/proxy/internal/core"
)

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

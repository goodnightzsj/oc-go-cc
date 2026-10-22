package history

import (
	"encoding/json"
	"testing"

	"github.com/routatic/proxy/pkg/types"
)

// The level a client states directly wins over one this proxy infers from a
// token budget: the first is the caller's own words, the second is arithmetic.
func TestRequestedEffortPrefersTheStatedLevel(t *testing.T) {
	for _, tc := range []struct {
		name string
		req  *types.MessageRequest
		want string
	}{
		{
			name: "output_config only",
			req:  &types.MessageRequest{OutputConfig: json.RawMessage(`{"effort":"max"}`)},
			want: "max",
		},
		{
			name: "thinking budget only",
			req:  &types.MessageRequest{Thinking: json.RawMessage(`{"type":"enabled","budget_tokens":63999}`)},
			want: "max",
		},
		{
			name: "stated level beats the budget",
			req: &types.MessageRequest{
				Thinking:     json.RawMessage(`{"type":"enabled","budget_tokens":1024}`),
				OutputConfig: json.RawMessage(`{"effort":"high"}`),
			},
			want: "high",
		},
		{
			// minimal is OpenAI's and has no Anthropic rung; it is raised the
			// same way the inbound paths raise it, so one row cannot disagree
			// with what was actually sent.
			// The column records what the client asked for. Raising the value
			// here would make the record say "low" for a request that said
			// "minimal" - the mapping belongs to the wire boundary, not to the
			// ledger.
			name: "lowest levels are recorded as stated",
			req:  &types.MessageRequest{OutputConfig: json.RawMessage(`{"effort":"minimal"}`)},
			want: "minimal",
		},
		{
			name: "thinking disabled is not a level",
			req:  &types.MessageRequest{Thinking: json.RawMessage(`{"type":"disabled"}`)},
			want: "",
		},
		{
			// A mode is not a level. Storing it would put a value in the column
			// that no provider accepts.
			name: "a mode in output_config is rejected",
			req:  &types.MessageRequest{OutputConfig: json.RawMessage(`{"effort":"enabled"}`)},
			want: "",
		},
		{name: "nothing asked for", req: &types.MessageRequest{}, want: ""},
		{name: "nil request", req: nil, want: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := RequestedEffort(tc.req); got != tc.want {
				t.Errorf("RequestedEffort = %q, want %q", got, tc.want)
			}
		})
	}
}

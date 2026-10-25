package provider

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/pkg/types"
)

// The native Messages path forwards the client's own body, which is the point
// of it. Effort is the one field that still has to be mapped: "none"/"minimal"
// are OpenAI rungs the Anthropic ladder does not have.
func TestNativeAnthropicPayloadMapsTheLowestEffortLevels(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"none", "low"},
		{"minimal", "low"},
		{"low", "low"},
		{"high", "high"},
		{"max", "max"},
	} {
		t.Run(tc.in, func(t *testing.T) {
			body := `{"model":"x","max_tokens":16,"output_config":{"effort":"` + tc.in + `"},"messages":[]}`
			ctx := core.WithRequestMetadata(context.Background(), core.RequestMetadata{Body: []byte(body)})
			out, err := anthropicPayload(ctx, &types.MessageRequest{MaxTokens: 16}, config.ModelConfig{ModelID: "real-model"}, true)
			if err != nil {
				t.Fatal(err)
			}
			var got struct {
				OutputConfig struct {
					Effort string `json:"effort"`
				} `json:"output_config"`
			}
			if err := json.Unmarshal(out, &got); err != nil {
				t.Fatal(err)
			}
			if got.OutputConfig.Effort != tc.want {
				t.Errorf("output_config.effort = %q, want %q", got.OutputConfig.Effort, tc.want)
			}
		})
	}
}

// Everything else must arrive exactly as the client wrote it: the whole reason
// this function passes the raw body through instead of re-marshalling a DTO.
func TestNativeAnthropicPayloadLeavesOtherFieldsAlone(t *testing.T) {
	body := `{"model":"x","max_tokens":16,"output_config":{"effort":"minimal","other":"kept"},"messages":[{"role":"user","content":"hi"}],"custom_field":{"a":1}}`
	ctx := core.WithRequestMetadata(context.Background(), core.RequestMetadata{Body: []byte(body)})
	out, err := anthropicPayload(ctx, &types.MessageRequest{MaxTokens: 16}, config.ModelConfig{ModelID: "real-model"}, true)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	oc := got["output_config"].(map[string]any)
	if oc["other"] != "kept" {
		t.Errorf("a sibling field of effort was dropped: %v", oc)
	}
	if _, ok := got["custom_field"]; !ok {
		t.Error("an unmodelled native field was dropped")
	}
	if got["model"] != "real-model" {
		t.Errorf("model = %v, want the routed model", got["model"])
	}
}

// A body with no effort block must not be rewritten at all.
func TestNativeAnthropicPayloadIsByteIdenticalWithoutEffort(t *testing.T) {
	body := `{"model":"x","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`
	ctx := core.WithRequestMetadata(context.Background(), core.RequestMetadata{Body: []byte(body)})
	out, err := anthropicPayload(ctx, &types.MessageRequest{MaxTokens: 16}, config.ModelConfig{ModelID: "x"}, true)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["output_config"]; ok {
		t.Error("output_config appeared in a request that never had one")
	}
}

// Anthropic checks one relation across two fields: thinking.budget_tokens must
// be smaller than max_tokens. A caller that asks for a large budget under a
// small output cap has stated two things that cannot both hold, and the answer
// is a 400 that names neither field.
//
// The budget is what expresses intent, so it is kept and the cap is raised: a
// cap is a ceiling, not a request.
func TestRaiseMaxTokensAboveBudget(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{
			name: "budget above the cap",
			body: `{"max_tokens":4096,"thinking":{"type":"enabled","budget_tokens":64000}}`,
			want: `68096`,
		},
		{
			name: "budget below the cap is untouched",
			body: `{"max_tokens":64000,"thinking":{"type":"enabled","budget_tokens":4096}}`,
			want: `64000`,
		},
		{
			name: "no thinking block is untouched",
			body: `{"max_tokens":4096}`,
			want: `4096`,
		},
		{
			name: "disabled thinking carries no budget",
			body: `{"max_tokens":4096,"thinking":{"type":"disabled"}}`,
			want: `4096`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := raiseMaxTokensAboveBudget([]byte(tc.body))
			var fields struct {
				MaxTokens int `json:"max_tokens"`
			}
			if err := json.Unmarshal(out, &fields); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got := strconv.Itoa(fields.MaxTokens); got != tc.want {
				t.Fatalf("max_tokens = %s, want %s (body %s)", got, tc.want, out)
			}
		})
	}
}

// Extended thinking pins the sampling knobs: Anthropic accepts only
// temperature 1 while thinking is on and answers 400 otherwise. The caller's
// temperature is meaningless in that mode, so the field is removed rather than
// rewritten - the model's defaults are what thinking wants.
//
// The DTO path has to be exercised directly because the raw pass-through path
// rewrites the captured body instead.
func TestThinkingEnabledDropsSamplingKnobs(t *testing.T) {
	if !thinkingEnabled([]byte(`{"type":"enabled","budget_tokens":4096}`)) {
		t.Error("enabled thinking was not detected")
	}
	if !thinkingEnabled([]byte(`{"type":"adaptive"}`)) {
		t.Error("adaptive thinking was not detected")
	}
	if thinkingEnabled([]byte(`{"type":"disabled"}`)) {
		t.Error("disabled thinking was treated as enabled")
	}
	if thinkingEnabled(nil) {
		t.Error("a missing thinking block was treated as enabled")
	}
}

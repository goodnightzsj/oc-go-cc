package provider

import (
	"context"
	"encoding/json"
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

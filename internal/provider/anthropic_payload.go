package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/internal/transformer"
	"github.com/routatic/proxy/pkg/types"
)

// Native requests retain fields not modeled by our cross-protocol DTO (for
// example tool_choice, cache controls and beta tool definitions).
func anthropicPayload(ctx context.Context, req *types.MessageRequest, model config.ModelConfig, stream bool) ([]byte, error) {
	if req.MaxTokens <= 0 {
		if model.MaxTokens <= 0 {
			return nil, fmt.Errorf("native Messages requires max_tokens in the request or model configuration")
		}
		copy := *req
		copy.MaxTokens = model.MaxTokens
		req = &copy
	}
	raw := core.RequestMetadataFromContext(ctx).Body
	if len(raw) == 0 {
		out := transformer.AnthropicForModel(req, model, stream)
		if adjusted, ok := anthropicEffortField(out.OutputConfig); ok {
			out.OutputConfig = adjusted
		}
		encoded, err := json.Marshal(out)
		if err != nil {
			return nil, err
		}
		return raiseMaxTokensAboveBudget(encoded), nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return nil, fmt.Errorf("invalid native Anthropic request")
	}
	fields["model"], _ = json.Marshal(model.ModelID)
	fields["stream"], _ = json.Marshal(stream)
	fields["max_tokens"], _ = json.Marshal(req.MaxTokens)
	// This is the one place the wire really is Anthropic, and the pass-through
	// deliberately forwards everything else untouched. Effort is the exception:
	// a Codex client's "none"/"minimal" are OpenAI rungs with no Anthropic
	// equivalent, and forwarding them verbatim asks for a level this ladder does
	// not have. Only that field is rewritten; every other native field keeps
	// travelling as the client sent it, which is what this function is for.
	if adjusted, ok := anthropicEffortField(fields["output_config"]); ok {
		fields["output_config"] = adjusted
	}
	encoded, err := json.Marshal(fields)
	if err != nil {
		return nil, err
	}
	return raiseMaxTokensAboveBudget(encoded), nil
}

// raiseMaxTokensAboveBudget enforces the one relation Anthropic checks across
// two fields: thinking.budget_tokens must be smaller than max_tokens.
//
// A caller that asks for a 64000-token budget under a 4096-token output cap has
// stated two things that cannot both hold, and Anthropic answers 400 without
// saying which field to change. Raising the cap keeps the budget the caller
// asked for, which is the part that expresses intent; the cap is a ceiling, not
// a request.
//
// The payload is returned unchanged when there is no thinking block, no budget,
// or the relation already holds, so an ordinary request is forwarded byte for
// byte.
func raiseMaxTokensAboveBudget(body []byte) []byte {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil || fields == nil {
		return body
	}
	var thinking struct {
		BudgetTokens int `json:"budget_tokens"`
	}
	if err := json.Unmarshal(fields["thinking"], &thinking); err != nil || thinking.BudgetTokens <= 0 {
		return body
	}
	var maxTokens int
	if err := json.Unmarshal(fields["max_tokens"], &maxTokens); err != nil || maxTokens > thinking.BudgetTokens {
		return body
	}
	fields["max_tokens"], _ = json.Marshal(thinking.BudgetTokens + 4096)
	raised, err := json.Marshal(fields)
	if err != nil {
		return body
	}
	return raised
}

// anthropicEffortField rewrites output_config.effort for the Anthropic ladder,
// leaving the rest of the object intact. It reports false when there is nothing
// to change, so a request carrying no effort is forwarded byte for byte.
func anthropicEffortField(raw json.RawMessage) (json.RawMessage, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil || obj == nil {
		return nil, false
	}
	var effort string
	if err := json.Unmarshal(obj["effort"], &effort); err != nil || effort == "" {
		return nil, false
	}
	level, ok := core.NormalizeEffort(effort)
	if !ok {
		// Not a level at all - a mode such as "enabled". Leave it alone: guessing
		// would send a value this function invented, and the upstream's rejection
		// names the real problem.
		return nil, false
	}
	mapped := core.AnthropicEffort(level)
	if mapped == effort {
		return nil, false
	}
	obj["effort"], _ = json.Marshal(mapped)
	encoded, err := json.Marshal(obj)
	if err != nil {
		return nil, false
	}
	return encoded, true
}

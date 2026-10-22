package core

import (
	"encoding/json"

	"github.com/routatic/proxy/pkg/types"
)

// thinkingConfig mirrors the Anthropic thinking field structure so we can
// decode it without coupling to a specific json.RawMessage layout.
type thinkingConfig struct {
	Type         string `json:"type"`
	BudgetTokens int    `json:"budget_tokens,omitempty"`
}

// NormalizeRequest converts an Anthropic MessageRequest to a NormalizedRequest.
// This is a lossless extraction: all data from the Anthropic format survives.
//
// Scope: this is a one-way adapter for the Gemini bridge, not the pipeline's
// canonical form. types.MessageRequest already is that - both inbound adapters
// (the native Anthropic decoder and ResponsesToMessageRequest) produce one, and
// every provider consumes one. NormalizedRequest exists because
// AnthropicToGemini needs a flatter shape, and it is read in exactly two places:
// that bridge and the debug capture.
//
// The distinction matters when changing effort handling: a Responses request
// never passes through here, so a field added to NormalizedRequest cannot
// describe what a Codex client asked for. Adding EffortSource here was proposed
// once on the mistaken premise that both entry paths met at this type; they do
// not, and they do not need to.
func NormalizeRequest(anthropicReq *types.MessageRequest) *NormalizedRequest {
	nr := &NormalizedRequest{
		Model:     anthropicReq.Model,
		MaxTokens: anthropicReq.MaxTokens,
		Stream:    anthropicReq.Stream != nil && *anthropicReq.Stream,
	}

	// Extract system prompt (string or array of content blocks).
	nr.SystemPrompt = anthropicReq.SystemText()

	// Set temperature if provided.
	if anthropicReq.Temperature != nil {
		nr.Temperature = anthropicReq.Temperature
	}

	// Extract reasoning effort and thinking budget.
	//
	// Thinking.Type is "enabled"/"disabled" - a mode, not a level - so it is
	// never stored as ReasoningEffort. Storing it there put a value the field's
	// own contract forbids ("low", "medium", "high") into every normalized
	// request, and a consumer reading it as an effort would send "enabled" to a
	// provider that only accepts levels.
	//
	// The level is derived from the budget only because some upstreams accept a
	// level and nothing else; when a caller states a real effort, that value
	// wins (see AnthropicToChatCompletion's precedence).
	if len(anthropicReq.Thinking) > 0 {
		var tc thinkingConfig
		if err := json.Unmarshal(anthropicReq.Thinking, &tc); err == nil {
			nr.ThinkingBudget = tc.BudgetTokens
			if tc.Type == "enabled" && tc.BudgetTokens > 0 {
				nr.ReasoningEffort = EffortFromBudget(tc.BudgetTokens)
			}
		}
	}
	// A client on the newer Anthropic surface states a level directly instead
	// of a budget, and it wins: it is the caller's own words, where the level
	// above is this proxy's inference from a token count. Without this the
	// capture showed no effort at all for those requests, which read as "the
	// client asked for nothing".
	if len(anthropicReq.OutputConfig) > 0 {
		var oc struct {
			Effort string `json:"effort"`
		}
		if err := json.Unmarshal(anthropicReq.OutputConfig, &oc); err == nil && oc.Effort != "" {
			nr.ReasoningEffort = oc.Effort
		}
	}

	// Convert messages.
	for _, msg := range anthropicReq.Messages {
		nm := NormalizedMessage{
			Role: msg.Role,
		}

		blocks := msg.ContentBlocks()
		for _, block := range blocks {
			switch block.Type {
			case "text":
				nm.Content += block.Text
			case "tool_use":
				nm.ToolCalls = append(nm.ToolCalls, NormalizedToolCall{
					ID:        block.ID,
					Name:      block.Name,
					Arguments: string(block.Input),
				})
			case "tool_result":
				nm.ToolResults = append(nm.ToolResults, NormalizedToolResult{
					ToolCallID: block.ToolUseID,
					Content:    block.TextContent(),
				})
			case "thinking":
				nm.Thinking += block.Thinking
			case "image":
				// Preserve image data so the downstream transformer can convert
				// to image_url (or append a [Image] placeholder if the model
				// does not support vision). Previously this was collapsed to
				// the literal text "[Image]" which destroyed the image bytes
				// before the transformer could inspect them.
				if block.Source != nil && block.Source.Data != "" {
					nm.Images = append(nm.Images, NormalizedImage{
						MediaType: block.Source.MediaType,
						Data:      block.Source.Data,
					})
				}
			}
		}

		nr.Messages = append(nr.Messages, nm)
	}

	// Convert tools.
	for _, tool := range anthropicReq.Tools {
		nt := NormalizedToolDef{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: tool.InputSchema,
		}
		nr.Tools = append(nr.Tools, nt)
	}

	return nr
}

// DenormalizeResponse converts a NormalizedResponse to an Anthropic MessageResponse.
func DenormalizeResponse(nr *NormalizedResponse) *types.MessageResponse {
	resp := &types.MessageResponse{
		ID:    nr.ID,
		Type:  "message",
		Model: nr.Model,
		Usage: types.Usage{
			InputTokens:              nr.Usage.InputTokens,
			OutputTokens:             nr.Usage.OutputTokens,
			CacheCreationInputTokens: nr.Usage.CacheCreationTokens,
			CacheReadInputTokens:     nr.Usage.CacheReadTokens,
		},
	}

	// Build content blocks from messages.
	for _, msg := range nr.Messages {
		switch msg.Role {
		case "assistant":
			resp.Role = "assistant"

			// Add thinking block if present.
			if msg.Thinking != "" {
				resp.Content = append(resp.Content, types.ContentBlock{
					Type:     "thinking",
					Thinking: msg.Thinking,
				})
			}

			// Add text block if present.
			if msg.Content != "" {
				resp.Content = append(resp.Content, types.ContentBlock{
					Type: "text",
					Text: msg.Content,
				})
			}

			// Add tool_use blocks.
			for _, tc := range msg.ToolCalls {
				resp.Content = append(resp.Content, types.ContentBlock{
					Type:  "tool_use",
					ID:    tc.ID,
					Name:  tc.Name,
					Input: []byte(tc.Arguments),
				})
			}
		}

		// Determine stop reason.
		switch nr.StopReason {
		case "end_turn":
			resp.StopReason = "end_turn"
		case "max_tokens":
			resp.StopReason = "max_tokens"
		case "tool_use":
			resp.StopReason = "tool_use"
		default:
			resp.StopReason = "end_turn"
		}
	}

	return resp
}

// effortLevels are the values the proxy accepts as a reasoning effort: the
// union of what Anthropic documents (low/medium/high/xhigh/max) and what
// OpenAI documents (which adds none/minimal). The two lowest are accepted and
// raised to "low" by the callers, because the Anthropic ladder has no rung
// below it - see NormalizeEffort.
var effortLevels = map[string]bool{
	"none": true, "minimal": true, "low": true, "medium": true,
	"high": true, "xhigh": true, "max": true,
}

// NormalizeEffort returns the level to send upstream, and whether the input was
// a level at all.
//
// "none" and "minimal" are OpenAI's lowest rungs and have no Anthropic
// equivalent, so they become "low": the request asked for as little reasoning
// as possible and gets the least this ladder offers. Raising a level costs more
// tokens than the caller wanted; rejecting the request delivers nothing.
//
// Anything that is not a level - notably the thinking config's own
// "enabled"/"disabled" - reports false, so a caller can fail instead of sending
// a value no provider accepts.
func NormalizeEffort(effort string) (string, bool) {
	if effort == "" {
		return "", true
	}
	if !effortLevels[effort] {
		return "", false
	}
	if effort == "none" || effort == "minimal" {
		return "low", true
	}
	return effort, true
}

// EffortFromBudget maps an Anthropic thinking budget to an OpenAI effort level.
//
// The bands are a judgement, not a published conversion: Anthropic documents
// budget_tokens as a token target and OpenAI documents effort as a discrete
// level, and neither says how one becomes the other. Normalization, upstream
// transformation and history recording share this conversion.
//
// The ladder stops at "max" rather than reaching OpenAI's "none"/"minimal":
// those mean "do not reason", and a request that carried a positive thinking
// budget did not ask for that.
func EffortFromBudget(budget int) string {
	switch {
	case budget <= 0:
		return ""
	case budget <= 2048:
		return "low"
	case budget <= 8192:
		return "medium"
	case budget <= 32768:
		return "high"
	default:
		return "max"
	}
}

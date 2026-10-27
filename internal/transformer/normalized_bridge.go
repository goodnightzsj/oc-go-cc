package transformer

import (
	"encoding/json"
	"fmt"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/pkg/types"
)

// ── Request-side: NormalizedRequest → wire format ─────────────────────

// AnthropicForModel returns a copy of the request addressed to the given model
// with the stream flag set explicitly. Providers on the Anthropic wire format
// pass the caller's request through unchanged apart from those two fields.
func AnthropicForModel(req *types.MessageRequest, model config.ModelConfig, stream bool) *types.MessageRequest {
	out := *req
	out.Model = model.ModelID
	if stream {
		out.Stream = &stream
	} else {
		out.Stream = nil
	}
	return &out
}

// AnthropicToResponses converts an Anthropic request to a ResponsesRequest.
func AnthropicToResponses(anthropicReq *types.MessageRequest, model config.ModelConfig) (*types.ResponsesRequest, error) {
	if len(anthropicReq.StopSequences) > 0 {
		return nil, fmt.Errorf("Responses does not support stop_sequences")
	}
	// Reuse the existing parameter/tool validation and model overrides. Build
	// input from the original blocks so mixed text/tool chronology is retained.
	chat, err := NewRequestTransformer().transformParameters(anthropicReq, model)
	if err != nil {
		return nil, err
	}
	responsesReq := &types.ResponsesRequest{
		Model: model.ModelID, Temperature: chat.Temperature, TopP: chat.TopP,
		ParallelToolCalls: chat.ParallelToolCalls,
		PromptCacheKey:    anthropicReq.PromptCacheKey,
	}
	if len(chat.ResponseFormat) > 0 {
		var format struct {
			JSONSchema map[string]json.RawMessage `json:"json_schema"`
		}
		if err := json.Unmarshal(chat.ResponseFormat, &format); err != nil {
			return nil, err
		}
		format.JSONSchema["type"] = rawJSONString("json_schema")
		responsesReq.Text, err = json.Marshal(map[string]any{"format": format.JSONSchema})
		if err != nil {
			return nil, err
		}
	}
	if chat.MaxTokens != nil {
		responsesReq.MaxOutputTokens = *chat.MaxTokens
	}
	if chat.ReasoningEffort != nil {
		responsesReq.Reasoning = &types.ResponsesReasoning{Effort: *chat.ReasoningEffort}
	}
	if chat.ToolChoice != nil {
		choice := chat.ToolChoice
		if named, ok := choice.(map[string]any); ok {
			function := named["function"].(map[string]string)
			choice = map[string]string{"type": "function", "name": function["name"]}
		}
		responsesReq.ToolChoice, _ = json.Marshal(choice)
	}

	// System prompt becomes a "developer" role input.
	if system := anthropicReq.SystemText(); system != "" {
		responsesReq.Input = append(responsesReq.Input, types.ResponsesInput{
			Role:    "developer",
			Content: rawJSONString(system),
		})
	}

	for _, msg := range anthropicReq.Messages {
		text := ""
		flushText := func() {
			if text != "" {
				responsesReq.Input = append(responsesReq.Input, types.ResponsesInput{Role: msg.Role, Content: rawJSONString(text)})
				text = ""
			}
		}
		for _, block := range msg.ContentBlocks() {
			switch block.Type {
			case "text":
				text += block.Text
			case "thinking":
				// Anthropic's opaque reasoning/signature is not Responses state.
				// Keep the pre-existing cross-protocol omission of this history.
			case "tool_use":
				flushText()
				if block.ID == "" || block.Name == "" {
					return nil, fmt.Errorf("responses tool_use requires id and name")
				}
				arguments := string(block.Input)
				if arguments == "" {
					arguments = "{}"
				}
				if !json.Valid([]byte(arguments)) {
					return nil, fmt.Errorf("invalid Responses tool arguments")
				}
				responsesReq.Input = append(responsesReq.Input, types.ResponsesInput{Type: "function_call", CallID: block.ID, Name: block.Name, Arguments: arguments})
			case "tool_result":
				flushText()
				output, err := responsesToolOutput(block, model)
				if err != nil {
					return nil, err
				}
				responsesReq.Input = append(responsesReq.Input, types.ResponsesInput{Type: "function_call_output", CallID: block.GetToolID(), Output: output})
			case "image":
				flushText()
				// Only a model known to refuse images fails here. An unstated
				// capability is forwarded: the Responses upstream is the party
				// that knows, and it can say no more accurately than a guess can.
				if vision, known := config.ModelHasVision(model); known && !vision {
					return nil, fmt.Errorf("responses image input requires a vision-capable model")
				}
				url, err := imageSourceURL(block.Source)
				if err != nil {
					return nil, err
				}
				content, _ := json.Marshal([]map[string]string{{"type": "input_image", "image_url": url}})
				responsesReq.Input = append(responsesReq.Input, types.ResponsesInput{Role: msg.Role, Content: content})
			default:
				return nil, fmt.Errorf("unsupported Responses input block %q", block.Type)
			}
		}
		flushText()
	}

	// Convert tools.
	for _, tool := range chat.Tools {
		responsesReq.Tools = append(responsesReq.Tools, types.ResponsesTool{
			Type:        "function",
			Name:        tool.Function.Name,
			Description: tool.Function.Description,
			Parameters:  tool.Function.Parameters,
		})
	}

	return responsesReq, nil
}

func responsesToolOutput(block types.ContentBlock, model config.ModelConfig) (json.RawMessage, error) {
	parts, err := toolResultBlocks(block)
	if err != nil {
		return nil, err
	}
	content := make([]map[string]string, 0, len(parts))
	text := ""
	hasImage := false
	for _, part := range parts {
		switch part.Type {
		case "text":
			text += part.Text
			content = append(content, map[string]string{"type": "input_text", "text": part.Text})
		case "image":
			if vision, known := config.ModelHasVision(model); known && !vision {
				return nil, fmt.Errorf("responses tool_result image requires a vision-capable model")
			}
			url, err := imageSourceURL(part.Source)
			if err != nil {
				return nil, err
			}
			hasImage = true
			content = append(content, map[string]string{"type": "input_image", "image_url": url})
		default:
			return nil, fmt.Errorf("unsupported Responses tool_result content %q", part.Type)
		}
	}
	if !hasImage {
		return rawJSONString(text), nil
	}
	return json.Marshal(content)
}

// geminiThinking resolves modes before converting a level or token budget.
func geminiThinking(req *core.NormalizedRequest, model config.ModelConfig) *types.GeminiThinkingConfig {
	support := core.ReasoningSupportFor(model)
	// Gemini 3 and later take a discrete level; 2.5 and earlier take a token
	// budget. Sending both is the conflict Gemini documents as a 400, so the
	// capability decides which one this target gets.
	if len(support.Levels) > 0 && !support.Budget {
		level := req.ReasoningEffort
		if req.ThinkingMode == "disabled" {
			level = "none"
		} else if level == "" && req.ThinkingBudget != nil {
			if *req.ThinkingBudget == 0 {
				level = "none"
			} else {
				level = core.EffortFromBudget(*req.ThinkingBudget)
			}
		}
		if level == "" {
			return nil
		}
		if clamped, ok := support.ClampLevel(level); ok {
			level = clamped
		}
		// A level request implies the caller wants to see what the model did
		// with it. Asking for thoughts costs nothing when the model has none
		// and is the difference between "no reasoning" and "not requested".
		include := true
		return &types.GeminiThinkingConfig{ThinkingLevel: &level, IncludeThoughts: &include}
	}
	var budget int
	switch {
	case req.ThinkingMode == "disabled":
		budget = 0
	case req.ThinkingBudget != nil:
		budget = *req.ThinkingBudget
	case isOffEffort(req.ReasoningEffort):
		budget = 0
	case req.ReasoningEffort != "":
		budget = core.BudgetForEffort(req.ReasoningEffort)
		if budget == 0 {
			return nil
		}
	case req.ThinkingMode == "adaptive":
		budget = -1
	default:
		return nil
	}
	if budget == 0 && support.CannotDisable {
		budget = support.MinBudget
	}
	budget = support.ClampBudget(budget)
	return &types.GeminiThinkingConfig{ThinkingBudget: &budget}
}

// AnthropicToGemini converts an Anthropic request to a GeminiRequest.
func AnthropicToGemini(anthropicReq *types.MessageRequest, model config.ModelConfig) (*types.GeminiRequest, error) {
	req := core.NormalizeRequest(anthropicReq)
	if req.ThinkingMode == "" && req.ReasoningEffort == "" && req.ThinkingBudget == nil {
		defaults := &types.MessageRequest{Thinking: model.Thinking}
		if model.ReasoningEffort != "" {
			defaults.OutputConfig, _ = json.Marshal(map[string]string{"effort": model.ReasoningEffort})
		}
		intent := core.NormalizeRequest(defaults)
		req.ThinkingMode, req.ThinkingBudget, req.ReasoningEffort = intent.ThinkingMode, intent.ThinkingBudget, intent.ReasoningEffort
	}
	geminiReq := &types.GeminiRequest{
		GenerationConfig: &types.GeminiGenerationConfig{
			MaxOutputTokens: req.MaxTokens,
			Temperature:     anthropicReq.Temperature,
			TopP:            anthropicReq.TopP,
			StopSequences:   anthropicReq.StopSequences,
		},
	}

	if err := geminiControls(anthropicReq, model, geminiReq); err != nil {
		return nil, err
	}

	// Gemini expresses reasoning as a token budget, and it was the one target
	// that received nothing at all: the normalised request already carried the
	// budget and the derived level, but this function never read them, so the
	// model silently used its default level. A budget the client stated is
	// forwarded as-is; a level is converted, because a level-only client has no
	// other way to say it. Neither present means the model decides, which is
	// what an absent thinkingConfig already means.
	if thinking := geminiThinking(req, model); thinking != nil {
		geminiReq.GenerationConfig.ThinkingConfig = thinking
	}

	if req.SystemPrompt != "" {
		geminiReq.SystemInstruction = &types.GeminiContent{
			Parts: []types.GeminiPart{{Text: req.SystemPrompt}},
		}
	}

	calls := map[string]string{}
	for _, msg := range anthropicReq.Messages {
		gc := types.GeminiContent{Role: msg.Role}
		if gc.Role == "assistant" {
			gc.Role = "model"
		} else if gc.Role != "user" {
			return nil, fmt.Errorf("unsupported Gemini message role %q", msg.Role)
		}
		for _, block := range msg.ContentBlocks() {
			if (block.Type == "tool_use" || block.Type == "thinking") && gc.Role != "model" || block.Type == "tool_result" && gc.Role != "user" {
				return nil, fmt.Errorf("Gemini %s cannot occur in role %s", block.Type, msg.Role)
			}
			part, err := geminiInputPart(block, model, calls)
			if err != nil {
				return nil, err
			}
			gc.Parts = append(gc.Parts, part)
		}
		geminiReq.Contents = append(geminiReq.Contents, gc)
	}

	// Convert tools.
	if len(req.Tools) > 0 {
		var functions []types.GeminiFunctionDeclaration
		for _, tool := range req.Tools {
			functions = append(functions, types.GeminiFunctionDeclaration{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.InputSchema,
			})
		}
		geminiReq.Tools = []types.GeminiTool{
			{FunctionDeclarations: functions},
		}
	}

	return geminiReq, nil
}

// ── Response-side: wire format → NormalizedResponse ───────────────────

// ResponsesToNormalized converts an OpenAI ResponsesResponse to NormalizedResponse.
func ResponsesToNormalized(responsesResp *types.ResponsesResponse, modelID string) (_ *core.NormalizedResponse, err error) {
	defer func() {
		if err != nil {
			err = &core.ResponseError{Err: err, Usage: ResponsesUsageToAnthropic(responsesResp.Usage)}
		}
	}()
	if err := ResponsesResponseError(responsesResp); err != nil {
		return nil, err
	}
	stopReason, _ := responsesFinishReason(responsesResp.Status, responsesResp.IncompleteDetails)
	nr := &core.NormalizedResponse{
		ID:         responsesResp.ID,
		Model:      modelID,
		StopReason: stopReason,
	}

	for _, output := range responsesResp.Output {
		switch output.Type {
		case "reasoning":
			if output.EncryptedContent != "" {
				return nil, fmt.Errorf("encrypted Responses reasoning requires a native Responses target")
			}
			for _, summary := range output.Summary {
				if summary.Type != "summary_text" {
					return nil, fmt.Errorf("unsupported Responses reasoning summary %q", summary.Type)
				}
				nr.Messages = append(nr.Messages, core.NormalizedMessage{Role: "assistant", Thinking: summary.Text})
			}
		case "message":
			nm := core.NormalizedMessage{Role: output.Role}
			for _, c := range output.Content {
				if c.Type == "output_text" {
					nm.Content += c.Text
				} else if c.Type == "refusal" {
					nm.Content += c.Refusal
					if nr.StopReason == "end_turn" {
						nr.StopReason = "refusal"
					}
				}
			}
			nr.Messages = append(nr.Messages, nm)
		case "function_call":
			if nr.StopReason == "end_turn" {
				nr.StopReason = "tool_use"
			}
			nm := core.NormalizedMessage{
				Role: "assistant",
				ToolCalls: []core.NormalizedToolCall{
					{
						ID:        output.CallID,
						Name:      output.Name,
						Arguments: output.Arguments,
					},
				},
			}
			nr.Messages = append(nr.Messages, nm)
		}
	}

	if usage := ResponsesUsageToAnthropic(responsesResp.Usage); usage != nil {
		nr.Usage = &core.NormalizedUsage{
			InputTokens: usage.InputTokens, OutputTokens: usage.OutputTokens, CacheReadTokens: usage.CacheReadInputTokens,
			ReasoningTokens: usage.ReasoningTokens,
		}
	}

	return nr, nil
}

func responsesFinishReason(status string, details *types.ResponsesIncompleteDetails) (string, error) {
	switch status {
	case "completed":
		return "end_turn", nil
	case "incomplete":
		if details != nil {
			switch details.Reason {
			case "max_output_tokens":
				return "max_tokens", nil
			case "content_filter":
				return "refusal", nil
			}
		}
	}
	return "", fmt.Errorf("unsuccessful or missing Responses status %q", status)
}

// GeminiToNormalized converts a GeminiResponse to NormalizedResponse.
func GeminiToNormalized(geminiResp *types.GeminiResponse, modelID string) (_ *core.NormalizedResponse, err error) {
	defer func() {
		if err != nil {
			err = &core.ResponseError{Err: err, Usage: geminiUsageToAnthropic(geminiResp.UsageMetadata)}
		}
	}()
	if err := upstreamJSONError("Gemini", geminiResp.Error); err != nil {
		return nil, err
	}
	nr := &core.NormalizedResponse{
		Model: modelID,
	}
	if geminiResp.PromptFeedback != nil && geminiResp.PromptFeedback.BlockReason != "" {
		stop, err := geminiFinishReason(geminiResp.PromptFeedback.BlockReason)
		if err != nil {
			return nil, err
		}
		nr.StopReason = stop
	} else if len(geminiResp.Candidates) == 0 {
		return nil, fmt.Errorf("Gemini response has no candidate or prompt feedback")
	}

	if len(geminiResp.Candidates) > 0 {
		candidate := geminiResp.Candidates[0]
		hasTools := false
		// Keep part order: a thought summary must never become answer text.
		for _, part := range candidate.Content.Parts {
			if err := validateGeminiOutputPart(part); err != nil {
				return nil, err
			}
			if part.Text == "" && part.FunctionCall == nil {
				continue
			}
			nm := core.NormalizedMessage{Role: candidate.Content.Role}
			if nm.Role == "model" {
				nm.Role = "assistant"
			}
			if part.FunctionCall != nil {
				call, err := geminiToolCall(part.FunctionCall)
				if err != nil {
					return nil, err
				}
				nm.ToolCalls = []core.NormalizedToolCall{call}
				hasTools = true
			} else if part.Thought {
				nm.Thinking = part.Text
			} else {
				nm.Content = part.Text
			}
			nr.Messages = append(nr.Messages, nm)
		}

		stop, err := geminiFinishReason(candidate.FinishReason)
		if err != nil {
			return nil, err
		}
		nr.StopReason = stop
		if hasTools && nr.StopReason == "end_turn" {
			nr.StopReason = "tool_use"
		}
	}

	if usage := geminiUsageToAnthropic(geminiResp.UsageMetadata); usage != nil {
		nr.Usage = &core.NormalizedUsage{
			InputTokens:     usage.InputTokens,
			OutputTokens:    usage.OutputTokens,
			CacheReadTokens: usage.CacheReadInputTokens,
			ReasoningTokens: usage.ReasoningTokens,
		}
	}

	return nr, nil
}

func geminiFinishReason(reason string) (string, error) {
	switch reason {
	case "STOP":
		return "end_turn", nil
	case "MAX_TOKENS":
		return "max_tokens", nil
	case "SAFETY", "RECITATION", "BLOCKLIST", "PROHIBITED_CONTENT", "SPII", "IMAGE_SAFETY", "IMAGE_PROHIBITED_CONTENT":
		return "refusal", nil
	default:
		return "", fmt.Errorf("unsuccessful or missing Gemini finish reason %q", reason)
	}
}

// ── Helpers ───────────────────────────────────────────────────────────

func rawJSONString(s string) json.RawMessage {
	b, err := json.Marshal(s)
	if err != nil {
		return json.RawMessage(`""`)
	}
	return json.RawMessage(b)
}

// AnthropicToChatCompletion converts an Anthropic request to the OpenAI Chat
// Completions format used by most upstreams.
func AnthropicToChatCompletion(req *types.MessageRequest, model config.ModelConfig) (*types.ChatCompletionRequest, error) {
	return NewRequestTransformer().TransformRequest(req, model)
}

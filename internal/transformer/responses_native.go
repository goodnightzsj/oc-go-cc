package transformer

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/routatic/proxy/pkg/types"
)

// ResponsesRoutingRequest extracts routing facts only. Its result must never be
// sent upstream: native-only fields remain in the original request body.
func ResponsesRoutingRequest(raw []byte) (*types.MessageRequest, bool, error) {
	var in responsesInboundRequest
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, false, fmt.Errorf("invalid Responses request: %w", err)
	}
	if strings.TrimSpace(in.Model) == "" {
		return nil, false, fmt.Errorf("model is required")
	}
	if in.Store != nil && *in.Store || in.Background || in.PreviousResponseID != "" || len(in.Conversation) > 0 && string(in.Conversation) != "null" {
		return nil, false, fmt.Errorf("store, background, previous_response_id and conversation are unavailable on the stateless Responses endpoint")
	}
	var resources struct {
		Prompt json.RawMessage   `json:"prompt"`
		Tools  []json.RawMessage `json:"tools"`
	}
	if err := json.Unmarshal(raw, &resources); err != nil {
		return nil, false, err
	}
	if len(resources.Prompt) > 0 && string(resources.Prompt) != "null" {
		return nil, false, fmt.Errorf("stored prompts require a source-bound resource contract")
	}
	if err := validateStatelessResponsesTools(resources.Tools); err != nil {
		return nil, false, err
	}
	out := &types.MessageRequest{Model: in.Model, Stream: in.Stream, Temperature: in.Temperature, TopP: in.TopP, PromptCacheKey: in.PromptCacheKey}
	if in.MaxOutputTokens != nil {
		if *in.MaxOutputTokens <= 0 {
			return nil, false, fmt.Errorf("max_output_tokens must be positive")
		}
		out.MaxTokens = *in.MaxOutputTokens
	}
	if in.Reasoning != nil {
		out.OutputConfig, _ = json.Marshal(map[string]string{"effort": in.Reasoning.Effort})
	}
	if in.Instructions != "" {
		out.System, _ = json.Marshal(in.Instructions)
	}
	for _, tool := range in.Tools {
		out.Tools = append(out.Tools, types.Tool{Name: tool.Name, InputSchema: tool.Parameters})
	}
	var text string
	if json.Unmarshal(in.Input, &text) == nil && len(in.Input) > 0 && in.Input[0] == '"' {
		out.Messages = []types.Message{{Role: "user", Content: in.Input}}
		return out, false, nil
	}
	var items []json.RawMessage
	if err := json.Unmarshal(in.Input, &items); err != nil || len(items) == 0 {
		return nil, false, fmt.Errorf("input must be a string or a nonempty item array")
	}
	replay := false
	for _, rawItem := range items {
		var item responsesInboundItem
		if err := json.Unmarshal(rawItem, &item); err != nil {
			return nil, false, fmt.Errorf("invalid Responses input item: %w", err)
		}
		role := item.Role
		var blocks []types.ContentBlock
		var err error
		switch item.Type {
		case "", "message":
			if role != "user" && role != "assistant" && role != "system" && role != "developer" {
				return nil, false, fmt.Errorf("unsupported input role %q", role)
			}
			blocks, err = responsesContent(item.Content, true, true)
		case "function_call", "custom_tool_call":
			role = "assistant"
			// Count the complete call in routing estimates without parsing or
			// rewriting freeform/partially generated arguments.
			blocks = []types.ContentBlock{{Type: "text", Text: string(rawItem)}}
		case "function_call_output", "custom_tool_call_output":
			role = "user"
			blocks, err = responsesContent(item.Output, true, true)
		case "reasoning":
			role = "assistant"
			replay = replay || item.EncryptedContent != "" || item.ID != ""
			for _, summary := range item.Summary {
				blocks = append(blocks, types.ContentBlock{Type: "thinking", Thinking: summary.Text})
			}
		case "web_search_call", "file_search_call", "computer_call", "computer_call_output", "code_interpreter_call", "image_generation_call", "mcp_call", "mcp_list_tools", "mcp_approval_request", "mcp_approval_response":
			return nil, false, fmt.Errorf("%s replay requires a source-bound resource contract", item.Type)
		default:
			return nil, false, fmt.Errorf("unsupported stateless input item %q", item.Type)
		}
		if err != nil {
			return nil, false, err
		}
		appendResponsesMessage(out, role, blocks)
	}
	return out, replay, nil
}

func validateStatelessResponsesTools(tools []json.RawMessage) error {
	for _, raw := range tools {
		var tool struct {
			Type      string            `json:"type"`
			Container json.RawMessage   `json:"container"`
			Tools     []json.RawMessage `json:"tools"`
		}
		if err := json.Unmarshal(raw, &tool); err != nil {
			return fmt.Errorf("invalid Responses tool: %w", err)
		}
		if tool.Type == "file_search" {
			return fmt.Errorf("file_search requires a source-bound resource contract")
		}
		if tool.Type == "code_interpreter" {
			var container struct {
				Type    string   `json:"type"`
				FileIDs []string `json:"file_ids"`
			}
			if json.Unmarshal(tool.Container, &container) != nil || container.Type != "auto" || len(container.FileIDs) > 0 {
				return fmt.Errorf("code_interpreter requires an auto container without stored files")
			}
		}
		if err := validateStatelessResponsesTools(tool.Tools); err != nil {
			return err
		}
	}
	return nil
}

// NativeResponsesPayload changes transport-owned fields only. The upstream
// validates native extensions, schemas and tool declarations, not a Chat DTO.
func NativeResponsesPayload(raw []byte, model string, stream bool) (json.RawMessage, error) {
	var body map[string]json.RawMessage
	if err := json.Unmarshal(raw, &body); err != nil || body == nil {
		return nil, fmt.Errorf("invalid native Responses object")
	}
	body["model"], _ = json.Marshal(model)
	body["stream"], _ = json.Marshal(stream)
	body["store"] = json.RawMessage("false")
	return json.Marshal(body)
}

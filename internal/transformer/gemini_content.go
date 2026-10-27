package transformer

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/pkg/types"
)

func geminiControls(req *types.MessageRequest, model config.ModelConfig, out *types.GeminiRequest) error {
	gc := out.GenerationConfig
	if model.Temperature > 0 {
		gc.Temperature = &model.Temperature
	}
	if model.MaxTokens > 0 {
		gc.MaxOutputTokens = model.MaxTokens
	}
	if len(req.StopSequences) > 5 {
		return fmt.Errorf("Gemini accepts at most 5 stop_sequences")
	}
	if len(req.OutputConfig) > 0 {
		var output struct {
			Format json.RawMessage `json:"format"`
		}
		if err := json.Unmarshal(req.OutputConfig, &output); err != nil {
			return fmt.Errorf("invalid output_config: %w", err)
		}
		if len(output.Format) > 0 && string(output.Format) != "null" {
			if _, err := openAIResponseFormat(output.Format); err != nil {
				return err
			}
			var format struct {
				Schema json.RawMessage `json:"schema"`
			}
			if err := json.Unmarshal(output.Format, &format); err != nil {
				return err
			}
			gc.ResponseMIMEType = "application/json"
			gc.ResponseJSONSchema = format.Schema
		}
	}
	if len(req.ToolChoice) > 0 {
		var choice struct {
			Type            string `json:"type"`
			Name            string `json:"name"`
			DisableParallel bool   `json:"disable_parallel_tool_use"`
		}
		if err := json.Unmarshal(req.ToolChoice, &choice); err != nil {
			return fmt.Errorf("invalid tool_choice: %w", err)
		}
		if choice.DisableParallel {
			return fmt.Errorf("Gemini cannot enforce disable_parallel_tool_use")
		}
		calling := types.GeminiFunctionCallingConfig{}
		switch choice.Type {
		case "auto":
			calling.Mode = "AUTO"
		case "none":
			calling.Mode = "NONE"
		case "any":
			calling.Mode = "ANY"
		case "tool":
			if strings.TrimSpace(choice.Name) == "" {
				return fmt.Errorf("tool_choice.name is required for a named tool")
			}
			calling.Mode = "ANY"
			calling.AllowedFunctionNames = []string{choice.Name}
		default:
			return fmt.Errorf("unsupported tool_choice type %q", choice.Type)
		}
		out.ToolConfig = &types.GeminiToolConfig{FunctionCallingConfig: calling}
	}
	return nil
}

func geminiInputPart(block types.ContentBlock, model config.ModelConfig, calls map[string]string) (types.GeminiPart, error) {
	part := types.GeminiPart{}
	switch block.Type {
	case "text":
		part.Text = block.Text
	case "thinking":
		if block.Signature != "" && block.Signature != thinkingSignaturePlaceholder {
			return part, fmt.Errorf("Anthropic signature cannot be replayed as Gemini thoughtSignature")
		}
		part.Text, part.Thought = block.Thinking, true
	case "image":
		return geminiImagePart(block.Source, model)
	case "tool_use":
		if block.ID == "" || block.Name == "" {
			return part, fmt.Errorf("Gemini tool_use requires id and name")
		}
		if _, exists := calls[block.ID]; exists {
			return part, fmt.Errorf("duplicate Gemini tool_use id %q", block.ID)
		}
		args := block.Input
		if len(args) == 0 {
			args = json.RawMessage(`{}`)
		}
		var object map[string]json.RawMessage
		if json.Unmarshal(args, &object) != nil || object == nil {
			return part, fmt.Errorf("Gemini tool arguments must be a JSON object")
		}
		calls[block.ID] = block.Name
		part.FunctionCall = &types.GeminiFunctionCall{ID: block.ID, Name: block.Name, Args: args}
	case "tool_result":
		name, exists := calls[block.GetToolID()]
		if !exists {
			return part, fmt.Errorf("Gemini tool_result has no matching tool_use %q", block.GetToolID())
		}
		inner, err := toolResultBlocks(block)
		if err != nil {
			return part, err
		}
		var text strings.Builder
		for _, content := range inner {
			if content.Type != "text" {
				return part, fmt.Errorf("Gemini tool_result content %q requires a multimodal result mapping", content.Type)
			}
			text.WriteString(content.Text)
		}
		key := "output"
		if block.IsError != nil && *block.IsError {
			key = "error"
		}
		response, err := json.Marshal(map[string]string{key: text.String()})
		if err != nil {
			return part, err
		}
		part.FunctionResponse = &types.GeminiFunctionResponse{ID: block.GetToolID(), Name: name, Response: response}
		delete(calls, block.GetToolID())
	default:
		return part, fmt.Errorf("unsupported Gemini content block %q", block.Type)
	}
	return part, nil
}

func geminiImagePart(source *types.ImageSource, model config.ModelConfig) (types.GeminiPart, error) {
	part := types.GeminiPart{}
	if vision, known := config.ModelHasVision(model); known && !vision {
		return part, fmt.Errorf("Gemini image requires a vision-capable model")
	}
	if _, err := imageSourceURL(source); err != nil {
		return part, err
	}
	if source.Type == "url" {
		part.FileData = &types.GeminiFileData{MIMEType: source.MediaType, FileURI: source.URL}
	} else {
		part.InlineData = &types.GeminiBlob{MIMEType: source.MediaType, Data: source.Data}
	}
	return part, nil
}

func geminiToolCall(call *types.GeminiFunctionCall) (core.NormalizedToolCall, error) {
	args := call.Args
	if len(args) == 0 {
		args = json.RawMessage(`{}`)
	}
	var object map[string]json.RawMessage
	if call.Name == "" || json.Unmarshal(args, &object) != nil || object == nil {
		return core.NormalizedToolCall{}, fmt.Errorf("Gemini functionCall requires name and JSON object args")
	}
	id := call.ID
	if id == "" {
		id = "toolu_" + generateID()
	}
	return core.NormalizedToolCall{ID: id, Name: call.Name, Arguments: string(args)}, nil
}

func validateGeminiOutputPart(part types.GeminiPart) error {
	if part.ThoughtSignature != "" {
		return fmt.Errorf("Gemini thoughtSignature requires a source-bound replay carrier")
	}
	if part.InlineData != nil || part.FileData != nil || part.FunctionResponse != nil {
		return fmt.Errorf("Gemini output media or functionResponse has no supported Messages mapping")
	}
	if part.FunctionCall != nil && part.Text != "" {
		return fmt.Errorf("Gemini part has conflicting text and functionCall")
	}
	return nil
}

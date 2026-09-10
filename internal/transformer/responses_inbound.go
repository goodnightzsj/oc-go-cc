package transformer

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/routatic/proxy/pkg/types"
)

// This adapter implements the stateless Responses contract over the existing
// Messages pipeline. Features without a lossless mapping fail at the boundary.
// Reference: https://developers.openai.com/api/reference/resources/responses
type responsesInboundRequest struct {
	Model              string          `json:"model"`
	Input              json.RawMessage `json:"input"`
	Instructions       string          `json:"instructions"`
	Stream             *bool           `json:"stream"`
	MaxOutputTokens    *int            `json:"max_output_tokens"`
	Temperature        *float64        `json:"temperature"`
	TopP               *float64        `json:"top_p"`
	ToolChoice         json.RawMessage `json:"tool_choice"`
	ParallelToolCalls  *bool           `json:"parallel_tool_calls"`
	Store              *bool           `json:"store"`
	Background         bool            `json:"background"`
	PreviousResponseID string          `json:"previous_response_id"`
	Conversation       json.RawMessage `json:"conversation"`
	Include            []string        `json:"include"`
	// Codex transport metadata does not contain conversation state. Cache
	// affinity is advisory; it is not a previous-response identifier.
	PromptCacheKey string          `json:"prompt_cache_key"`
	ClientMetadata json.RawMessage `json:"client_metadata"`
	Reasoning      *struct {
		Effort  string `json:"effort"`
		Summary string `json:"summary"`
	} `json:"reasoning"`
	Text *struct {
		Format *struct {
			Type        string          `json:"type"`
			Name        string          `json:"name"`
			Description string          `json:"description"`
			Schema      json.RawMessage `json:"schema"`
			Strict      *bool           `json:"strict"`
		} `json:"format"`
		Verbosity string `json:"verbosity"`
	} `json:"text"`
	Tools []struct {
		Type        string          `json:"type"`
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Parameters  json.RawMessage `json:"parameters"`
		Strict      *bool           `json:"strict"`
		Format      json.RawMessage `json:"format"`
	} `json:"tools"`
}

type responsesInboundItem struct {
	Type             string          `json:"type"`
	ID               string          `json:"id"`
	Status           string          `json:"status"`
	Role             string          `json:"role"`
	Content          json.RawMessage `json:"content"`
	CallID           string          `json:"call_id"`
	Name             string          `json:"name"`
	Arguments        string          `json:"arguments"`
	Output           json.RawMessage `json:"output"`
	EncryptedContent string          `json:"encrypted_content"`
	Summary          []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"summary"`
}

func decodeResponsesInput(raw []byte, dst any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("expected one JSON value")
	}
	return nil
}

// ResponsesToMessageRequest preserves message/tool order and rejects unsupported
// state, hosted tools and multimodal tool outputs instead of dropping them.
func ResponsesToMessageRequest(raw []byte) (*types.MessageRequest, error) {
	var in responsesInboundRequest
	if err := decodeResponsesInput(raw, &in); err != nil {
		return nil, fmt.Errorf("invalid Responses request: %w", err)
	}
	if strings.TrimSpace(in.Model) == "" {
		return nil, fmt.Errorf("model is required")
	}
	for name, enabled := range map[string]bool{
		"store": in.Store != nil && *in.Store, "background": in.Background,
		"previous_response_id": in.PreviousResponseID != "",
		"conversation":         len(in.Conversation) > 0 && string(in.Conversation) != "null",
		"include":              len(in.Include) > 0,
	} {
		if enabled {
			return nil, fmt.Errorf("%s is not supported by the stateless Responses adapter", name)
		}
	}
	if in.Text != nil && (in.Text.Verbosity != "" || (in.Text.Format != nil && in.Text.Format.Type != "text")) {
		return nil, fmt.Errorf("text format/verbosity is not supported; use plain text")
	}
	if in.Text != nil && in.Text.Format != nil {
		format := in.Text.Format
		if format.Name != "" || format.Description != "" || len(format.Schema) > 0 || format.Strict != nil {
			return nil, fmt.Errorf("text.format options are not supported; use plain text")
		}
	}
	out := &types.MessageRequest{Model: in.Model, Stream: in.Stream, Temperature: in.Temperature, TopP: in.TopP}
	if in.MaxOutputTokens != nil {
		if *in.MaxOutputTokens <= 0 {
			return nil, fmt.Errorf("max_output_tokens must be positive")
		}
		out.MaxTokens = *in.MaxOutputTokens
	}
	if in.Reasoning != nil {
		if in.Reasoning.Summary != "" {
			return nil, fmt.Errorf("reasoning.summary is not supported by this adapter")
		}
		switch in.Reasoning.Effort {
		case "":
		case "low", "medium", "high", "max":
			out.OutputConfig, _ = json.Marshal(map[string]string{"effort": in.Reasoning.Effort})
		default:
			return nil, fmt.Errorf("reasoning.effort %q has no supported upstream mapping", in.Reasoning.Effort)
		}
	}
	for _, tool := range in.Tools {
		if tool.Type != "function" {
			return nil, fmt.Errorf("tool type %q is not supported; use JSON function tools, not custom/freeform or hosted tools", tool.Type)
		}
		if tool.Strict == nil || *tool.Strict {
			return nil, fmt.Errorf("function tools require strict:false; strict schema enforcement is not supported")
		}
		if strings.TrimSpace(tool.Name) == "" || len(tool.Format) != 0 {
			return nil, fmt.Errorf("function tool requires a name and does not support format")
		}
		schema := tool.Parameters
		if len(schema) == 0 || string(schema) == "null" {
			schema = json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
		}
		var object map[string]json.RawMessage
		if err := json.Unmarshal(schema, &object); err != nil || string(object["type"]) != `"object"` {
			return nil, fmt.Errorf("function tool parameters must be an object schema")
		}
		if properties := object["properties"]; len(properties) > 0 {
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(properties, &fields); err != nil || fields == nil {
				return nil, fmt.Errorf("function tool parameters.properties must be an object")
			}
		}
		out.Tools = append(out.Tools, types.Tool{Name: tool.Name, Description: tool.Description, InputSchema: schema})
	}
	if len(in.ToolChoice) > 0 && string(in.ToolChoice) != "null" || in.ParallelToolCalls != nil {
		choice := map[string]any{"type": "auto"}
		if len(in.ToolChoice) > 0 && string(in.ToolChoice) != "null" {
			var name string
			if json.Unmarshal(in.ToolChoice, &name) == nil {
				switch name {
				case "auto", "none":
					choice["type"] = name
				case "required":
					choice["type"] = "any"
				default:
					return nil, fmt.Errorf("unsupported tool_choice %q", name)
				}
			} else {
				var tool struct {
					Type string `json:"type"`
					Name string `json:"name"`
				}
				if err := decodeResponsesInput(in.ToolChoice, &tool); err != nil || tool.Type != "function" || tool.Name == "" {
					return nil, fmt.Errorf("tool_choice must be auto, none, required, or a named function")
				}
				choice["type"], choice["name"] = "tool", tool.Name
			}
		}
		if in.ParallelToolCalls != nil && !*in.ParallelToolCalls {
			choice["disable_parallel_tool_use"] = true
		}
		if name, named := choice["name"].(string); named {
			found := false
			for _, tool := range out.Tools {
				found = found || tool.Name == name
			}
			if !found {
				return nil, fmt.Errorf("tool_choice names an undeclared function")
			}
		}
		if len(out.Tools) > 0 {
			out.ToolChoice, _ = json.Marshal(choice)
		} else if choice["type"] == "any" {
			return nil, fmt.Errorf("required tool_choice needs at least one function tool")
		}
	}
	var items []responsesInboundItem
	input := bytes.TrimSpace(in.Input)
	if len(input) > 0 && input[0] == '"' {
		items = []responsesInboundItem{{Role: "user", Content: input}}
	} else if err := decodeResponsesInput(input, &items); err != nil {
		return nil, fmt.Errorf("input must be a string or an array of messages/tool calls: %w", err)
	}
	var system []string
	if in.Instructions != "" {
		system = append(system, in.Instructions)
	}
	for i, item := range items {
		var role string
		var blocks []types.ContentBlock
		switch item.Type {
		case "", "message":
			role = item.Role
			if role != "user" && role != "assistant" && role != "system" && role != "developer" {
				return nil, fmt.Errorf("input[%d] has unsupported role %q", i, role)
			}
			var err error
			blocks, err = responsesInputContent(item.Content, role == "user")
			if err != nil {
				return nil, fmt.Errorf("input[%d]: %w", i, err)
			}
			if role == "system" || role == "developer" {
				if len(out.Messages) > 0 {
					return nil, fmt.Errorf("system/developer input after conversation content is not supported")
				}
				for _, block := range blocks {
					system = append(system, block.Text)
				}
				continue
			}
		case "function_call":
			var arguments map[string]json.RawMessage
			if item.CallID == "" || item.Name == "" || json.Unmarshal([]byte(item.Arguments), &arguments) != nil || arguments == nil {
				return nil, fmt.Errorf("input[%d] function_call requires call_id, name and JSON object arguments", i)
			}
			role = "assistant"
			blocks = []types.ContentBlock{{Type: "tool_use", ID: item.CallID, Name: item.Name, Input: json.RawMessage(item.Arguments)}}
		case "function_call_output":
			if item.CallID == "" {
				return nil, fmt.Errorf("input[%d] function_call_output requires call_id", i)
			}
			content, err := responsesInputContent(item.Output, false)
			if err != nil {
				return nil, fmt.Errorf("input[%d] tool result supports text only: %w", i, err)
			}
			encoded, _ := json.Marshal(content)
			role = "user"
			blocks = []types.ContentBlock{{Type: "tool_result", ToolUseID: item.CallID, Content: encoded}}
		case "reasoning":
			var content []json.RawMessage
			if item.EncryptedContent != "" || len(item.Content) > 0 && (json.Unmarshal(item.Content, &content) != nil || len(content) > 0) {
				return nil, fmt.Errorf("reasoning encrypted_content/content cannot be replayed by this adapter")
			}
			role = "assistant"
			for _, summary := range item.Summary {
				if summary.Type != "summary_text" {
					return nil, fmt.Errorf("unsupported reasoning summary type %q", summary.Type)
				}
				blocks = append(blocks, types.ContentBlock{Type: "thinking", Thinking: summary.Text})
			}
		default:
			return nil, fmt.Errorf("input item type %q is not supported", item.Type)
		}
		if len(blocks) == 0 {
			continue
		}
		if last := len(out.Messages) - 1; last >= 0 && out.Messages[last].Role == role {
			blocks = append(out.Messages[last].ContentBlocks(), blocks...)
			out.Messages[last].Content, _ = json.Marshal(blocks)
		} else {
			content, _ := json.Marshal(blocks)
			out.Messages = append(out.Messages, types.Message{Role: role, Content: content})
		}
	}
	if len(out.Messages) == 0 {
		return nil, fmt.Errorf("input must contain conversation content")
	}
	if len(system) > 0 {
		out.System, _ = json.Marshal(strings.Join(system, "\n\n"))
	}
	return out, nil
}

func responsesInputContent(raw json.RawMessage, allowImage bool) ([]types.ContentBlock, error) {
	raw = bytes.TrimSpace(raw)
	var text string
	if len(raw) > 0 && raw[0] == '"' && json.Unmarshal(raw, &text) == nil {
		return []types.ContentBlock{{Type: "text", Text: text}}, nil
	}
	var parts []struct {
		Type        string          `json:"type"`
		Text        string          `json:"text"`
		ImageURL    string          `json:"image_url"`
		FileID      string          `json:"file_id"`
		Detail      string          `json:"detail"`
		Annotations json.RawMessage `json:"annotations"`
		Logprobs    json.RawMessage `json:"logprobs"`
	}
	if err := decodeResponsesInput(raw, &parts); err != nil || parts == nil {
		return nil, fmt.Errorf("content must be text or a content array")
	}
	blocks := make([]types.ContentBlock, 0, len(parts))
	for _, part := range parts {
		switch part.Type {
		case "input_text", "output_text":
			blocks = append(blocks, types.ContentBlock{Type: "text", Text: part.Text})
		case "input_image":
			if !allowImage || part.FileID != "" || part.Detail != "" && part.Detail != "auto" {
				return nil, fmt.Errorf("input_image requires a user image_url with detail:auto; file_id and tool-result images are unsupported")
			}
			source := &types.ImageSource{Type: "url", URL: part.ImageURL}
			if strings.HasPrefix(part.ImageURL, "data:") {
				header, data, ok := strings.Cut(strings.TrimPrefix(part.ImageURL, "data:"), ";base64,")
				if !ok || !strings.HasPrefix(header, "image/") || data == "" {
					return nil, fmt.Errorf("image_url must be an image base64 data URL")
				}
				if _, err := base64.StdEncoding.DecodeString(data); err != nil {
					return nil, fmt.Errorf("image_url contains invalid base64")
				}
				source = &types.ImageSource{Type: "base64", MediaType: header, Data: data}
			} else {
				u, err := url.Parse(part.ImageURL)
				if err != nil || u.Host == "" || u.Scheme != "https" && u.Scheme != "http" {
					return nil, fmt.Errorf("image_url must use http or https")
				}
			}
			blocks = append(blocks, types.ContentBlock{Type: "image", Source: source})
		default:
			return nil, fmt.Errorf("content type %q is not supported", part.Type)
		}
	}
	return blocks, nil
}

func responsesUsage(usage types.Usage) map[string]any {
	input := usage.InputTokens + usage.CacheReadInputTokens + usage.CacheCreationInputTokens
	return map[string]any{
		"input_tokens": input, "output_tokens": usage.OutputTokens, "total_tokens": input + usage.OutputTokens,
		"input_tokens_details": map[string]int{"cached_tokens": usage.CacheReadInputTokens, "cache_write_tokens": usage.CacheCreationInputTokens},
	}
}

func newInboundResponse(model string) map[string]any {
	return map[string]any{
		"id": "resp_" + generateID(), "object": "response", "created_at": time.Now().Unix(),
		"status": "in_progress", "model": model, "output": []map[string]any{}, "usage": nil,
		"error": nil, "incomplete_details": nil, "store": false, "previous_response_id": nil,
	}
}

func finishInboundResponse(response map[string]any, stopReason string) error {
	switch stopReason {
	case "end_turn", "stop_sequence", "tool_use":
		response["status"] = "completed"
	case "max_tokens", "refusal":
		reason := "max_output_tokens"
		if stopReason == "refusal" {
			reason = "content_filter"
		}
		response["status"] = "incomplete"
		response["incomplete_details"] = map[string]string{"reason": reason}
	default:
		return fmt.Errorf("unsupported or missing upstream stop_reason %q", stopReason)
	}
	return nil
}

func inboundResponseItem(block types.ContentBlock) (map[string]any, error) {
	switch block.Type {
	case "text":
		return map[string]any{"id": "msg_" + generateID(), "type": "message", "role": "assistant", "status": "in_progress", "content": []map[string]any{{"type": "output_text", "text": block.Text, "annotations": []any{}}}}, nil
	case "tool_use":
		if block.ID == "" || block.Name == "" {
			return nil, fmt.Errorf("upstream tool_use requires id and name")
		}
		arguments := string(block.Input)
		if arguments == "" {
			arguments = "{}"
		}
		var object map[string]json.RawMessage
		if json.Unmarshal([]byte(arguments), &object) != nil || object == nil {
			return nil, fmt.Errorf("upstream tool arguments are not a JSON object")
		}
		return map[string]any{"id": "fc_" + generateID(), "type": "function_call", "call_id": block.ID, "name": block.Name, "arguments": arguments, "status": "in_progress"}, nil
	case "thinking":
		if block.Signature != "" && block.Signature != thinkingSignaturePlaceholder {
			return nil, fmt.Errorf("signed Anthropic thinking cannot be replayed through this Responses adapter")
		}
		return map[string]any{"id": "rs_" + generateID(), "type": "reasoning", "status": "in_progress", "summary": []map[string]any{{"type": "summary_text", "text": block.Thinking}}}, nil
	default:
		return nil, fmt.Errorf("upstream content block %q has no supported Responses mapping", block.Type)
	}
}

// AnthropicMessageToResponse converts non-streaming output without changing the
// accounting path. Unknown reasoning-token breakdowns are intentionally omitted.
func AnthropicMessageToResponse(body []byte, model string) (map[string]any, error) {
	var msg struct {
		types.MessageResponse
		Usage *types.Usage `json:"usage"`
	}
	if err := json.Unmarshal(body, &msg); err != nil || msg.Type != "message" {
		return nil, fmt.Errorf("invalid upstream Messages response")
	}
	if msg.Model != "" {
		model = msg.Model
	}
	response := newInboundResponse(model)
	output := []map[string]any{}
	for _, block := range msg.Content {
		item, err := inboundResponseItem(block)
		if err != nil {
			return nil, err
		}
		item["status"] = "completed"
		output = append(output, item)
	}
	response["output"] = output
	if msg.Usage != nil {
		response["usage"] = responsesUsage(*msg.Usage)
	}
	if err := finishInboundResponse(response, msg.StopReason); err != nil {
		return nil, err
	}
	return response, nil
}

type inboundResponseBlock struct {
	outputIndex int
	item        map[string]any
	text        strings.Builder
	arguments   strings.Builder
	initialJSON string
	closed      bool
}

// ResponsesStreamWriter converts complete Anthropic SSE events as soon as their
// bytes arrive. It does not buffer a complete response before emitting deltas.
type ResponsesStreamWriter struct {
	out       io.Writer
	response  map[string]any
	output    []map[string]any
	blocks    map[int]*inboundResponseBlock
	usage     types.Usage
	usageSeen bool
	sequence  int
	line      []byte
	data      []byte
	eventName string
	stop      string
	started   bool
	ended     bool
	err       error
}

func NewResponsesStreamWriter(out io.Writer, model string) *ResponsesStreamWriter {
	return &ResponsesStreamWriter{out: out, response: newInboundResponse(model), output: []map[string]any{}, blocks: make(map[int]*inboundResponseBlock)}
}

func (w *ResponsesStreamWriter) emit(kind string, fields map[string]any) error {
	fields["type"], fields["sequence_number"] = kind, w.sequence
	w.sequence++
	data, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w.out, "event: %s\ndata: %s\n\n", kind, data)
	return err
}

func (w *ResponsesStreamWriter) fail(err error) error {
	if w.err == nil {
		w.err = err
	}
	if !w.ended {
		w.ended = true
		w.response["status"] = "failed"
		w.response["output"] = w.output
		if w.usageSeen {
			w.response["usage"] = responsesUsage(w.usage)
		}
		w.response["error"] = map[string]string{"code": "server_error", "message": err.Error()}
		w.err = errors.Join(w.err, w.emit("response.failed", map[string]any{"response": w.response}))
	}
	return w.err
}

func (w *ResponsesStreamWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	for _, b := range p {
		if b != '\n' {
			w.line = append(w.line, b)
			continue
		}
		line := strings.TrimSuffix(string(w.line), "\r")
		w.line = w.line[:0]
		switch {
		case strings.HasPrefix(line, "event:"):
			w.eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			w.data = append(w.data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " ")...)
			w.data = append(w.data, '\n')
		case strings.HasPrefix(line, ":"):
			if _, err := fmt.Fprintln(w.out, line+"\n"); err != nil {
				return 0, w.fail(err)
			}
		case line == "":
			if len(w.data) > 0 {
				if err := w.event(w.data); err != nil {
					return 0, w.fail(err)
				}
			}
			w.data = w.data[:0]
			w.eventName = ""
		}
	}
	return len(p), nil
}

func (w *ResponsesStreamWriter) event(data []byte) error {
	var event struct {
		Type    string `json:"type"`
		Message *struct {
			Model string       `json:"model"`
			Usage *types.Usage `json:"usage"`
		} `json:"message"`
		Index        *int                `json:"index"`
		ContentBlock *types.ContentBlock `json:"content_block"`
		Delta        *types.Delta        `json:"delta"`
		Usage        map[string]int      `json:"usage"`
		Error        *types.APIError     `json:"error"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		return fmt.Errorf("invalid upstream SSE JSON: %w", err)
	}
	if event.Type == "" {
		event.Type = w.eventName
	}
	if event.Type == "ping" {
		_, err := io.WriteString(w.out, ":keepalive\n\n")
		return err
	}
	if w.ended {
		return fmt.Errorf("upstream event after message_stop")
	}
	if event.Type == "error" {
		if event.Error == nil {
			return fmt.Errorf("upstream error without details")
		}
		return fmt.Errorf("upstream %s: %s", event.Error.Type, event.Error.Message)
	}
	if event.Type == "message_start" {
		if w.started || event.Message == nil {
			return fmt.Errorf("invalid upstream message_start")
		}
		w.started = true
		if event.Message.Model != "" {
			w.response["model"] = event.Message.Model
		}
		if event.Message.Usage != nil {
			w.usage, w.usageSeen = *event.Message.Usage, true
		}
		if err := w.emit("response.created", map[string]any{"response": w.response}); err != nil {
			return err
		}
		return w.emit("response.in_progress", map[string]any{"response": w.response})
	}
	if !w.started {
		return fmt.Errorf("upstream SSE event before message_start")
	}
	switch event.Type {
	case "content_block_start":
		if event.Index == nil || *event.Index < 0 || event.ContentBlock == nil || w.blocks[*event.Index] != nil {
			return fmt.Errorf("invalid upstream content_block_start")
		}
		item, err := inboundResponseItem(*event.ContentBlock)
		if err != nil {
			return err
		}
		block := &inboundResponseBlock{item: item, outputIndex: len(w.output)}
		w.blocks[*event.Index] = block
		w.output = append(w.output, item)
		kind := item["type"].(string)
		if kind == "function_call" {
			block.initialJSON = item["arguments"].(string)
			item["arguments"] = ""
		} else if kind == "message" {
			item["content"] = []map[string]any{}
		} else {
			item["summary"] = []map[string]any{}
		}
		if err := w.emit("response.output_item.added", map[string]any{"output_index": block.outputIndex, "item": item}); err != nil {
			return err
		}
		if kind == "function_call" {
			return nil
		}
		part := map[string]any{"type": "output_text", "text": "", "annotations": []any{}}
		partEvent, indexName := "response.content_part.added", "content_index"
		if kind == "reasoning" {
			part = map[string]any{"type": "summary_text", "text": ""}
			partEvent, indexName = "response.reasoning_summary_part.added", "summary_index"
		}
		if err := w.emit(partEvent, map[string]any{"item_id": item["id"], "output_index": block.outputIndex, indexName: 0, "part": part}); err != nil {
			return err
		}
		text := event.ContentBlock.Text
		if kind == "reasoning" {
			text = event.ContentBlock.Thinking
		}
		return w.textDelta(block, text)
	case "content_block_delta", "content_block_stop":
		if event.Index == nil || w.blocks[*event.Index] == nil || w.blocks[*event.Index].closed {
			return fmt.Errorf("upstream content event has no open block")
		}
		block := w.blocks[*event.Index]
		if event.Type == "content_block_stop" {
			return w.closeBlock(block)
		}
		if event.Delta == nil {
			return fmt.Errorf("upstream content_block_delta has no delta")
		}
		switch event.Delta.Type {
		case "text_delta":
			if block.item["type"] != "message" {
				return fmt.Errorf("text delta on non-message block")
			}
			return w.textDelta(block, event.Delta.Text)
		case "thinking_delta":
			if block.item["type"] != "reasoning" {
				return fmt.Errorf("thinking delta on non-reasoning block")
			}
			return w.textDelta(block, event.Delta.Thinking)
		case "signature_delta":
			if block.item["type"] != "reasoning" || event.Delta.Signature != "" && event.Delta.Signature != thinkingSignaturePlaceholder {
				return fmt.Errorf("signed Anthropic thinking cannot be replayed through this Responses adapter")
			}
			return nil
		case "input_json_delta":
			if block.item["type"] != "function_call" {
				return fmt.Errorf("argument delta on non-function block")
			}
			block.arguments.WriteString(event.Delta.PartialJSON)
			return w.emit("response.function_call_arguments.delta", map[string]any{"item_id": block.item["id"], "output_index": block.outputIndex, "delta": event.Delta.PartialJSON})
		default:
			return fmt.Errorf("unsupported upstream delta %q", event.Delta.Type)
		}
	case "message_delta":
		if event.Delta != nil && event.Delta.StopReason != "" {
			w.stop = event.Delta.StopReason
		}
		for field, value := range event.Usage {
			switch field {
			case "input_tokens":
				w.usage.InputTokens = value
			case "output_tokens":
				w.usage.OutputTokens = value
			case "cache_read_input_tokens":
				w.usage.CacheReadInputTokens = value
			case "cache_creation_input_tokens":
				w.usage.CacheCreationInputTokens = value
			default:
				continue
			}
			w.usageSeen = true
		}
		return nil
	case "message_stop":
		for _, block := range w.blocks {
			if !block.closed {
				return fmt.Errorf("upstream message_stop with unfinished content")
			}
		}
		if err := finishInboundResponse(w.response, w.stop); err != nil {
			return err
		}
		w.response["output"] = w.output
		if w.usageSeen {
			w.response["usage"] = responsesUsage(w.usage)
		}
		if err := w.emit("response."+w.response["status"].(string), map[string]any{"response": w.response}); err != nil {
			return err
		}
		w.ended = true
		return nil
	default:
		return fmt.Errorf("unsupported upstream SSE event %q", event.Type)
	}
}

func (w *ResponsesStreamWriter) textDelta(block *inboundResponseBlock, text string) error {
	if text == "" {
		return nil
	}
	block.text.WriteString(text)
	kind, indexName := "response.output_text.delta", "content_index"
	if block.item["type"] == "reasoning" {
		kind, indexName = "response.reasoning_summary_text.delta", "summary_index"
	}
	return w.emit(kind, map[string]any{"item_id": block.item["id"], "output_index": block.outputIndex, indexName: 0, "delta": text})
}

func (w *ResponsesStreamWriter) closeBlock(block *inboundResponseBlock) error {
	item := block.item
	if item["type"] == "function_call" {
		arguments := block.arguments.String()
		if arguments == "" {
			arguments = block.initialJSON
		}
		var object map[string]json.RawMessage
		if json.Unmarshal([]byte(arguments), &object) != nil || object == nil {
			return fmt.Errorf("upstream tool arguments are not a JSON object")
		}
		item["arguments"] = arguments
		if err := w.emit("response.function_call_arguments.done", map[string]any{"item_id": item["id"], "output_index": block.outputIndex, "arguments": arguments}); err != nil {
			return err
		}
	} else {
		text := block.text.String()
		part := map[string]any{"type": "output_text", "text": text, "annotations": []any{}}
		textEvent, partEvent, indexName := "response.output_text.done", "response.content_part.done", "content_index"
		if item["type"] == "reasoning" {
			part = map[string]any{"type": "summary_text", "text": text}
			textEvent, partEvent, indexName = "response.reasoning_summary_text.done", "response.reasoning_summary_part.done", "summary_index"
			item["summary"] = []map[string]any{part}
		} else {
			item["content"] = []map[string]any{part}
		}
		if err := w.emit(textEvent, map[string]any{"item_id": item["id"], "output_index": block.outputIndex, indexName: 0, "text": text}); err != nil {
			return err
		}
		if err := w.emit(partEvent, map[string]any{"item_id": item["id"], "output_index": block.outputIndex, indexName: 0, "part": part}); err != nil {
			return err
		}
	}
	item["status"] = "completed"
	block.closed = true
	return w.emit("response.output_item.done", map[string]any{"output_index": block.outputIndex, "item": item})
}

// Finish must be called at EOF. A missing terminal event is a failed response,
// never an invented successful completion.
func (w *ResponsesStreamWriter) Finish() error {
	if w.err != nil {
		return w.err
	}
	if !w.ended || len(w.data) > 0 || len(bytes.TrimSpace(w.line)) > 0 {
		return w.fail(fmt.Errorf("upstream stream ended without a complete message_stop"))
	}
	return nil
}

package metrics

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/routatic/proxy/internal/core"
)

// Like the existing stream readers, observation is bounded to 1 MiB. Invalid
// samples are discarded; forwarding and its error classification stay untouched.
func (b *generationBody) observe(chunk []byte, at time.Time) {
	for _, c := range chunk {
		if b.skipLF && c == '\n' {
			b.skipLF = false
			continue
		}
		b.skipLF = c == '\r'
		b.frameBytes++
		if b.frameBytes > 1<<20 {
			b.invalid = true
		}
		if b.invalid || b.terminal {
			b.line, b.data = nil, nil
			return
		}
		if c != '\n' && c != '\r' {
			b.line = append(b.line, c)
			continue
		}
		if len(b.line) == 0 {
			b.observeEvent(at)
			b.data = b.data[:0]
			b.event = ""
			b.frameBytes = 0
		} else if field, value, ok := bytes.Cut(b.line, []byte(":")); ok {
			value = bytes.TrimPrefix(value, []byte(" "))
			switch string(field) {
			case "event":
				b.event = string(value)
			case "data":
				b.data = append(b.data, value...)
				b.data = append(b.data, '\n')
			}
		}
		b.line = b.line[:0]
	}
}

type generationUsage struct {
	Output     *int64 `json:"output_tokens"`
	Completion *int64 `json:"completion_tokens"`
}

type generationEvent struct {
	Type         string
	Error        json.RawMessage
	Delta        json.RawMessage
	ContentBlock *struct{ Type, Text, Thinking, Name string } `json:"content_block"`
	Item         *struct{ Type, Name, Arguments, Input string }
	Usage        *generationUsage
	Response     *struct {
		Status string
		Usage  *generationUsage
	}
	Choices []struct {
		Delta struct {
			Content          string
			Refusal          string
			ReasoningContent string `json:"reasoning_content"`
			Reasoning        string `json:"reasoning"`
			ToolCalls        []struct {
				Function struct{ Name, Arguments string }
			} `json:"tool_calls"`
		}
	}
}

func (b *generationBody) observeEvent(at time.Time) {
	data := bytes.TrimSpace(b.data)
	if len(data) == 0 {
		return
	}
	if bytes.Equal(data, []byte("[DONE]")) {
		if b.observation.format == core.WireFormatOpenAIChat {
			b.complete(at)
		}
		return
	}
	var event generationEvent
	if data[0] != '{' || json.Unmarshal(data, &event) != nil || (b.event != "" && event.Type != "" && b.event != event.Type) {
		b.invalid = true
		return
	}
	if event.Type == "" {
		event.Type = b.event
	}
	if event.Type == "error" || event.Type == "response.failed" || event.Type == "response.incomplete" ||
		(len(event.Error) > 0 && !bytes.Equal(event.Error, []byte("null"))) {
		b.invalid = true
		return
	}
	output, terminal := false, false
	switch b.observation.format {
	case core.WireFormatOpenAIChat:
		for _, choice := range event.Choices {
			d := choice.Delta
			output = output || d.Content != "" || d.Refusal != "" || d.ReasoningContent != "" || d.Reasoning != ""
			for _, tool := range d.ToolCalls {
				output = output || tool.Function.Name != "" || tool.Function.Arguments != ""
			}
		}
		if event.Choices != nil && event.Usage != nil {
			b.mergeOutput(event.Usage.Completion)
		}
	case core.WireFormatAnthropic:
		switch event.Type {
		case "content_block_start":
			if block := event.ContentBlock; block != nil {
				output = block.Type == "text" && block.Text != "" || block.Type == "thinking" && block.Thinking != "" || block.Type == "tool_use" && block.Name != ""
			}
		case "content_block_delta":
			var delta struct {
				Type, Text, Thinking string
				PartialJSON          string `json:"partial_json"`
			}
			if json.Unmarshal(event.Delta, &delta) != nil {
				b.invalid = true
				return
			}
			output = delta.Type == "text_delta" && delta.Text != "" || delta.Type == "thinking_delta" && delta.Thinking != "" || delta.Type == "input_json_delta" && delta.PartialJSON != ""
		case "message_delta", "message_stop":
			if event.Usage != nil {
				b.mergeOutput(event.Usage.Output)
			}
			terminal = event.Type == "message_stop"
		}
	case core.WireFormatOpenAIResponses:
		switch event.Type {
		case "response.output_text.delta", "response.refusal.delta", "response.reasoning_summary_text.delta", "response.reasoning_text.delta", "response.reasoning.delta", "response.function_call_arguments.delta", "response.custom_tool_call_input.delta":
			var delta string
			if json.Unmarshal(event.Delta, &delta) != nil {
				b.invalid = true
				return
			}
			output = delta != ""
		case "response.output_item.added":
			if item := event.Item; item != nil && (item.Type == "function_call" || item.Type == "custom_tool_call") {
				output = item.Name != "" || item.Arguments != "" || item.Input != ""
			}
		case "response.completed", "response.done":
			usage := event.Usage
			if event.Response != nil {
				if status := event.Response.Status; status != "" && status != "completed" {
					b.invalid = true
					return
				}
				if usage == nil {
					usage = event.Response.Usage
				}
			}
			if usage != nil {
				b.mergeOutput(usage.Output)
			}
			terminal = true
		}
	}
	if output && b.firstOutput.IsZero() {
		b.firstOutput = at
	}
	if terminal {
		b.complete(at)
	}
}

func (b *generationBody) mergeOutput(tokens *int64) {
	if tokens == nil {
		return
	}
	if *tokens < 0 || *tokens > 1<<53-1 {
		b.invalid = true
		return
	}
	// Totals are cumulative. Some terminals repeat zero or omit the field.
	if b.output == nil || *tokens > *b.output {
		b.output = tokens
	}
}

func (b *generationBody) complete(at time.Time) {
	b.terminal = true
	if b.invalid || b.output == nil || b.firstOutput.IsZero() || b.observation.start.IsZero() || b.firstOutput.Before(b.observation.start) {
		return
	}
	sample := &GenerationSample{
		FirstOutputMs: b.firstOutput.Sub(b.observation.start).Milliseconds(),
		DurationMs:    at.Sub(b.firstOutput).Milliseconds(),
		OutputTokens:  *b.output,
	}
	if sample.Valid() {
		b.observation.sample = sample
	}
}

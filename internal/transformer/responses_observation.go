package transformer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/pkg/types"
)

// ResponsesObservation owns source status and cumulative usage independently
// of whether content is forwarded verbatim or translated to Messages.
type ResponsesObservation struct {
	terminal      bool
	stopReason    string
	usage         *types.ResponsesUsage
	Model         string
	ErrorReported bool
	functions     map[string]*responsesFunctionState
}

func (s *ResponsesObservation) Done() bool { return s.terminal }
func (s *ResponsesObservation) ReportedUsage() *types.Usage {
	return ResponsesUsageToAnthropic(s.usage)
}

func ResponsesResponseError(response *types.ResponsesResponse) error {
	stop, err := responsesFinishReason(response.Status, response.IncompleteDetails)
	if err != nil {
		return &core.ResponsesStatusError{Status: response.Status, Usage: ResponsesUsageToAnthropic(response.Usage)}
	}
	if err := upstreamJSONError("Responses", response.Error); err != nil {
		return &core.ResponseError{Err: err, Usage: ResponsesUsageToAnthropic(response.Usage)}
	}
	if stop != "max_tokens" {
		for _, item := range response.Output {
			if item.Type == "function_call" && !json.Valid([]byte(item.Arguments)) {
				return &core.ResponseError{Err: fmt.Errorf("invalid completed Responses function arguments"), Usage: ResponsesUsageToAnthropic(response.Usage)}
			}
		}
	}
	return nil
}

func (s *ResponsesObservation) ReadEvent(event string, data []byte) (*types.ResponsesChunk, error) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || bytes.Equal(data, []byte("[DONE]")) {
		return nil, nil
	}
	previousReasoning := 0
	if s.usage != nil && s.usage.OutputTokensDetails != nil {
		previousReasoning = s.usage.OutputTokensDetails.ReasoningTokens
	}
	chunk := &types.ResponsesChunk{Usage: s.usage, Response: &types.ResponsesResponse{Usage: s.usage}}
	if err := json.Unmarshal(data, chunk); err != nil {
		return nil, fmt.Errorf("invalid Responses SSE: %w", err)
	}
	if chunk.Type == "" {
		chunk.Type = event
	}
	usage := chunk.Usage
	if chunk.Response != nil {
		if chunk.Response.Model != "" {
			s.Model = chunk.Response.Model
		}
		if chunk.Response.Usage != nil {
			usage = chunk.Response.Usage
		}
	}
	if usage != nil {
		if previousReasoning > 0 && (usage.OutputTokensDetails == nil || usage.OutputTokensDetails.ReasoningTokens <= 0) {
			usage.OutputTokensDetails = &types.CompletionTokensDetails{ReasoningTokens: previousReasoning}
		}
		s.usage = usage
	}
	if chunk.Type == "response.failed" || chunk.Type == "response.cancelled" || chunk.Type == "error" {
		s.ErrorReported = true
		return chunk, fmt.Errorf("upstream Responses event %s", chunk.Type)
	}
	if err := upstreamJSONError("Responses", chunk.Error); err != nil {
		s.ErrorReported = true
		return chunk, err
	}
	if chunk.Response != nil {
		if err := upstreamJSONError("Responses", chunk.Response.Error); err != nil {
			s.ErrorReported = true
			return chunk, err
		}
	}
	if chunk.Type == "response.completed" || chunk.Type == "response.done" || chunk.Type == "response.incomplete" {
		status := "completed"
		if chunk.Type == "response.incomplete" {
			status = "incomplete"
		}
		var details *types.ResponsesIncompleteDetails
		if chunk.Response != nil {
			details = chunk.Response.IncompleteDetails
			if chunk.Response.Status != "" {
				if chunk.Type != "response.done" && chunk.Response.Status != status {
					return chunk, fmt.Errorf("Responses terminal event conflicts with response.status")
				}
				status = chunk.Response.Status
			}
		}
		stop, err := responsesFinishReason(status, details)
		if err != nil {
			return chunk, err
		}
		if chunk.Response != nil {
			response := *chunk.Response
			response.Status = status
			if err := ResponsesResponseError(&response); err != nil {
				return chunk, err
			}
			for _, item := range response.Output {
				if item.Type == "function_call" {
					if err := s.functionItem(item, true, stop == "max_tokens"); err != nil {
						return chunk, err
					}
				}
			}
		}
		for _, tool := range s.functions {
			if err := tool.finishArguments(stop == "max_tokens"); err != nil {
				return chunk, err
			}
		}
		s.terminal, s.stopReason = true, stop
		return chunk, nil
	}
	if s.terminal && (strings.HasPrefix(chunk.Type, "response.output_") || strings.HasPrefix(chunk.Type, "response.reasoning_summary_") || strings.HasPrefix(chunk.Type, "response.refusal.") || chunk.Type == "response.function_call_arguments.delta") {
		return chunk, fmt.Errorf("Responses content arrived after terminal event")
	}
	if chunk.Type == "response.output_item.added" || chunk.Type == "response.output_item.done" {
		item := chunk.Item
		if item == nil && len(chunk.Output) > 0 {
			item = &chunk.Output[0]
		}
		if item == nil {
			return chunk, fmt.Errorf("responses output item is missing")
		}
		if item.Type == "function_call" {
			return chunk, s.functionItem(*item, chunk.Type == "response.output_item.done", false)
		}
	}
	if chunk.Type == "response.function_call_arguments.delta" {
		tool := s.functions[chunk.ItemID]
		if tool == nil || tool.closed {
			return chunk, fmt.Errorf("responses arguments reference an unopened function")
		}
		tool.arguments.WriteString(chunk.Delta)
	}
	return chunk, nil
}

func (s *ResponsesObservation) functionItem(item types.ResponsesOutput, done, budget bool) error {
	tool := s.functions[item.ID]
	if tool == nil {
		tool = &responsesFunctionState{}
	}
	delta, err := tool.snapshotDelta(item)
	if err != nil {
		return err
	}
	if prior := s.functions[item.ID]; prior == nil {
		tool.callID, tool.name = item.CallID, item.Name
		if s.functions == nil {
			s.functions = make(map[string]*responsesFunctionState)
		}
		s.functions[item.ID] = tool
	}
	tool.arguments.WriteString(delta)
	if done && !tool.closed {
		if err := tool.finishArguments(budget || item.Status == "incomplete"); err != nil {
			return err
		}
		tool.closed = true
	}
	return nil
}

// Pure source rules are shared with delivery; each owner commits its own state.
type responsesFunctionState struct {
	callID, name string
	arguments    strings.Builder
	closed       bool
}

func (t *responsesFunctionState) snapshotDelta(item types.ResponsesOutput) (string, error) {
	if t.callID == "" {
		if item.ID == "" || item.CallID == "" || item.Name == "" {
			return "", fmt.Errorf("incomplete Responses function identity")
		}
	} else if (item.CallID != "" && item.CallID != t.callID) || (item.Name != "" && item.Name != t.name) {
		return "", fmt.Errorf("Responses function identity changed after streaming")
	}
	arguments := t.arguments.String()
	if item.Arguments == "" || item.Arguments == arguments {
		return "", nil
	}
	if t.closed || !strings.HasPrefix(item.Arguments, arguments) {
		return "", fmt.Errorf("responses function arguments changed after streaming")
	}
	return item.Arguments[len(arguments):], nil
}

func (t *responsesFunctionState) finishArguments(budget bool) error {
	if !budget && !json.Valid([]byte(t.arguments.String())) {
		return fmt.Errorf("invalid completed Responses function arguments")
	}
	return nil
}

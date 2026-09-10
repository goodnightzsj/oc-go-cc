package transformer

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/pkg/types"
)

func TestResponsesRequestPreservesStructuredHistory(t *testing.T) {
	req := &types.MessageRequest{
		Model: "alias", MaxTokens: 77, System: json.RawMessage(`"instructions"`),
		ToolChoice:   json.RawMessage(`{"type":"tool","name":"lookup","disable_parallel_tool_use":true}`),
		OutputConfig: json.RawMessage(`{"effort":"high"}`),
		Tools:        []types.Tool{{Name: "lookup", InputSchema: json.RawMessage(`{"type":"object","properties":{}}`)}},
		Messages: []types.Message{
			{Role: "assistant", Content: json.RawMessage(`[{"type":"text","text":"before"},{"type":"tool_use","id":"call_1","name":"lookup","input":{"q":"hi"}},{"type":"text","text":"after"}]`)},
			{Role: "user", Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"call_1","content":"result"},{"type":"text","text":"caption"},{"type":"image","source":{"type":"url","url":"https://example.invalid/image.png"}}]`)},
		},
	}
	got, err := AnthropicToResponses(req, config.ModelConfig{ModelID: "synthetic", Vision: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Input) != 7 || got.Input[2].Type != "function_call" || got.Input[2].CallID != "call_1" || got.Input[4].Type != "function_call_output" || got.Input[4].CallID != "call_1" {
		t.Fatalf("tool chronology was lost: %+v", got.Input)
	}
	if string(got.Input[1].Content) != `"before"` || string(got.Input[3].Content) != `"after"` || string(got.Input[4].Output) != `"result"` || !strings.Contains(string(got.Input[6].Content), "image_url") {
		t.Fatalf("content changed: %+v", got.Input)
	}
	if got.MaxOutputTokens != 77 || got.Reasoning == nil || got.Reasoning.Effort != "high" || got.ParallelToolCalls == nil || *got.ParallelToolCalls || string(got.ToolChoice) != `{"name":"lookup","type":"function"}` || len(got.Tools) != 1 {
		t.Fatalf("request constraints lost: %+v", got)
	}
}

func TestNormalizedResponsesPreservesCacheAndToolStop(t *testing.T) {
	var response types.ResponsesResponse
	if err := json.Unmarshal([]byte(`{"id":"resp_1","status":"completed","output":[{"type":"function_call","call_id":"call_1","name":"lookup","arguments":"{}"}],"usage":{"input_tokens":101,"output_tokens":7,"input_tokens_details":{"cached_tokens":90}}}`), &response); err != nil {
		t.Fatal(err)
	}
	got := core.DenormalizeResponse(ResponsesToNormalized(&response, "synthetic"))
	if got.StopReason != "tool_use" || len(got.Content) != 1 || got.Content[0].ID != "call_1" || got.Usage.InputTokens != 11 || got.Usage.CacheReadInputTokens != 90 || got.Usage.OutputTokens != 7 {
		t.Fatalf("Responses conversion = %+v", got)
	}
	response.IncompleteDetails = &types.ResponsesIncompleteDetails{Reason: "max_output_tokens"}
	if got := ResponsesToNormalized(&response, "synthetic"); got.StopReason != "max_tokens" {
		t.Fatalf("incomplete treated as complete: %+v", got)
	}
}

func TestNormalizedChatUsesSharedCacheSplit(t *testing.T) {
	for _, usage := range []string{
		`{"prompt_tokens":101,"completion_tokens":7,"prompt_tokens_details":{"cached_tokens":90}}`,
		`{"prompt_tokens":101,"completion_tokens":7,"prompt_cache_hit_tokens":90,"prompt_cache_miss_tokens":11}`,
		`{"prompt_tokens":101,"completion_tokens":7,"prompt_cache_hit_tokens":90,"prompt_cache_miss_tokens":101}`,
		`{"prompt_tokens":101,"completion_tokens":7,"prompt_cache_hit_tokens":90,"prompt_tokens_details":{"cached_tokens":90}}`,
	} {
		var response types.ChatCompletionResponse
		if err := json.Unmarshal([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":`+usage+`}`), &response); err != nil {
			t.Fatal(err)
		}
		got := core.DenormalizeResponse(OpenAIResponseToNormalized(&response, "synthetic"))
		if got.Usage.InputTokens != 11 || got.Usage.CacheReadInputTokens != 90 || got.Usage.CacheCreationInputTokens != 0 || got.Usage.OutputTokens != 7 {
			t.Fatalf("usage=%+v source=%s", got.Usage, usage)
		}
	}
}

type responsesUsageRecorder struct {
	*httptest.ResponseRecorder
	usage [4]int
}

func (w *responsesUsageRecorder) SetPartialUsage(in, out, read, create int) {
	w.usage = [4]int{in, out, read, create}
}

func TestResponsesUpstreamStreamToolsOrderingAndUsage(t *testing.T) {
	for _, terminal := range []string{
		`{"type":"response.completed","response":{"usage":{"input_tokens":101,"output_tokens":7,"input_tokens_details":{"cached_tokens":90}}}}`,
		`{"type":"response.done","usage":{"input_tokens":101,"output_tokens":7,"input_tokens_details":{"cached_tokens":90}}}`,
	} {
		body := sseLines(
			`{"type":"response.output_text.delta","delta":"before"}`,
			`{"type":"response.output_item.added","item":{"type":"function_call","id":"item_1","call_id":"call_1","name":"lookup","arguments":""}}`,
			`{"type":"response.output_item.added","output":[{"type":"function_call","id":"item_2","call_id":"call_2","name":"lookup","arguments":""}]}`,
			`{"type":"response.function_call_arguments.delta","item_id":"item_1","delta":"{\"q\":"}`,
			`{"type":"response.function_call_arguments.delta","item_id":"item_2","delta":"{}"}`,
			`{"type":"response.function_call_arguments.delta","item_id":"item_1","delta":"\"line\\ntext\"}"}`,
			`{"type":"response.output_item.done","item":{"type":"function_call","id":"item_2","call_id":"call_2","name":"lookup","arguments":"{}"}}`,
			`{"type":"response.output_item.done","item":{"type":"function_call","id":"item_1","call_id":"call_1","name":"lookup","arguments":"{\"q\":\"line\\ntext\"}"}}`,
			`{"type":"response.output_text.delta","delta":"after"}`, terminal,
		)
		ctx, cancel := context.WithCancel(context.Background())
		w := &responsesUsageRecorder{ResponseRecorder: httptest.NewRecorder()}
		err := NewStreamHandler().ProxyResponsesStream(w, io.NopCloser(iotest.OneByteReader(body)), "synthetic", ctx, 0, cancel)
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		seen := map[int]bool{}
		args := map[int]string{}
		stops, deltas := 0, 0
		for _, event := range parseSSEEvents(t, w.Body.String()) {
			switch event.Type {
			case "content_block_start":
				if seen[*event.Index] {
					t.Fatalf("duplicate block index: %d", *event.Index)
				}
				seen[*event.Index] = true
			case "content_block_delta":
				if event.Delta.Type == "input_json_delta" {
					args[*event.Index] += event.Delta.PartialJSON
				}
			case "message_delta":
				deltas++
				if event.Delta.StopReason != "tool_use" || event.Usage.InputTokens != 11 || event.Usage.CacheReadInputTokens != 90 || event.Usage.OutputTokens != 7 {
					t.Fatalf("terminal=%+v", event)
				}
			case "message_stop":
				stops++
			}
		}
		if len(seen) != 4 || deltas != 1 || stops != 1 || args[1] != `{"q":"line\ntext"}` || args[2] != "{}" || w.usage != [4]int{11, 7, 90, 0} {
			t.Fatalf("stream contract: starts=%v args=%v deltas=%d stops=%d usage=%v", seen, args, deltas, stops, w.usage)
		}
	}
}

func TestResponsesUpstreamFailuresNeverSynthesizeCompletion(t *testing.T) {
	for _, tail := range []string{
		`{"type":"response.output_text.delta","delta":"partial"}`,
		`{"type":"response.failed","response":{"usage":{"input_tokens":101,"output_tokens":7,"input_tokens_details":{"cached_tokens":90}}}}`,
		`{"type":"response.function_call_arguments.delta","item_id":"missing","delta":"{}"}`,
		`{invalid`,
	} {
		ctx, cancel := context.WithCancel(context.Background())
		w := &responsesUsageRecorder{ResponseRecorder: httptest.NewRecorder()}
		err := NewStreamHandler().ProxyResponsesStream(w, sseLines(tail), "synthetic", ctx, 0, cancel)
		cancel()
		if err == nil || strings.Contains(w.Body.String(), "message_stop") {
			t.Fatalf("failure became success: err=%v body=%s", err, w.Body.String())
		}
		if strings.Contains(tail, "response.failed") && w.usage != [4]int{11, 7, 90, 0} {
			t.Fatalf("failure lost known usage: %v", w.usage)
		}
	}
}

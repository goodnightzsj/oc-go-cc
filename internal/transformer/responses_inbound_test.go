package transformer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/routatic/proxy/pkg/types"
)

func TestResponsesInboundPreservesConversationAndParameters(t *testing.T) {
	raw := []byte(`{"model":"synthetic","store":false,"stream":true,"max_output_tokens":1234,"temperature":0.2,"top_p":0.9,
		"instructions":"first","reasoning":{"effort":"high"},"parallel_tool_calls":false,"tool_choice":{"type":"function","name":"lookup"},
		"tools":[{"type":"function","name":"lookup","strict":false,"parameters":{"type":"object","properties":{"q":{"type":"string"}}}}],
		"input":[{"role":"developer","content":"second"},{"role":"user","content":[{"type":"input_text","text":"before"},{"type":"input_image","image_url":"https://example.invalid/test.png"},{"type":"input_text","text":"after"}]},
		{"type":"function_call","call_id":"call_1","name":"lookup","arguments":"{\"q\":\"one\"}"},
		{"type":"function_call","call_id":"call_2","name":"lookup","arguments":"{\"q\":\"two\"}"},
		{"type":"function_call_output","call_id":"call_1","output":"one"},
		{"type":"function_call_output","call_id":"call_2","output":[{"type":"input_text","text":"two"}]},
		{"role":"assistant","content":[{"type":"output_text","text":"done","annotations":[]}]}]}`)
	req, err := ResponsesToMessageRequest(raw)
	if err != nil {
		t.Fatal(err)
	}
	if req.Model != "synthetic" || req.MaxTokens != 1234 || req.Stream == nil || !*req.Stream || *req.Temperature != 0.2 || *req.TopP != 0.9 {
		t.Fatalf("parameters = %+v", req)
	}
	if req.SystemText() != "first\n\nsecond" || len(req.Messages) != 4 || len(req.Tools) != 1 {
		t.Fatalf("system/messages/tools = %q / %+v / %+v", req.SystemText(), req.Messages, req.Tools)
	}
	blocks := req.Messages[0].ContentBlocks()
	if len(blocks) != 3 || blocks[0].Text != "before" || blocks[1].Source.URL != "https://example.invalid/test.png" || blocks[2].Text != "after" {
		t.Fatalf("image order or source lost: %+v", blocks)
	}
	tools := req.Messages[1].ContentBlocks()
	results := req.Messages[2].ContentBlocks()
	if len(tools) != 2 || tools[0].ID != "call_1" || tools[1].ID != "call_2" || string(tools[1].Input) != `{"q":"two"}` || len(results) != 2 || results[1].ToolUseID != "call_2" || results[1].TextContent() != "two" {
		t.Fatalf("function round trip lost: tools=%+v results=%+v", tools, results)
	}
	encoded, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(encoded, &fields)
	var choice map[string]any
	_ = json.Unmarshal(fields["tool_choice"], &choice)
	if choice["type"] != "tool" || choice["name"] != "lookup" || choice["disable_parallel_tool_use"] != true || string(fields["output_config"]) != `{"effort":"high"}` {
		t.Fatalf("tool/effort parameters lost: %s", encoded)
	}
}

func TestResponsesInboundStringAndDataImage(t *testing.T) {
	req, err := ResponsesToMessageRequest([]byte(`{"model":"m","input":"hello"}`))
	if err != nil || len(req.Messages) != 1 || req.Messages[0].ContentBlocks()[0].Text != "hello" {
		t.Fatalf("string input: req=%+v err=%v", req, err)
	}
	req, err = ResponsesToMessageRequest([]byte(`{"model":"m","input":[{"role":"user","content":[{"type":"input_image","image_url":"data:image/png;base64,aW1hZ2U="}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	source := req.Messages[0].ContentBlocks()[0].Source
	if source.Type != "base64" || source.MediaType != "image/png" || source.Data != "aW1hZ2U=" {
		t.Fatalf("data URL = %+v", source)
	}
}

func TestResponsesReasoningRoundTripAndUnknownUsage(t *testing.T) {
	response, err := AnthropicMessageToResponse([]byte(`{"type":"message","model":"m","stop_reason":"end_turn","content":[{"type":"thinking","thinking":"synthetic reasoning"},{"type":"text","text":"answer"}]}`), "m")
	if err != nil {
		t.Fatal(err)
	}
	if response["usage"] != nil {
		t.Fatalf("missing usage became zero: %+v", response["usage"])
	}
	input, _ := json.Marshal(map[string]any{"model": "m", "input": response["output"]})
	req, err := ResponsesToMessageRequest(input)
	if err != nil {
		t.Fatal(err)
	}
	blocks := req.Messages[0].ContentBlocks()
	if len(blocks) != 2 || blocks[0].Type != "thinking" || blocks[0].Thinking != "synthetic reasoning" || blocks[1].Text != "answer" {
		t.Fatalf("reasoning/text lost: %+v", blocks)
	}
	if _, err := ResponsesToMessageRequest([]byte(`{"model":"m","input":[{"type":"reasoning","content":[ ],"summary":[{"type":"summary_text","text":"synthetic"}]}]}`)); err != nil {
		t.Fatalf("empty reasoning content rejected because of whitespace: %v", err)
	}
	if _, err := AnthropicMessageToResponse([]byte(`{"type":"message","model":"m","stop_reason":"end_turn","content":[{"type":"thinking","thinking":"synthetic","signature":"synthetic-opaque-signature"}]}`), "m"); err == nil {
		t.Fatal("signed thinking was silently stripped")
	}
	var out bytes.Buffer
	w := NewResponsesStreamWriter(&out, "m")
	_ = w.Finish()
	if !strings.Contains(out.String(), `"usage":null`) {
		t.Fatalf("failed request invented zero usage: %s", out.String())
	}
}

func TestResponsesInboundRejectsUnsupportedOrInvalidInput(t *testing.T) {
	cases := []struct{ name, raw, want string }{
		{"custom tool", `{"model":"m","input":"hi","tools":[{"type":"custom","name":"apply_patch"}]}`, "custom"},
		{"built in tool", `{"model":"m","input":"hi","tools":[{"type":"web_search"}]}`, "web_search"},
		{"strict tool", `{"model":"m","input":"hi","tools":[{"type":"function","name":"f","strict":true,"parameters":{"type":"object"}}]}`, "strict"},
		{"unspecified strict", `{"model":"m","input":"hi","tools":[{"type":"function","name":"f","parameters":{"type":"object"}}]}`, "strict"},
		{"required without tools", `{"model":"m","input":"hi","tool_choice":"required"}`, "tool_choice"},
		{"undeclared tool", `{"model":"m","input":"hi","tool_choice":{"type":"function","name":"missing"}}`, "tool_choice"},
		{"stateful", `{"model":"m","input":"hi","previous_response_id":"resp_old"}`, "previous_response_id"},
		{"store", `{"model":"m","input":"hi","store":true}`, "store"},
		{"background", `{"model":"m","input":"hi","background":true}`, "background"},
		{"encrypted reasoning", `{"model":"m","input":[{"type":"reasoning","encrypted_content":"opaque","summary":[]}]}`, "encrypted_content"},
		{"image result", `{"model":"m","input":[{"type":"function_call_output","call_id":"c","output":[{"type":"input_image","image_url":"https://example.invalid/a.png"}]}]}`, "tool result"},
		{"reference", `{"model":"m","input":[{"type":"item_reference","id":"msg_old"}]}`, "item_reference"},
		{"file", `{"model":"m","input":[{"role":"user","content":[{"type":"input_file","file_id":"file_1"}]}]}`, "input_file"},
		{"arguments", `{"model":"m","input":[{"type":"function_call","call_id":"c","name":"f","arguments":"oops"}]}`, "arguments"},
		{"image url", `{"model":"m","input":[{"role":"user","content":[{"type":"input_image","image_url":"file:///private/a.png"}]}]}`, "image_url"},
		{"reasoning effort", `{"model":"m","input":"hi","reasoning":{"effort":"xhigh"}}`, "effort"},
		{"summary", `{"model":"m","input":"hi","reasoning":{"summary":"detailed"}}`, "summary"},
		{"structured output", `{"model":"m","input":"hi","text":{"format":{"type":"json_schema","schema":{}}}}`, "text"},
		{"unknown field", `{"model":"m","input":"hi","unsupported_feature":true}`, "unsupported_feature"},
		{"missing model", `{"input":"hi"}`, "model"},
		{"invalid input", `{"model":"m","input":42}`, "input"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ResponsesToMessageRequest([]byte(tt.raw)); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err=%v, want %q", err, tt.want)
			}
		})
	}
}

func responsesSSE(event any) []byte {
	data, err := json.Marshal(event)
	if err != nil {
		panic(err)
	}
	return append(append([]byte("data: "), data...), '\n', '\n')
}

func TestResponsesOutboundIncrementalTextToolsAndUsage(t *testing.T) {
	var out bytes.Buffer
	w := NewResponsesStreamWriter(&out, "requested")
	events := []string{
		`{"type":"message_start","message":{"id":"upstream","model":"actual","usage":{"input_tokens":11,"cache_read_input_tokens":90,"cache_creation_input_tokens":3}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"first"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"call_1","name":"lookup","input":{}}}`,
		`{"type":"content_block_start","index":2,"content_block":{"type":"tool_use","id":"call_2","name":"lookup","input":{}}}`,
		`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"q\":"}}`,
		`{"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"{\"q\":2}"}}`,
		`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"1}"}}`,
		`{"type":"content_block_stop","index":1}`,
		`{"type":"content_block_stop","index":2}`,
		`{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":7}}`,
		`{"type":"message_stop"}`,
	}
	for i, event := range events {
		// Arbitrary network fragmentation and CRLF must not affect the mapping.
		for _, b := range []byte("event: ignored\r\ndata: " + event + "\r\n\r\n") {
			if _, err := w.Write([]byte{b}); err != nil {
				t.Fatal(err)
			}
		}
		if i == 2 && (!strings.Contains(out.String(), "response.output_text.delta") || strings.Contains(out.String(), "response.completed")) {
			t.Fatalf("stream buffered until completion: %s", out.String())
		}
	}
	if err := w.Finish(); err != nil {
		t.Fatal(err)
	}
	var complete map[string]any
	sequence := -1
	for _, line := range strings.Split(out.String(), "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var event map[string]any
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err != nil {
			t.Fatal(err)
		}
		if int(event["sequence_number"].(float64)) != sequence+1 {
			t.Fatalf("sequence = %v after %d", event["sequence_number"], sequence)
		}
		sequence++
		if event["type"] == "response.completed" {
			complete = event["response"].(map[string]any)
		}
	}
	if complete == nil || complete["model"] != "actual" || complete["status"] != "completed" {
		t.Fatalf("missing completion: %s", out.String())
	}
	items := complete["output"].([]any)
	if len(items) != 3 || items[1].(map[string]any)["arguments"] != `{"q":1}` || items[2].(map[string]any)["call_id"] != "call_2" {
		t.Fatalf("output = %+v", items)
	}
	usage := complete["usage"].(map[string]any)
	if usage["input_tokens"] != float64(104) || usage["output_tokens"] != float64(7) || usage["total_tokens"] != float64(111) || usage["input_tokens_details"].(map[string]any)["cached_tokens"] != float64(90) {
		t.Fatalf("usage = %+v", usage)
	}
}

func TestResponsesOutboundErrorsAreNotCompletions(t *testing.T) {
	for _, input := range []string{
		"data: {broken}\n\n",
		"data: {\"type\":\"message_start\",\"message\":{\"model\":\"m\"}}\n\n",
		"data: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\",\"message\":\"busy\"}}\n\n",
	} {
		var out bytes.Buffer
		w := NewResponsesStreamWriter(&out, "m")
		_, writeErr := w.Write([]byte(input))
		finishErr := w.Finish()
		if writeErr == nil && finishErr == nil {
			t.Fatalf("failed stream accepted: %q", input)
		}
		if !strings.Contains(out.String(), "response.failed") || strings.Contains(out.String(), "response.completed") {
			t.Fatalf("incorrect terminal event: %s", out.String())
		}
	}
}

func TestResponsesOutboundNonStreamingAndIncomplete(t *testing.T) {
	msg := types.MessageResponse{Type: "message", Model: "m", StopReason: "max_tokens", Content: []types.ContentBlock{{Type: "text", Text: "partial"}}, Usage: types.Usage{InputTokens: 11, CacheReadInputTokens: 90, OutputTokens: 7}}
	body, _ := json.Marshal(msg)
	response, err := AnthropicMessageToResponse(body, "requested")
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(response)
	if !strings.Contains(string(encoded), `"status":"incomplete"`) || !strings.Contains(string(encoded), `"reason":"max_output_tokens"`) || !strings.Contains(string(encoded), `"cached_tokens":90`) {
		t.Fatalf("response = %s", encoded)
	}
	if _, err := AnthropicMessageToResponse([]byte(`{"type":"error"}`), "m"); err == nil {
		t.Fatal("non-message accepted as success")
	}
	var out bytes.Buffer
	w := NewResponsesStreamWriter(&out, "m")
	for _, event := range []any{
		map[string]any{"type": "message_start", "message": map[string]any{"model": "m"}},
		map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": "max_tokens"}},
		map[string]any{"type": "message_stop"},
	} {
		if _, err := w.Write(responsesSSE(event)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Finish(); err != nil || !strings.Contains(out.String(), "response.incomplete") || strings.Contains(out.String(), "response.completed") {
		t.Fatalf("incomplete stream = %s, err=%v", out.String(), err)
	}
}

func TestResponsesOutboundRejectsNonObjectToolArguments(t *testing.T) {
	for _, arguments := range []string{`null`, `[]`, `"not an object"`, `42`} {
		t.Run(arguments, func(t *testing.T) {
			body := []byte(fmt.Sprintf(`{"type":"message","model":"m","stop_reason":"tool_use","content":[{"type":"tool_use","id":"call_1","name":"lookup","input":%s}]}`, arguments))
			if _, err := AnthropicMessageToResponse(body, "m"); err == nil || !strings.Contains(err.Error(), "JSON object") {
				t.Fatalf("invalid upstream tool arguments %s accepted: %v", arguments, err)
			}
		})
	}
}

// Codex 0.144 sends some of its function tools inside a "namespace" container
// in the same array as ordinary function tools. The container only groups them,
// so failing the whole request on a field the adapter does not model would
// reject tools it can already serve.
func TestResponsesInboundFlattensCodexToolNamespaces(t *testing.T) {
	raw := []byte(`{"model":"m","input":"hi","tools":[
		{"type":"function","name":"exec_command","strict":false,"parameters":{"type":"object","properties":{}}},
		{"type":"namespace","name":"multi_agent_v1","description":"Tools for spawning and managing sub-agents.","tools":[
			{"type":"function","name":"close_agent","strict":false,"parameters":{"type":"object","properties":{"target":{"type":"string"}}}},
			{"type":"namespace","name":"inner","tools":[
				{"type":"function","name":"resume_agent","strict":false,"parameters":{"type":"object","properties":{}}}
			]}
		]}
	]}`)
	req, err := ResponsesToMessageRequest(raw)
	if err != nil {
		t.Fatalf("a namespace container must not fail the request: %v", err)
	}
	var names []string
	for _, tool := range req.Tools {
		names = append(names, tool.Name)
	}
	if got := strings.Join(names, ","); got != "exec_command,close_agent,resume_agent" {
		t.Fatalf("tools = %q, want the container expanded in place", got)
	}

	// Flattening must not become a way to smuggle in a tool kind the adapter
	// refuses, nor to skip its validation.
	for name, body := range map[string]string{
		"custom tool":  `{"type":"namespace","name":"n","tools":[{"type":"custom","name":"c"}]}`,
		"strict tool":  `{"type":"namespace","name":"n","tools":[{"type":"function","name":"c","strict":true,"parameters":{"type":"object"}}]}`,
		"unnamed tool": `{"type":"namespace","name":"n","tools":[{"type":"function","strict":false,"parameters":{"type":"object"}}]}`,
	} {
		if _, err := ResponsesToMessageRequest([]byte(`{"model":"m","input":"hi","tools":[` + body + `]}`)); err == nil {
			t.Fatalf("%s inside a namespace was accepted", name)
		}
	}
}

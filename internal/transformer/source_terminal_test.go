package transformer

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/pkg/types"
)

func TestChatFailuresNeverSynthesizeCompletion(t *testing.T) {
	const content = `{"choices":[{"delta":{"content":"partial"}}],"usage":{"prompt_tokens":11,"completion_tokens":7}}`
	for _, tail := range []string{
		"", "[DONE]",
		`{"error":{"type":"server_error","message":"synthetic"},"choices":[{"delta":{"content":"not an answer"}}]}`,
		`{"choices":[{"delta":{"content":"broken"}}]`,
		`{"choices":[{"delta":{},"finish_reason":"unknown_terminal"}]}`,
	} {
		t.Run(tail, func(t *testing.T) {
			w := &responsesUsageRecorder{ResponseRecorder: httptest.NewRecorder()}
			err := NewStreamHandler().ProxyStream(w, sseLines(content, tail), "synthetic", t.Context(), 0, func() {})
			if err == nil || strings.Contains(w.Body.String(), "message_stop") || w.usage != [4]int{11, 7, 0, 0} {
				t.Fatalf("failure lost: err=%v usage=%v body=%s", err, w.usage, w.Body.String())
			}
		})
	}
}

func TestSourceTerminalEdgeCases(t *testing.T) {
	for _, raw := range []string{
		`{"promptFeedback":{"blockReason":"SAFETY"}}`,
		`{"candidates":[{"finishReason":"SAFETY"}]}`,
	} {
		var response types.GeminiResponse
		if err := json.Unmarshal([]byte(raw), &response); err != nil {
			t.Fatal(err)
		}
		normalized, err := GeminiToNormalized(&response, "synthetic")
		if err != nil {
			t.Fatal(err)
		}
		got := core.DenormalizeResponse(normalized)
		if got.StopReason != "refusal" || got.Role != "assistant" || got.Content == nil {
			t.Fatalf("empty refusal lost: %+v", got)
		}
	}
	for _, raw := range []string{`{}`, `{"candidates":[{"finishReason":"MALFORMED_FUNCTION_CALL"}]}`, `{"error":{"code":503,"message":"synthetic"}}`} {
		var response types.GeminiResponse
		if err := json.Unmarshal([]byte(raw), &response); err != nil {
			t.Fatal(err)
		}
		if got, err := GeminiToNormalized(&response, "synthetic"); err == nil {
			t.Fatalf("Gemini failure became success: %+v", got)
		}
	}
	for _, status := range []string{"", "in_progress", "queued", "incomplete"} {
		if got, err := ResponsesToNormalized(&types.ResponsesResponse{Status: status}, "synthetic"); err == nil {
			t.Fatalf("Responses status %q became success: %+v", status, got)
		}
	}
	w := httptest.NewRecorder()
	raw := "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"},\"finish_reason\":\"stop\"}]}\n\nevent:error\ndata:{\"message\":\"synthetic\"}\n\n"
	if err := NewStreamHandler().ProxyStream(w, io.NopCloser(strings.NewReader(raw)), "synthetic", t.Context(), 0, func() {}); err == nil || strings.Contains(w.Body.String(), "message_stop") {
		t.Fatalf("late named error became success: err=%v body=%s", err, w.Body.String())
	}
}

func TestChatIncompleteToolArgumentsDoNotComplete(t *testing.T) {
	w := httptest.NewRecorder()
	body := sseLines(`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"lookup","arguments":"{\"q\":"}}]}}]}`, `{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`)
	if err := NewStreamHandler().ProxyStream(w, body, "synthetic", t.Context(), 0, func() {}); err == nil || strings.Contains(w.Body.String(), "message_stop") {
		t.Fatalf("partial tool became complete: err=%v body=%s", err, w.Body.String())
	}
}

func TestChatRefusalAndUnknownFinishJSON(t *testing.T) {
	for _, tc := range []struct{ reason, want string }{
		{"stop", "end_turn"}, {"length", "max_tokens"}, {"content_filter", "refusal"}, {"unknown", ""}, {"", ""},
	} {
		var upstream types.ChatCompletionResponse
		if err := json.Unmarshal([]byte(`{"choices":[{"message":{"role":"assistant","content":"answer"},"finish_reason":"`+tc.reason+`"}]}`), &upstream); err != nil {
			t.Fatal(err)
		}
		got, err := NewResponseTransformer().TransformResponse(&upstream, "synthetic")
		if tc.want == "" {
			if err == nil {
				t.Errorf("%q became success: %+v", tc.reason, got)
			}
		} else if err != nil || got.StopReason != tc.want {
			t.Errorf("%q: got=%+v err=%v", tc.reason, got, err)
		}
	}
}

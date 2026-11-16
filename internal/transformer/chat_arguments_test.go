package transformer

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/internal/router"
	"github.com/routatic/proxy/pkg/types"
)

func TestChatToolArgumentBoundary(t *testing.T) {
	payload := `"` + strings.Repeat("x", (1<<20)-2) + `"`
	var tool chatToolState
	for i := 0; i < len(payload); i += 1024 {
		if err := tool.appendArguments(payload[i:min(i+1024, len(payload))]); err != nil {
			t.Fatal(err)
		}
	}
	if err := tool.finishArguments("tool_calls"); err != nil {
		t.Fatal(err)
	}
	if err := tool.appendArguments("x"); err == nil {
		t.Fatal("accepted arguments above 1 MiB")
	}
	if err := tool.finishArguments("tool_calls"); err != nil {
		t.Fatal("failed append changed arguments:", err)
	}
	var truncated chatToolState
	if err := truncated.appendArguments(`{"incomplete":`); err != nil {
		t.Fatal(err)
	}
	if err := truncated.finishArguments("length"); err != nil {
		t.Fatal(err)
	}
	if err := truncated.finishArguments("tool_calls"); err == nil {
		t.Fatal("accepted incomplete arguments")
	}
	var empty chatToolState
	if err := empty.finishArguments("tool_calls"); err != nil {
		t.Fatal(err)
	}
}

func TestChatNonstreamArgumentsStaySourceOwned(t *testing.T) {
	for _, arguments := range []string{"", `{}`, `{"q":"hi"}`, `[]`, `null`, `{"q":`, " "} {
		t.Run(arguments, func(t *testing.T) {
			upstream := &types.ChatCompletionResponse{
				Choices: []types.Choice{{Message: types.ChatMessage{Role: "assistant", ToolCalls: []types.ToolCall{{ID: "call", Type: "function", Function: types.FunctionCall{Name: "lookup", Arguments: arguments}}}}, FinishReason: "tool_calls"}},
				Usage:   &types.UsageInfo{PromptTokens: 11, CompletionTokens: 7},
			}
			response, err := NewResponseTransformer().TransformResponse(upstream, "synthetic")
			if arguments == "" || json.Valid([]byte(arguments)) {
				if err != nil {
					t.Fatal(err)
				}
				if _, err := json.Marshal(response); err != nil {
					t.Fatal(err)
				}
				want := arguments
				if want == "" {
					want = `{}`
				}
				if string(response.Content[0].Input) != want {
					t.Fatalf("valid arguments changed: %s", response.Content[0].Input)
				}
				return
			}
			var source *core.ResponseError
			if !errors.As(err, &source) || source.ReportedUsage() == nil || source.ReportedUsage().OutputTokens != 7 || router.IsNonRetryableRequestError(err) {
				t.Fatalf("source error/usage escaped transformer: %v", err)
			}
		})
	}
}

func BenchmarkChatToolArguments(b *testing.B) {
	part := strings.Repeat("x", 1024)
	b.ReportAllocs()
	for b.Loop() {
		var tool chatToolState
		for range 1024 {
			if err := tool.appendArguments(part); err != nil {
				b.Fatal(err)
			}
		}
	}
}

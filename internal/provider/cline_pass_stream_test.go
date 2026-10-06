package provider

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/routatic/proxy/internal/config"
)

func TestClinePassExecuteRequiresCompleteStream(t *testing.T) {
	const content = "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n"
	const finish = "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n"
	const done = "data: [DONE]\n\n"
	for _, tc := range []struct {
		name, wire string
		fail       bool
	}{
		{"complete", content + finish + done, false},
		{"content EOF", content, true},
		{"finish without terminal", content + finish, true},
		{"error after content", content + "data: {\"error\":{\"message\":\"synthetic failure\"}}\n\n" + done, true},
		{"malformed chunk", content + "data: {broken}\n\n" + finish + done, true},
		{"metadata only", "data: {\"usage\":{\"prompt_tokens\":7}}\n\n" + done, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, _ := clinePassTestProvider(t, func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, tc.wire)
			})
			result, err := p.Execute(context.Background(), clinePassRequest(), config.ModelConfig{Provider: "cline-pass", ModelID: "synthetic"})
			if tc.fail {
				if err == nil || result != nil {
					t.Fatalf("incomplete stream returned success: result=%v err=%v", result, err)
				}
			} else if err != nil || result == nil {
				t.Fatalf("complete stream failed: %v", err)
			}
		})
	}
}

func TestAggregateChatStreamPreservesTailAndReadError(t *testing.T) {
	const finish = "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\n"
	const tail = "data: {\"usage\":{\"prompt_tokens\":7,\"completion_tokens\":2}}\n\ndata: {\"provider_metadata\":{\"gateway\":{\"routing\":{\"finalProvider\":\"synthetic\"}}}}\n\ndata: [DONE]\n\n"
	response, err := aggregateChatStream(iotest.OneByteReader(strings.NewReader(finish+tail)), "synthetic")
	if err != nil || response == nil || response.Usage.PromptTokens != 7 || response.Usage.CompletionTokens != 2 {
		t.Fatalf("late usage lost: response=%v err=%v", response, err)
	}
	response, err = aggregateChatStream(io.MultiReader(strings.NewReader(finish), iotest.ErrReader(io.ErrUnexpectedEOF)), "synthetic")
	if response != nil || !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("read error lost: response=%v err=%v", response, err)
	}
}

func TestAggregateChatStreamSizeLimit(t *testing.T) {
	const chunk = "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\n"
	const ending = "\ndata: [DONE]\n\n"
	// Short comment lines exercise the total limit, not Scanner's line limit.
	padding := strings.Repeat(":"+strings.Repeat("x", 1022)+"\n", maxAggregatedStreamBytes/1024+1)
	for _, tc := range []struct {
		name string
		size int64
		fail bool
	}{
		{"below", maxAggregatedStreamBytes - 1, false},
		{"exact", maxAggregatedStreamBytes, false},
		{"above", maxAggregatedStreamBytes + 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := io.MultiReader(strings.NewReader(chunk),
				io.LimitReader(strings.NewReader(padding), tc.size-int64(len(chunk)+len(ending))),
				strings.NewReader(ending))
			result, err := aggregateChatStream(body, "synthetic")
			if tc.fail {
				if err == nil || result != nil || !strings.Contains(err.Error(), "exceeds") {
					t.Fatalf("oversized stream must fail without a partial response: result=%v, err=%v", result, err)
				}
				return
			}
			if err != nil || result == nil || result.Choices[0].Message.ContentText() != "ok" {
				t.Fatalf("valid bounded stream: result=%v, err=%v", result, err)
			}
		})
	}
}

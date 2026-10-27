package transformer

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/pkg/types"
)

const geminiObservedUsage = `{"promptTokenCount":101,"cachedContentTokenCount":90,"candidatesTokenCount":10,"thoughtsTokenCount":26,"totalTokenCount":137}`

func TestGeminiUsageIncludesThoughtsOnceAndSeparatesCache(t *testing.T) {
	var response types.GeminiResponse
	if err := json.Unmarshal([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]},"finishReason":"STOP"}],"usageMetadata":`+geminiObservedUsage+`}`), &response); err != nil {
		t.Fatal(err)
	}
	normalized, err := GeminiToNormalized(&response, "gemini-3.8-flash")
	if err != nil {
		t.Fatal(err)
	}
	usage := core.DenormalizeResponse(normalized).Usage
	if usage.InputTokens != 11 || usage.CacheReadInputTokens != 90 || usage.OutputTokens != 36 || usage.ReasoningTokens != 26 {
		t.Fatalf("Gemini usage = %+v", usage)
	}
}

func TestGeminiStreamRetainsPartialAndTrailingUsage(t *testing.T) {
	for _, tc := range []struct {
		name, tail string
		fail       bool
	}{
		{"finish without usage", `{"candidates":[{"finishReason":"STOP"}]}`, false},
		{"trailing totals", `{"candidates":[{"finishReason":"STOP"}]}` + "\n\ndata: " + `{"usageMetadata":{"promptTokenCount":101,"candidatesTokenCount":10}}`, false},
		{"truncated", "", true},
		{"malformed", `{"candidates":`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			w := &responsesUsageRecorder{ResponseRecorder: httptest.NewRecorder()}
			body := sseLines(`{"candidates":[{"content":{"parts":[{"text":"ok"}]}}],"usageMetadata":`+geminiObservedUsage+`}`, tc.tail)
			err := NewStreamHandler().ProxyGeminiStream(w, body, "gemini-3.8-flash", ctx, 0, cancel)
			if (err != nil) != tc.fail || w.usage != [4]int{11, 36, 90, 0} || w.reasoning != 26 {
				t.Fatalf("err=%v usage=%v reasoning=%d", err, w.usage, w.reasoning)
			}
			if tc.fail {
				if strings.Contains(w.Body.String(), "message_stop") {
					t.Fatal("failed stream fabricated success")
				}
				return
			}
			var final *types.Usage
			for _, event := range parseSSEEvents(t, w.Body.String()) {
				if event.Type == "message_delta" {
					final = event.Usage
				}
			}
			if final == nil || final.InputTokens != 11 || final.OutputTokens != 36 || final.CacheReadInputTokens != 90 || final.ReasoningTokens != 26 {
				t.Fatalf("terminal usage = %+v", final)
			}
		})
	}
}

package transformer

import (
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/routatic/proxy/pkg/types"
)

func TestPartialUsageUpdatesDoNotEraseEarlierCounts(t *testing.T) {
	for _, wire := range []string{"openai", "responses"} {
		for _, output := range []int{0, 7} {
			t.Run(fmt.Sprintf("%s/output=%d", wire, output), func(t *testing.T) {
				w := &responsesUsageRecorder{ResponseRecorder: httptest.NewRecorder()}
				h := NewStreamHandler()
				var err error
				if wire == "openai" {
					body := sseLines(`{"choices":[{"delta":{"content":"answer"}}],"usage":{"prompt_tokens":101,"completion_tokens":5,"prompt_tokens_details":{"cached_tokens":90}}}`,
						fmt.Sprintf(`{"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"completion_tokens":%d}}`, output))
					err = h.ProxyStream(w, body, "m", t.Context(), 0, func() {})
				} else {
					body := sseLines(`{"type":"response.output_text.delta","delta":"answer","usage":{"input_tokens":101,"output_tokens":5,"input_tokens_details":{"cached_tokens":90}}}`,
						fmt.Sprintf(`{"type":"response.completed","response":{"status":"completed","usage":{"output_tokens":%d}}}`, output))
					err = h.ProxyResponsesStream(w, body, "m", t.Context(), 0, func() {})
				}
				if err != nil {
					t.Fatal(err)
				}
				var final *types.Usage
				for _, event := range parseSSEEvents(t, w.Body.String()) {
					if event.Type == "message_delta" {
						final = event.Usage
					}
				}
				if final == nil || final.InputTokens != 11 || final.CacheReadInputTokens != 90 || final.OutputTokens != output || w.usage != [4]int{11, output, 90, 0} {
					t.Fatalf("cumulative/zero update lost: final=%+v partial=%v", final, w.usage)
				}
			})
		}
	}
}

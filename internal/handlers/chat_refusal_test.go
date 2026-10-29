package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/internal/router"
	"github.com/routatic/proxy/pkg/types"
)

func TestEmptyRefusalIsNotLowValueFailure(t *testing.T) {
	for _, scenario := range []router.Scenario{router.ScenarioComplex, router.ScenarioLongContext} {
		for _, responses := range []bool{false, true} {
			for _, usage := range []string{"null", `{"prompt_tokens":0,"completion_tokens":0}`, `{"prompt_tokens":5,"completion_tokens":2}`} {
				t.Run(fmt.Sprintf("%s/responses=%t/%s", scenario, responses, usage), func(t *testing.T) {
					synctest.Test(t, func(t *testing.T) {
						body := &timingScriptedBody{frames: []string{fmt.Sprintf(`{"choices":[{"delta":{"refusal":""},"finish_reason":"content_filter"}],"usage":%s}`, usage), "[DONE]"}}
						p := &timingProvider{body: body, wire: core.WireFormatOpenAIChat}
						h, records := newTimingHandler(t, p)
						out := httptest.NewRecorder()
						var w http.ResponseWriter = out
						if responses {
							w = &responsesHTTPWriter{ResponseWriter: out, headers: make(http.Header), model: "synthetic", streaming: true}
						}
						h.handleStreaming(w, httptest.NewRequest(http.MethodPost, "/v1/messages", nil), &types.MessageRequest{Stream: boolPtr(true), MaxTokens: 100},
							[]config.ModelConfig{{Provider: p.Name(), ModelID: "synthetic"}, {Provider: p.Name(), ModelID: "unused"}}, nil, scenario, "synthetic")
						if p.calls != 1 || len(records.rows) != 1 || !records.rows[0].Success || strings.Contains(out.Body.String(), `"type":"error"`) || strings.Contains(out.Body.String(), `"type":"response.failed"`) {
							t.Fatalf("refusal retried/failed: calls=%d rows=%+v body=%s", p.calls, records.rows, out.Body.String())
						}
						r := records.rows[0]
						if r.UsageUnobserved != (usage == "null") || r.GenerationSample != nil {
							t.Fatalf("fabricated usage/timing: %+v", r)
						}
					})
				})
			}
		}
	}
}

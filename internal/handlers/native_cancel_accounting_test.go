package handlers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/internal/provider"
	"github.com/routatic/proxy/internal/router"
	"github.com/routatic/proxy/internal/storage"
	"github.com/routatic/proxy/pkg/types"
)

type nativeCancelProvider struct {
	core.Provider
	cancel context.CancelFunc
	seen   *core.NativeResponseError
}

func (p *nativeCancelProvider) Execute(ctx context.Context, req *types.MessageRequest, model config.ModelConfig) (*core.ExecuteResult, error) {
	result, err := p.Provider.Execute(ctx, req, model)
	if errors.As(err, &p.seen) && p.cancel != nil {
		p.cancel()
	}
	return result, err
}

func TestNativeFailureSurvivesLateCancellation(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		for _, status := range []string{"failed", "cancelled"} {
			for _, usageMode := range []string{"missing", "null", "zero", "present"} {
				t.Run(fmt.Sprintf("canceled=%t/%s/usage=%s", canceled, status, usageMode), func(t *testing.T) {
					var calls atomic.Int32
					raw := fmt.Sprintf(`{"id":"source-failure","object":"response","model":"source-model","status":%q,"output":[],"error":{"code":"server_error","message":"synthetic source failure"}`, status)
					switch usageMode {
					case "null":
						raw += `,"usage":null`
					case "zero":
						raw += `,"usage":{"input_tokens":0,"output_tokens":0}`
					case "present":
						raw += `,"usage":{"input_tokens":101,"output_tokens":7,"input_tokens_details":{"cached_tokens":90},"output_tokens_details":{"reasoning_tokens":3}}`
					}
					raw += `}`
					h, db := newResponsesTestHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls.Add(1)
						_, _ = io.Copy(io.Discard, r.Body)
						w.Header().Set("Content-Type", "application/json")
						_, _ = io.WriteString(w, raw)
					}))
					cfg := *h.modelRouter.Config()
					cfg.CommandCode.ResponsesBaseURL = strings.TrimSuffix(cfg.CommandCode.BaseURL, "/chat/completions") + "/responses"
					cfg.Models = map[string]config.ModelConfig{"default": {Provider: "commandcode", ModelID: "synthetic-model", WireFormat: "responses", MaxTokens: 100}}
					fallbacks := []config.ModelConfig{{Provider: "commandcode", ModelID: "backup", WireFormat: "responses", MaxTokens: 100}}
					cfg.Fallbacks = map[string][]config.ModelConfig{"default": fallbacks, "fast": fallbacks}
					atomicCfg := config.NewAtomicConfig(&cfg, "")
					h.modelRouter = router.NewModelRouter(atomicCfg)
					ctx, cancel := context.WithCancel(t.Context())
					defer cancel()
					wrapped := &nativeCancelProvider{Provider: provider.NewCommandCodeProvider(atomicCfg, nil)}
					if canceled {
						wrapped.cancel = cancel
					}
					h.providerRegistry = core.NewProviderRegistry()
					if err := h.providerRegistry.Register(wrapped); err != nil {
						t.Fatal(err)
					}
					out := httptest.NewRecorder()
					h.HandleResponses(out, httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"alias","input":"hi"}`)).WithContext(ctx))
					if calls.Load() != 1 || wrapped.seen == nil || string(wrapped.seen.FinalResponse()) != raw {
						t.Fatalf("fixture did not receive exactly one complete native failure: calls=%d source=%+v body=%s", calls.Load(), wrapped.seen, out.Body.String())
					}
					if canceled && (out.Body.Len() != 0 || out.Header().Get("Content-Type") != "") {
						t.Errorf("response written after cancellation: headers=%v body=%s", out.Header(), out.Body.String())
					}
					if !canceled && (out.Code != http.StatusOK || out.Body.String() != raw) {
						t.Errorf("complete native failure changed: status=%d body=%s", out.Code, out.Body.String())
					}
					snapshot := h.metrics.GetSnapshot()
					wantFailed := int64(1)
					if canceled {
						wantFailed = 0
					}
					if snapshot.RequestsFailed != wantFailed || snapshot.RequestsSuccess != 0 || snapshot.UpstreamCalls != 1 {
						t.Errorf("logical request/cancellation metrics changed: %+v", snapshot)
					}
					rows, total, err := storage.NewRequests(db).Query(t.Context(), storage.RequestQuery{Page: 1, PageSize: 10})
					if err != nil || total != 1 {
						t.Fatalf("native source failure vanished or duplicated: rows=%+v total=%d err=%v", rows, total, err)
					}
					row := rows[0]
					// An all-zero usage object is unreported, not a measured zero.
					observed := usageMode == "present"
					source := wrapped.seen.Response
					if row.Success || !row.OutcomeKnown || row.DetailsKnown != observed || !observed && row.CostKnown {
						t.Errorf("source outcome/usage presence lost: %+v", row)
					}
					if row.Model != "synthetic-model" || row.Provider != "commandcode" || row.RequestedModel != "alias" || row.ResponseModel != "source-model" || row.UpstreamModel != source.UpstreamModel || row.UpstreamChannel != source.UpstreamChannel || row.KeyHint == "" || row.KeyHint != source.KeyHint || row.Attempt != 1 || row.CandidatePosition != 1 || row.Streaming {
						t.Errorf("source/execution identity lost: %+v", row)
					}
					wantIn, wantRead, wantOut, wantReasoning := 0, 0, 0, 0
					if usageMode == "present" {
						wantIn, wantRead, wantOut, wantReasoning = 11, 90, 7, 3
					}
					if row.InputTokens != wantIn || row.CacheReadTokens != wantRead || row.OutputTokens != wantOut || row.ReasoningTokens != wantReasoning || !strings.Contains(row.ErrorMsg, wrapped.seen.Error()) {
						t.Errorf("reported source usage/error lost: %+v", row)
					}
				})
			}
		}
	}
}

package handlers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/internal/storage"
	"github.com/routatic/proxy/pkg/types"
)

type lateCancelFailureProvider struct {
	cancel  context.CancelFunc
	failure error
}

func (p *lateCancelFailureProvider) Name() string { return "commandcode" }
func (p *lateCancelFailureProvider) WireFormat(string) core.WireFormat {
	return core.WireFormatOpenAIChat
}
func (p *lateCancelFailureProvider) Execute(context.Context, *types.MessageRequest, config.ModelConfig) (*core.ExecuteResult, error) {
	p.cancel()
	return nil, p.failure
}
func (p *lateCancelFailureProvider) Stream(context.Context, *types.MessageRequest, config.ModelConfig) (io.ReadCloser, error) {
	p.cancel()
	return nil, p.failure
}

func TestObservedFailureSurvivesLateCancellation(t *testing.T) {
	sourceErr := errors.New("observed malformed source response")
	for _, tc := range []struct {
		name       string
		failure    error
		want       int64
		usageKnown bool
	}{
		{"source without usage", &core.ResponseError{Err: sourceErr}, 1, false},
		{"source with usage", &core.ResponseError{Err: sourceErr, Usage: &types.Usage{InputTokens: 11}}, 1, true},
		{"source and delivery cancellation", errors.Join(&core.ResponseError{Err: sourceErr}, context.Canceled), 1, false},
		{"source HTTP rejection", &client.APIError{StatusCode: 429, Body: "observed rate limit"}, 1, false},
		{"source status without usage", &core.ResponsesStatusError{Status: "failed"}, 1, false},
		{"caller cancellation only", context.Canceled, 0, false},
		{"wrapped caller cancellation", &core.ResponseError{Err: context.Canceled}, 0, false},
	} {
		for _, path := range []string{"/v1/messages", "/v1/responses"} {
			for _, stream := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/stream=%t", tc.name, path, stream), func(t *testing.T) {
					h, db := newResponsesTestHandler(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
						t.Error("stubbed provider must not make an HTTP request")
					}))
					ctx, cancel := context.WithCancel(t.Context())
					defer cancel()
					h.providerRegistry = core.NewProviderRegistry()
					if err := h.providerRegistry.Register(&lateCancelFailureProvider{cancel: cancel, failure: tc.failure}); err != nil {
						t.Fatal(err)
					}
					payload := fmt.Sprintf(`{"model":"alias","max_tokens":64,"messages":[{"role":"user","content":"hi"}],"stream":%t}`, stream)
					if path == "/v1/responses" {
						payload = fmt.Sprintf(`{"model":"alias","input":"hi","stream":%t}`, stream)
					}
					w := httptest.NewRecorder()
					r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(payload)).WithContext(ctx)
					if path == "/v1/responses" {
						h.HandleResponses(w, r)
					} else {
						h.HandleMessages(w, r)
					}
					rows, total, err := storage.NewRequests(db).Query(t.Context(), storage.RequestQuery{Page: 1, PageSize: 10})
					if err != nil || total != tc.want {
						t.Fatalf("observed failure records=%d want=%d err=%v rows=%+v", total, tc.want, err, rows)
					}
					if total == 1 {
						row := rows[0]
						if row.Success || !row.OutcomeKnown || row.ErrorMsg != tc.failure.Error() || row.DetailsKnown != tc.usageKnown || (!tc.usageKnown && row.CostKnown) {
							t.Fatalf("source failure lost its outcome/usage boundary: %+v", row)
						}
						if tc.usageKnown && row.InputTokens != 11 {
							t.Fatalf("observed usage lost: %+v", row)
						}
					}
					if h.metrics.GetSnapshot().RequestsFailed != 0 {
						t.Fatal("late caller cancellation must remain neutral for logical request metrics")
					}
				})
			}
		}
	}
}

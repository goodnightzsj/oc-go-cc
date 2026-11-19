package handlers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/internal/router"
	"github.com/routatic/proxy/internal/storage"
	"github.com/routatic/proxy/pkg/types"
)

type cancelCauseProvider struct {
	core.Provider
	format          core.WireFormat
	onRead, onClose func()
	fail            func(context.Context) error
	calls           int
}

func (*cancelCauseProvider) Name() string                        { return "opencode-zen" }
func (p *cancelCauseProvider) WireFormat(string) core.WireFormat { return p.format }
func (p *cancelCauseProvider) Stream(ctx context.Context, _ *types.MessageRequest, _ config.ModelConfig) (io.ReadCloser, error) {
	p.calls++
	if p.fail != nil {
		return nil, p.fail(ctx)
	}
	return &cancelCauseBody{ctx: ctx, onRead: p.onRead, onClose: p.onClose}, nil
}

func (p *cancelCauseProvider) Execute(ctx context.Context, _ *types.MessageRequest, _ config.ModelConfig) (*core.ExecuteResult, error) {
	p.calls++
	return nil, p.fail(ctx)
}

type cancelCauseBody struct {
	ctx             context.Context
	onRead, onClose func()
}

func (b *cancelCauseBody) Read([]byte) (int, error) {
	if b.onRead != nil {
		b.onRead()
	}
	<-b.ctx.Done()
	return 0, b.ctx.Err()
}
func (b *cancelCauseBody) Close() error {
	if b.onClose != nil {
		b.onClose()
	}
	return nil
}

func TestStreamCancellationKeepsFirstCause(t *testing.T) {
	for _, path := range []struct {
		name   string
		format core.WireFormat
		native bool
	}{
		{"anthropic", core.WireFormatAnthropic, true},
		{"native-responses", core.WireFormatOpenAIResponses, true},
		{"chat", core.WireFormatOpenAIChat, false},
		{"responses", core.WireFormatOpenAIResponses, false},
		{"gemini", core.WireFormatGemini, false},
	} {
		for _, cause := range []string{"timeout", "idle", "caller-cancel", "caller-deadline"} {
			for _, lateCancel := range []string{"none", "cancel", "deadline"} {
				t.Run(fmt.Sprintf("%s/%s/late=%s", path.name, cause, lateCancel), func(t *testing.T) {
					db, h := newFailureTestHandler(t, nil)
					ctx, cancel := context.WithCancel(t.Context())
					defer cancel()
					if cause == "caller-deadline" || lateCancel == "deadline" {
						var stop context.CancelFunc
						ctx, stop = context.WithTimeout(ctx, 100*time.Millisecond)
						defer stop()
					}
					if path.native && path.format == core.WireFormatOpenAIResponses {
						ctx = core.WithRequestMetadata(ctx, core.RequestMetadata{ResponsesBody: []byte(`{"input":"hi"}`)})
					}
					zen := config.OpenCodeZenConfig{StreamingTimeoutMs: 5000, StreamTimeoutMs: 5000}
					if cause == "timeout" {
						zen.StreamingTimeoutMs = 20
					}
					if cause == "idle" {
						zen.StreamTimeoutMs = 20
					}
					h.client = client.NewOpenCodeClient(config.NewAtomicConfig(&config.Config{OpenCodeZen: zen}, ""))
					p := &cancelCauseProvider{format: path.format}
					if lateCancel == "cancel" {
						p.onClose = cancel
					} else if lateCancel == "deadline" {
						p.onClose = func() { <-ctx.Done() }
					}
					if cause == "caller-cancel" {
						p.onRead = cancel
					}
					h.providerRegistry = core.NewProviderRegistry()
					if err := h.providerRegistry.Register(p); err != nil {
						t.Fatal(err)
					}
					w := httptest.NewRecorder()
					h.handleStreaming(w, httptest.NewRequest(http.MethodPost, "/v1/messages", nil).WithContext(ctx),
						&types.MessageRequest{Stream: boolPtr(true), MaxTokens: 64},
						[]config.ModelConfig{{Provider: "opencode-zen", ModelID: "synthetic"}},
						nil, router.ScenarioDefault, "cancel-cause")
					rows, total, err := storage.NewRequests(db).Query(t.Context(), storage.RequestQuery{Page: 1, PageSize: 10})
					if err != nil {
						t.Fatal(err)
					}
					sourceFailed := cause == "timeout" || cause == "idle"
					if sourceFailed {
						if total != 1 {
							t.Fatalf("known %s failure records=%d want=1", cause, total)
						}
						row := rows[0]
						if row.Success || !row.OutcomeKnown || row.DetailsKnown || row.CostKnown || !strings.Contains(row.ErrorMsg, cause) {
							t.Fatalf("lost source cause or invented usage: %+v", row)
						}
					} else if path.native && total != 0 {
						t.Fatalf("pure caller cancellation recorded %d false source failures", total)
					}
					wantFailures := int64(0)
					if sourceFailed && lateCancel == "none" {
						wantFailures = 1
					}
					if snapshot := h.metrics.GetSnapshot(); snapshot.RequestsFailed != wantFailures || snapshot.RequestsSuccess != 0 || p.calls != 1 {
						t.Fatalf("wrong settlement: %+v calls=%d want failures=%d", snapshot, p.calls, wantFailures)
					}
				})
			}
		}
	}
}

func TestAttemptDeadlineBeforeParentDeadline(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, childFirst := range []bool{false, true} {
			for _, failure := range []string{"plain", "wrapped", "usage", "source-error"} {
				t.Run(fmt.Sprintf("stream=%t/child-first=%t/%s", stream, childFirst, failure), func(t *testing.T) {
					db, h := newFailureTestHandler(t, nil)
					ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
					defer cancel()
					timeout := 5000
					if childFirst {
						timeout = 20
					}
					h.client = client.NewOpenCodeClient(config.NewAtomicConfig(&config.Config{OpenCodeZen: config.OpenCodeZenConfig{
						TimeoutMs: timeout, StreamingTimeoutMs: timeout,
					}}, ""))
					p := &cancelCauseProvider{format: core.WireFormatAnthropic}
					p.fail = func(attempt context.Context) error {
						<-attempt.Done()
						err := attempt.Err()
						<-ctx.Done() // Cleanup may outlive the caller's deadline.
						if failure == "plain" {
							return err
						}
						result := &core.ResponseError{Err: err}
						if failure == "usage" {
							result.Usage = &types.Usage{InputTokens: 7, OutputTokens: 3}
						}
						if failure == "source-error" {
							result.Err = fmt.Errorf("source rejected")
						}
						return result
					}
					h.providerRegistry = core.NewProviderRegistry()
					if err := h.providerRegistry.Register(p); err != nil {
						t.Fatal(err)
					}
					request := httptest.NewRequest(http.MethodPost, "/v1/messages", nil).WithContext(ctx)
					message := &types.MessageRequest{Stream: boolPtr(stream), MaxTokens: 64}
					chain := []config.ModelConfig{{Provider: p.Name(), ModelID: "synthetic"}}
					if stream {
						h.handleStreaming(httptest.NewRecorder(), request, message, chain, nil, router.ScenarioDefault, "deadline")
					} else {
						h.handleNonStreaming(httptest.NewRecorder(), request, message, chain, nil, router.ScenarioDefault, "deadline")
					}
					rows, total, err := storage.NewRequests(db).Query(t.Context(), storage.RequestQuery{Page: 1, PageSize: 10})
					if err != nil {
						t.Fatal(err)
					}
					want := int64(0)
					if childFirst || failure == "usage" || failure == "source-error" {
						want = 1
					}
					if total != want {
						t.Fatalf("failure records=%d want=%d", total, want)
					}
					if total == 1 {
						row := rows[0]
						if row.Success || !row.OutcomeKnown || row.DetailsKnown != (failure == "usage") {
							t.Fatalf("lost outcome or invented usage: %+v", row)
						}
						if failure == "usage" && (row.InputTokens != 7 || row.OutputTokens != 3) {
							t.Fatalf("lost reported usage: %+v", row)
						}
						if failure == "source-error" && row.ErrorMsg != "source rejected" {
							t.Fatalf("replaced explicit source error: %s", row.ErrorMsg)
						}
						if childFirst && failure != "source-error" && !strings.Contains(row.ErrorMsg, "timeout") {
							t.Fatalf("lost attempt timeout identity: %s", row.ErrorMsg)
						}
					}
					if snapshot := h.metrics.GetSnapshot(); p.calls != 1 || snapshot.RequestsFailed != 0 || snapshot.RequestsSuccess != 0 {
						t.Fatalf("cancellation changed logical outcome: calls=%d metrics=%+v", p.calls, snapshot)
					}
				})
			}
		}
	}
}

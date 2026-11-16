package handlers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/internal/metrics"
	"github.com/routatic/proxy/internal/router"
	"github.com/routatic/proxy/pkg/types"
)

type failedEventWriter struct{ *httptest.ResponseRecorder }

type shortJSONWriter struct {
	*httptest.ResponseRecorder
	writes int
}

func (w *shortJSONWriter) Write(p []byte) (int, error) {
	w.writes++
	return len(p) - 1, nil
}

type responsesJSONProvider struct {
	timingProvider
	body []byte
}

func (p *responsesJSONProvider) Execute(context.Context, *types.MessageRequest, config.ModelConfig) (*core.ExecuteResult, error) {
	return &core.ExecuteResult{Body: p.body}, nil
}

func TestResponsesJSONShortWriteRecordsDeliveryFailure(t *testing.T) {
	for _, native := range []bool{false, true} {
		t.Run(map[bool]string{false: "converted", true: "native"}[native], func(t *testing.T) {
			format := core.WireFormatAnthropic
			body := []byte(`{"type":"message","model":"m","content":[{"type":"text","text":"answer"}],"stop_reason":"end_turn","usage":{"input_tokens":2,"output_tokens":7}}`)
			if native {
				format = core.WireFormatOpenAIResponses
				body = []byte(`{"object":"response","status":"completed","model":"m","output":[],"usage":{"input_tokens":2,"output_tokens":7}}`)
			}
			p := &responsesJSONProvider{timingProvider: timingProvider{wire: format}, body: body}
			h, records := newTimingHandler(t, &p.timingProvider)
			h.providerRegistry = core.NewProviderRegistry()
			if err := h.providerRegistry.Register(p); err != nil {
				t.Fatal(err)
			}
			out := &shortJSONWriter{ResponseRecorder: httptest.NewRecorder()}
			bridge := &responsesHTTPWriter{ResponseWriter: out, headers: make(http.Header), model: "m"}
			r := httptest.NewRequest("POST", "/v1/messages", nil)
			r = r.WithContext(core.WithRequestMetadata(r.Context(), core.RequestMetadata{ResponsesBody: []byte(`{}`)}))
			h.handleNonStreaming(bridge, r, &types.MessageRequest{Model: "m", MaxTokens: 16}, []config.ModelConfig{{Provider: p.Name(), ModelID: "m"}}, nil, router.ScenarioDefault, "synthetic")
			if !errors.Is(bridge.Finish(), io.ErrShortWrite) || out.writes != 1 {
				t.Errorf("final short write lost or finish repeated: err=%v writes=%d", bridge.Finish(), out.writes)
			}
			if len(records.rows) != 1 || records.rows[0].Success {
				t.Errorf("partial delivery counted as success: %+v", records.rows)
			}
			key := config.ModelKey(config.ModelConfig{Provider: p.Name(), ModelID: "m"})
			if state := h.fallbackHandler.GetCircuitStates()[key]; state != "closed" {
				t.Errorf("delivery failure penalized healthy source: %s", state)
			}
		})
	}
}

func (w failedEventWriter) Write(p []byte) (int, error) {
	if strings.Contains(string(p), "response.failed") {
		return 0, io.ErrClosedPipe
	}
	return w.ResponseRecorder.Write(p)
}

func TestResponsesFailureOwnershipSourceSurvivesFailedErrorDelivery(t *testing.T) {
	out := failedEventWriter{httptest.NewRecorder()}
	bridge := &responsesHTTPWriter{ResponseWriter: out, headers: make(http.Header), streaming: true}
	bridge.Header().Set("Content-Type", "text/event-stream")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	w := &responseWriter{ResponseWriter: bridge, cancelWrites: cancel}
	_, err := w.Write([]byte("data: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\",\"message\":\"synthetic 503\"}}\n\n"))
	var source *core.ResponseError
	if !errors.As(err, &source) || !errors.Is(err, io.ErrClosedPipe) || ctx.Err() != nil {
		t.Fatalf("source error lost to failed error delivery: err=%v canceled=%v", err, ctx.Err())
	}
}

func TestNativeResponsesFailureOwnershipWriteAndFlush(t *testing.T) {
	for _, sourceFailed := range []bool{false, true} {
		for _, flush := range []bool{false, true} {
			t.Run(fmt.Sprintf("source-failed=%t/flush=%t", sourceFailed, flush), func(t *testing.T) {
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				out := &deadlineCheckWriter{ResponseRecorder: httptest.NewRecorder()}
				if flush {
					out.flushErr = io.ErrClosedPipe
				} else {
					out.writeErr = io.ErrClosedPipe
				}
				bridge := &responsesHTTPWriter{ResponseWriter: out, headers: make(http.Header), streaming: true}
				bridge.Header().Set("Content-Type", "text/event-stream")
				w := &responseWriter{ResponseWriter: bridge, cancelWrites: cancel}
				w.setSourceFormat(core.WireFormatOpenAIResponses, true)
				wire := nativeResponseTerminal
				if sourceFailed {
					wire = "data: {\"type\":\"response.failed\"}\n\n"
				}
				_, err := w.Write([]byte(wire))
				if flush {
					if err != nil {
						t.Fatal(err)
					}
					err = w.FlushError()
				}
				var source *core.ResponseError
				if !errors.Is(err, io.ErrClosedPipe) || errors.As(err, &source) != sourceFailed || (ctx.Err() != nil) == sourceFailed {
					t.Fatalf("error ownership lost: err=%v context=%v", err, ctx.Err())
				}
				if !errors.Is(w.finish(), io.ErrClosedPipe) {
					t.Fatal("finish lost delivery failure")
				}
			})
		}
	}
}

func TestNativeResponsesFailureOwnershipHistory(t *testing.T) {
	wire := "data: {\"type\":\"response.failed\",\"response\":{\"status\":\"failed\",\"usage\":{\"input_tokens\":2,\"output_tokens\":7}}}\n\n"
	p := &timingProvider{wire: core.WireFormatOpenAIResponses, body: io.NopCloser(strings.NewReader(wire))}
	h, records := newTimingHandler(t, p)
	bridge := &responsesHTTPWriter{ResponseWriter: failedEventWriter{httptest.NewRecorder()}, headers: make(http.Header), model: "synthetic", streaming: true}
	r := httptest.NewRequest("POST", "/v1/messages", nil)
	r = r.WithContext(core.WithRequestMetadata(r.Context(), core.RequestMetadata{ResponsesBody: []byte(`{}`)}))
	h.handleStreaming(bridge, r, &types.MessageRequest{Model: "synthetic", MaxTokens: 16, Stream: boolPtr(true)}, []config.ModelConfig{{Provider: p.Name(), ModelID: "synthetic"}}, nil, router.ScenarioDefault, "synthetic")
	if len(records.rows) != 1 {
		t.Fatalf("records=%+v", records.rows)
	}
	rec := records.rows[0]
	if rec.Success || rec.InputTokens != 2 || rec.OutputTokens != 7 || !strings.Contains(rec.ErrorMsg, "response.failed") || !strings.Contains(rec.ErrorMsg, io.ErrClosedPipe.Error()) {
		t.Fatalf("source/delivery failure or usage lost: %+v", rec)
	}
}

func TestResponsesFailureOwnershipSourceError(t *testing.T) {
	h, _ := newResponsesTestHandler(t, http.NotFoundHandler())
	h.providerRegistry = core.NewProviderRegistry()
	p := &fragmentedAnthropicProvider{usageLimitStreamProvider: usageLimitStreamProvider{
		name: "commandcode",
		body: "data: {\"type\":\"message_start\",\"message\":{\"model\":\"synthetic-model\",\"usage\":{\"input_tokens\":1}}}\n\n" +
			"data: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\",\"message\":\"synthetic 503 overload\"}}\n\n",
	}}
	if err := h.providerRegistry.Register(p); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		h.HandleResponses(httptest.NewRecorder(), httptest.NewRequest("POST", "/v1/responses", strings.NewReader(`{"model":"m","input":"hi","store":false,"stream":true}`)))
	}
	key := config.ModelKey(config.ModelConfig{Provider: "commandcode", ModelID: "synthetic-model"})
	if state := h.fallbackHandler.GetCircuitStates()[key]; state != "open" {
		t.Fatalf("source overload was neutralized as a client disconnect: %s", state)
	}
}

type finalFlushTimeout struct{}

func (finalFlushTimeout) Error() string   { return "synthetic client write timeout" }
func (finalFlushTimeout) Timeout() bool   { return true }
func (finalFlushTimeout) Temporary() bool { return true }

type finalFlushFailure struct {
	*httptest.ResponseRecorder
	completedFlushes int
}

func (w *finalFlushFailure) FlushError() error {
	if strings.Contains(w.Body.String(), "event: response.completed") {
		w.completedFlushes++
		if w.completedFlushes == 2 {
			return finalFlushTimeout{}
		}
	}
	return nil
}

func TestResponsesFailureOwnershipFinalFlush(t *testing.T) {
	h, _ := newResponsesTestHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"answer\"}}]}\n\ndata: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":1}}\n\ndata: [DONE]\n\n")
	}))
	for i := 0; i < 3; i++ {
		w := &finalFlushFailure{ResponseRecorder: httptest.NewRecorder()}
		h.HandleResponses(w, httptest.NewRequest("POST", "/v1/responses", strings.NewReader(`{"model":"m","input":"hi","store":false,"stream":true}`)))
		if w.completedFlushes < 2 {
			t.Fatal("fixture did not reach the final flush")
		}
	}
	key := config.ModelKey(config.ModelConfig{Provider: "commandcode", ModelID: "synthetic-model"})
	if state := h.fallbackHandler.GetCircuitStates()[key]; state != "closed" {
		t.Fatalf("healthy source penalized for final client flush: %s", state)
	}
}

type nativeFailureDeliveryWriter struct {
	*httptest.ResponseRecorder
	failure error
}

func (w *nativeFailureDeliveryWriter) Write(p []byte) (int, error) {
	if strings.Contains(string(p), "response.failed") {
		return 0, w.failure
	}
	return w.ResponseRecorder.Write(p)
}

func TestNativeSourceHealthIndependentOfDeliveryError(t *testing.T) {
	for _, mode := range []string{"delivered", "closed-pipe", "timeout"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				_, _ = io.Copy(io.Discard, r.Body)
				_, _ = io.WriteString(w, "data: {\"type\":\"response.failed\",\"response\":{\"status\":\"failed\",\"usage\":{\"input_tokens\":2,\"output_tokens\":7}}}\n\n")
			}))
			defer upstream.Close()
			cfg := config.NewAtomicConfig(&config.Config{OpenCodeZen: config.OpenCodeZenConfig{APIKey: "synthetic-only", ResponsesBaseURL: upstream.URL}}, "")
			store := &timingRecordStore{}
			h := &MessagesHandler{client: client.NewOpenCodeClient(cfg), providerRegistry: newTestProviderRegistry(t, cfg), fallbackHandler: router.NewFallbackHandler(nil, 3, time.Minute), streamProxy: NewStreamProxy(), metrics: metrics.New(), logger: slog.New(slog.NewTextHandler(io.Discard, nil)), storage: store}
			for range 3 {
				out := httptest.NewRecorder()
				var destination http.ResponseWriter = out
				if mode == "closed-pipe" {
					destination = &nativeFailureDeliveryWriter{out, io.ErrClosedPipe}
				} else if mode == "timeout" {
					destination = &nativeFailureDeliveryWriter{out, finalFlushTimeout{}}
				}
				bridge := &responsesHTTPWriter{ResponseWriter: destination, headers: make(http.Header), model: "m", streaming: true}
				r := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
				r = r.WithContext(core.WithRequestMetadata(r.Context(), core.RequestMetadata{ResponsesBody: []byte(`{"model":"m","input":"hi","store":false,"stream":true}`)}))
				h.handleStreaming(bridge, r, &types.MessageRequest{Model: "m", MaxTokens: 16, Stream: boolPtr(true)}, []config.ModelConfig{{Provider: "opencode-zen", ModelID: "m", WireFormat: "responses"}}, nil, router.ScenarioDefault, "synthetic")
			}
			if calls != 3 || len(store.rows) != 3 {
				t.Fatalf("executions=%d records=%d", calls, len(store.rows))
			}
			for _, rec := range store.rows {
				if rec.Success || rec.InputTokens != 2 || rec.OutputTokens != 7 || !strings.Contains(rec.ErrorMsg, "response.failed") {
					t.Fatalf("source failure/usage lost: %+v", rec)
				}
			}
			key := config.ModelKey(config.ModelConfig{Provider: "opencode-zen", ModelID: "m"})
			if state := h.fallbackHandler.GetCircuitStates()[key]; state != "closed" {
				t.Fatalf("same source classified differently due to %s delivery: %s", mode, state)
			}
		})
	}
}

type nonstreamDeliveryFailureWriter struct {
	*httptest.ResponseRecorder
	stage string
}

func (w *nonstreamDeliveryFailureWriter) Write(p []byte) (int, error) {
	if w.stage == "write" {
		return 0, io.ErrClosedPipe
	}
	return w.ResponseRecorder.Write(p)
}

func (w *nonstreamDeliveryFailureWriter) Finish() error {
	if w.stage == "finish" {
		return io.ErrClosedPipe
	}
	return nil
}

func TestNonstreamSourceAndDeliveryOwnership(t *testing.T) {
	for _, sourceFailed := range []bool{false, true} {
		for _, stage := range []string{"delivered", "write", "finish"} {
			t.Run(fmt.Sprintf("source-failed=%t/%s", sourceFailed, stage), func(t *testing.T) {
				status := "completed"
				if sourceFailed {
					status = "failed"
				}
				calls := 0
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					_, _ = io.Copy(io.Discard, r.Body)
					_, _ = fmt.Fprintf(w, `{"object":"response","status":%q,"model":"m","output":[],"usage":{"input_tokens":2,"output_tokens":7}}`, status)
				}))
				defer upstream.Close()
				cfg := config.NewAtomicConfig(&config.Config{OpenCodeZen: config.OpenCodeZenConfig{APIKey: "synthetic-only", ResponsesBaseURL: upstream.URL}}, "")
				store := &timingRecordStore{}
				h := &MessagesHandler{client: client.NewOpenCodeClient(cfg), providerRegistry: newTestProviderRegistry(t, cfg), fallbackHandler: router.NewFallbackHandler(nil, 3, time.Minute), metrics: metrics.New(), logger: slog.New(slog.NewTextHandler(io.Discard, nil)), storage: store}
				out := &nonstreamDeliveryFailureWriter{ResponseRecorder: httptest.NewRecorder(), stage: stage}
				r := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
				r = r.WithContext(core.WithRequestMetadata(r.Context(), core.RequestMetadata{ResponsesBody: []byte(`{"model":"m","input":"hi","store":false}`)}))
				h.handleNonStreaming(out, r, &types.MessageRequest{Model: "m", MaxTokens: 16}, []config.ModelConfig{{Provider: "opencode-zen", ModelID: "m", WireFormat: "responses"}}, nil, router.ScenarioDefault, "synthetic")
				if calls != 1 || len(store.rows) != 1 {
					t.Fatalf("executions=%d records=%d", calls, len(store.rows))
				}
				rec := store.rows[0]
				if rec.Success != (!sourceFailed && stage == "delivered") || rec.InputTokens != 2 || rec.OutputTokens != 7 {
					t.Fatalf("outcome/usage changed: %+v", rec)
				}
				if sourceFailed && !strings.Contains(rec.ErrorMsg, "upstream Responses status: failed") {
					t.Fatalf("source error erased: %q", rec.ErrorMsg)
				}
				if stage != "delivered" && !strings.Contains(rec.ErrorMsg, io.ErrClosedPipe.Error()) {
					t.Fatalf("delivery error erased: %q", rec.ErrorMsg)
				}
			})
		}
	}
}

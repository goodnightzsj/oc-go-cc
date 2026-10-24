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
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/internal/history"
	"github.com/routatic/proxy/internal/metrics"
	"github.com/routatic/proxy/internal/router"
	"github.com/routatic/proxy/pkg/types"
)

func timingFrames(wire core.WireFormat) []string {
	switch wire {
	case core.WireFormatAnthropic:
		return []string{
			`{"type":"message_start","message":{"id":"msg_test","type":"message","role":"assistant","model":"synthetic","content":[],"usage":{"input_tokens":5}}}`,
			`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}` + "\n\n" +
				`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`,
			`{"type":"content_block_stop","index":0}` + "\n\n" +
				`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":600,"reasoning_tokens":100}}` + "\n\n" +
				`data: {"type":"message_stop"}`,
		}
	case core.WireFormatOpenAIResponses:
		return []string{
			`{"type":"response.created","response":{"id":"resp_test","status":"in_progress"}}`,
			`{"type":"response.output_text.delta","delta":"hello"}`,
			`{"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":5,"output_tokens":600,"output_tokens_details":{"reasoning_tokens":100}}}}`,
		}
	case core.WireFormatGemini:
		return []string{
			`{"candidates":[{"content":{"parts":[]}}]}`,
			`{"candidates":[{"content":{"parts":[{"text":"hello"}]}}]}`,
			`{"candidates":[{"finishReason":"STOP","content":{"parts":[]}}]}`,
		}
	default:
		return []string{
			`{"choices":[{"delta":{"role":"assistant","content":""}}]}`,
			`{"choices":[{"delta":{"content":"hello"}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":600,"completion_tokens_details":{"reasoning_tokens":100}}}`,
		}
	}
}

func TestStreamTimingAcrossProtocolsAndCapture(t *testing.T) {
	for _, wire := range []core.WireFormat{core.WireFormatAnthropic, core.WireFormatOpenAIChat, core.WireFormatOpenAIResponses, core.WireFormatGemini} {
		for _, responses := range []bool{false, true} {
			for _, capture := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/responses=%t/capture=%t", wire, responses, capture), func(t *testing.T) {
					synctest.Test(t, func(t *testing.T) {
						body := &timingScriptedBody{frames: timingFrames(wire)}
						var source io.ReadCloser = body
						var captured string
						if capture {
							source = client.CaptureBody(source, func(b []byte) { captured = string(b) })
						}
						provider := &timingProvider{body: source, wire: wire}
						h, records := newTimingHandler(t, provider)
						out := httptest.NewRecorder()
						var writer http.ResponseWriter = out
						if responses {
							writer = &responsesHTTPWriter{ResponseWriter: out, headers: make(http.Header), model: "synthetic", streaming: true}
						}
						h.handleStreaming(writer, httptest.NewRequest(http.MethodPost, "/v1/messages", nil),
							&types.MessageRequest{Stream: boolPtr(true), MaxTokens: 1000},
							[]config.ModelConfig{{Provider: provider.Name(), ModelID: "synthetic"}},
							nil, router.ScenarioDefault, "synthetic")
						if len(records.rows) != 1 || !records.rows[0].Success {
							t.Fatalf("records=%+v body=%s", records.rows, out.Body.String())
						}
						r := records.rows[0]
						wantEvent, wantEnd := int64(0), int64(7000)
						if wire == core.WireFormatAnthropic {
							wantEvent, wantEnd = 1000, 6000
						}
						if r.FirstEventMs == nil || *r.FirstEventMs != wantEvent ||
							r.FirstContentMs == nil || *r.FirstContentMs != 3000 ||
							r.MessageEndMs == nil || *r.MessageEndMs != wantEnd || r.Duration != 7*time.Second {
							t.Fatalf("unexpected timing: %+v", r)
						}
						if !body.closed.Load() || (capture && captured != body.delivered.String()) {
							t.Fatal("capture or body lifecycle changed")
						}
						if !strings.Contains(out.Body.String(), "hello") {
							t.Fatal("content did not reach the downstream")
						}
						terminal := `"type":"message_stop"`
						if responses {
							terminal = `"type":"response.completed"`
						}
						if !strings.Contains(out.Body.String(), terminal) {
							t.Fatalf("missing downstream terminal: %s", out.Body.String())
						}
						// Gemini's existing stream path does not report usage. Timing
						// must not invent accounting to manufacture a throughput.
						if wire == core.WireFormatGemini {
							if r.OutputTokens != 0 || r.ContentTokensPerSecond() != nil {
								t.Fatal("unreported Gemini usage was fabricated")
							}
						} else if r.OutputTokens != 600 || r.ReasoningTokens != 100 {
							t.Fatalf("usage changed: %+v", r)
						}
					})
				})
			}
		}
	}
}

func TestStreamTimingFallbackAndInterruptedHistory(t *testing.T) {
	for _, outcome := range []string{"success", "truncated", "cancelled"} {
		t.Run(outcome, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				interrupted := outcome != "success"
				body := &timingScriptedBody{frames: timingFrames(core.WireFormatAnthropic)}
				if interrupted {
					body.frames = body.frames[:2]
					body.finalErr = io.ErrUnexpectedEOF
				}
				if outcome == "cancelled" {
					body.cancelFinal, body.finalErr = cancel, context.Canceled
				}
				provider := &timingProvider{body: body, wire: core.WireFormatAnthropic, refuseFirst: true}
				h, records := newTimingHandler(t, provider)
				h.handleStreaming(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/messages", nil).WithContext(ctx),
					&types.MessageRequest{Stream: boolPtr(true), MaxTokens: 1000},
					[]config.ModelConfig{{Provider: provider.Name(), ModelID: "refused"}, {Provider: provider.Name(), ModelID: "served"}},
					nil, router.ScenarioDefault, "synthetic")
				if provider.calls != 2 || len(records.rows) != 1 {
					t.Fatalf("calls=%d rows=%+v", provider.calls, records.rows)
				}
				r := records.rows[0]
				if r.Attempt != 2 || r.FirstEventMs == nil || *r.FirstEventMs != 3000 ||
					r.FirstContentMs == nil || *r.FirstContentMs != 5000 {
					t.Fatalf("fallback reset execution origin: %+v", r)
				}
				if interrupted {
					if r.Success || r.MessageEndMs != nil || r.ContentTokensPerSecond() != nil || r.EventTokensPerSecond() != nil {
						t.Fatalf("interruption fabricated success: %+v", r)
					}
				} else if !r.Success || r.MessageEndMs == nil || *r.MessageEndMs != 8000 || r.Duration != 9*time.Second {
					t.Fatalf("completion after fallback: %+v", r)
				}
			})
		})
	}
}

type timingScriptedBody struct {
	frames      []string
	next        int
	pending     string
	delivered   strings.Builder
	closed      atomic.Bool
	finalErr    error
	cancelFinal context.CancelFunc
}

func (r *timingScriptedBody) Read(p []byte) (int, error) {
	if r.pending == "" {
		if r.next == len(r.frames) {
			time.Sleep(time.Second)
			if r.cancelFinal != nil {
				r.cancelFinal()
			}
			if r.finalErr != nil {
				return 0, r.finalErr
			}
			return 0, io.EOF
		}
		time.Sleep(time.Duration(r.next+1) * time.Second)
		r.pending = "data: " + r.frames[r.next] + "\n\n"
		r.next++
	}
	n := copy(p, r.pending)
	r.delivered.WriteString(r.pending[:n])
	r.pending = r.pending[n:]
	return n, nil
}

func (r *timingScriptedBody) Close() error { r.closed.Store(true); return nil }

type timingProvider struct {
	body        io.ReadCloser
	wire        core.WireFormat
	refuseFirst bool
	calls       int
}

func (p *timingProvider) Name() string                      { return "opencode-zen" }
func (p *timingProvider) WireFormat(string) core.WireFormat { return p.wire }
func (p *timingProvider) Execute(context.Context, *types.MessageRequest, config.ModelConfig) (*core.ExecuteResult, error) {
	return nil, errors.New("unexpected non-streaming call")
}
func (p *timingProvider) Stream(context.Context, *types.MessageRequest, config.ModelConfig) (io.ReadCloser, error) {
	p.calls++
	if p.refuseFirst && p.calls == 1 {
		time.Sleep(2 * time.Second)
		return nil, errors.New("synthetic refusal before payload")
	}
	return p.body, nil
}

type timingRecordStore struct{ rows []history.RequestRecord }

func (s *timingRecordStore) InsertRequest(r history.RequestRecord) error {
	s.rows = append(s.rows, r)
	return nil
}

func newTimingHandler(t *testing.T, p *timingProvider) (*MessagesHandler, *timingRecordStore) {
	t.Helper()
	registry := core.NewProviderRegistry()
	if err := registry.Register(p); err != nil {
		t.Fatal(err)
	}
	store := &timingRecordStore{}
	return &MessagesHandler{
		fallbackHandler:  router.NewFallbackHandler(nil, 3, time.Minute),
		client:           client.NewOpenCodeClient(config.NewAtomicConfig(&config.Config{}, "")),
		providerRegistry: registry, streamProxy: NewStreamProxy(),
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)), metrics: metrics.New(), storage: store,
	}, store
}

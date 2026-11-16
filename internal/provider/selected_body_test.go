package provider

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/internal/transformer"
)

type terminalReadError struct{ payload string }

func (b *terminalReadError) Read(p []byte) (int, error) {
	n := copy(p, b.payload)
	b.payload = b.payload[n:]
	return n, io.ErrUnexpectedEOF
}
func (b *terminalReadError) Close() error { return nil }

func TestSelectedBodyNativeTerminalBeforeTransportCleanup(t *testing.T) {
	for _, tc := range []struct {
		name, wire string
		format     core.WireFormat
		healthy    bool
	}{
		{"anthropic", "data: {\"type\":\"message_stop\"}\n\n", core.WireFormatAnthropic, true},
		{"responses", "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n", core.WireFormatOpenAIResponses, true},
		{"chat-awaits-eof", "data: {\"choices\":[{\"finish_reason\":\"stop\"}]}\n\n", core.WireFormatOpenAIChat, false},
		{"half-tail", "data: {\"type\":\"message_stop\"}\n\ndata: {", core.WireFormatAnthropic, false},
		{"source-error", "data: {\"type\":\"message_stop\"}\n\ndata: {\"type\":\"error\"}\n\n", core.WireFormatAnthropic, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pool := newKeyPool()
			keys := []string{"synthetic-a", "synthetic-b"}
			for i := 0; i < 3; i++ {
				sel := pool.Reserve(keys)
				b := &selectedBody{ReadCloser: &terminalReadError{tc.wire}, outcome: &keyOutcome{ctx: t.Context(), sel: sel.handoff()}, observation: transformer.StreamTermination{Format: tc.format}}
				_, err := b.Read(make([]byte, 2048))
				if !errors.Is(err, io.ErrUnexpectedEOF) {
					t.Fatalf("observer changed raw transport error: %v", err)
				}
				_ = b.Close()
			}
			next := pool.Reserve(keys)
			defer next.Release()
			if (next.Key() == keys[0]) != tc.healthy {
				t.Fatalf("terminal health disagrees with protocol: next=%s healthy=%t", next.Key(), tc.healthy)
			}
		})
	}
}

func TestSelectedBodyMalformedToolMatchesConsumer(t *testing.T) {
	for _, finish := range []string{"tool_calls", "length"} {
		t.Run(finish, func(t *testing.T) {
			payload := "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"function\":{\"name\":\"lookup\",\"arguments\":\"{\\\"q\\\":\"}}]}}]}\n\n" +
				"data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"" + finish + "\"}]}\n\ndata: [DONE]\n\n"
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, payload) }))
			defer upstream.Close()
			p, pool := selectedTestProvider("workbuddy", upstream.URL)
			_, _, req := cancellationTestProvider("")
			pool.stateFor("synthetic-a").consecutive = 1
			body, err := p.Stream(t.Context(), req, config.ModelConfig{Provider: "workbuddy", ModelID: "synthetic", WireFormat: "openai"})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			err = transformer.NewStreamHandler().ProxyStream(httptest.NewRecorder(), body, "synthetic", ctx, 0, cancel)
			want := 0
			if finish != "length" {
				want = 2
				if err == nil || !strings.Contains(err.Error(), "arguments") {
					t.Fatalf("consumer did not reject incomplete arguments: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if got := pool.states["synthetic-a"].consecutive; got != want {
				t.Fatalf("consumer=%v key failures=%d want=%d", err, got, want)
			}
		})
	}
}

func selectedTestProvider(name, endpoint string) (core.Provider, *keyPool) {
	keys := []string{"synthetic-a", "synthetic-b"}
	cfg := &config.Config{
		APIKeys:     keys,
		OpenCodeGo:  config.OpenCodeGoConfig{BaseURL: endpoint, AnthropicBaseURL: endpoint, ResponsesBaseURL: endpoint},
		OpenCodeZen: config.OpenCodeZenConfig{BaseURL: endpoint, AnthropicBaseURL: endpoint, ResponsesBaseURL: endpoint, GeminiBaseURL: endpoint},
		AWSBedrock:  config.AWSBedrockConfig{BaseURL: endpoint, AnthropicBaseURL: endpoint},
		OpenRouter:  config.OpenRouterConfig{BaseURL: endpoint},
		CommandCode: config.CommandCodeConfig{BaseURL: endpoint, AnthropicBaseURL: endpoint, ResponsesBaseURL: endpoint, APIKeys: keys},
		ClinePass:   config.ClinePassConfig{BaseURL: endpoint, APIKeys: keys},
		WorkBuddy:   config.WorkBuddyConfig{BaseURL: endpoint, APIKeys: keys},
	}
	a := config.NewAtomicConfig(cfg, "")
	switch name {
	case "opencode-go":
		p := NewOpenCodeGoProvider(a, nil)
		return p, p.keys
	case "opencode-zen":
		p := NewOpenCodeZenProvider(a)
		return p, p.keys
	case "aws-bedrock":
		p := NewAWSBedrockProvider(a)
		return p, p.keys
	case "openrouter":
		p := NewOpenRouterProvider(a, nil)
		return p, p.keys
	case "commandcode":
		p := NewCommandCodeProvider(a, nil)
		return p, p.keys
	case "cline-pass":
		p := NewClinePassProvider(a, nil)
		return p, p.keys
	default:
		p := NewWorkBuddyProvider(a, nil)
		return p, p.keys
	}
}

func TestSelectedBodyProtocolCompletion(t *testing.T) {
	fixtures := map[core.WireFormat]string{
		core.WireFormatOpenAIChat:      "data: {\"model\":\"synthetic\",\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n",
		core.WireFormatAnthropic:       "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
		core.WireFormatOpenAIResponses: "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n",
		core.WireFormatGemini:          "data: {\"candidates\":[{\"finishReason\":\"STOP\"}]}\n\n",
	}
	for _, name := range []string{"opencode-go", "opencode-zen", "aws-bedrock", "openrouter", "commandcode", "cline-pass", "workbuddy"} {
		for format, complete := range fixtures {
			if (name == "openrouter" || name == "cline-pass" || name == "workbuddy") && format != core.WireFormatOpenAIChat {
				continue
			}
			if format == core.WireFormatGemini && name != "opencode-zen" {
				continue
			}
			for _, state := range []string{"complete", "partial", "source_error", "terminal_then_error", "close", "cancel"} {
				t.Run(name+"/"+format.String()+"/"+state, func(t *testing.T) {
					payload := complete
					switch state {
					case "partial", "close", "cancel":
						payload = "data: {\"model\":\"synthetic\"}\n\n"
					case "source_error":
						payload = "event: error\ndata: {\"type\":\"error\",\"error\":{\"message\":\"synthetic\"}}\n\n"
					case "terminal_then_error":
						payload += "event: error\ndata: {\"type\":\"error\",\"error\":{\"message\":\"synthetic\"}}\n\n"
					}
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						_, _ = io.WriteString(w, payload)
						if state == "close" || state == "cancel" {
							w.(http.Flusher).Flush()
							<-r.Context().Done()
						}
					}))
					defer upstream.Close()
					p, pool := selectedTestProvider(name, upstream.URL)
					pool.stateFor("synthetic-a").consecutive = 1
					_, _, req := cancellationTestProvider("")
					req.MaxTokens = 16
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()
					body, err := p.Stream(ctx, req, config.ModelConfig{Provider: name, ModelID: "synthetic", WireFormat: format.String()})
					if err != nil {
						t.Fatal(err)
					}
					if state == "cancel" {
						cancel()
					}
					if state != "close" && state != "cancel" {
						_, _ = io.Copy(io.Discard, body)
					}
					_ = body.Close()
					_ = body.Close()
					want := 2
					if state == "complete" {
						want = 0
					}
					if state == "close" || state == "cancel" {
						want = 1
					}
					st := pool.states["synthetic-a"]
					if st.inFlight != 0 || st.consecutive != want {
						t.Fatalf("inFlight=%d failures=%d want %d", st.inFlight, st.consecutive, want)
					}
				})
			}
		}
	}
}

func TestSelectedBodyConcurrentCloseReadIsNeutral(t *testing.T) {
	pool := newKeyPool()
	sel := pool.Reserve([]string{"synthetic"})
	r, w := io.Pipe()
	defer w.Close()
	b := &selectedBody{ReadCloser: r, outcome: &keyOutcome{ctx: context.Background(), sel: sel.handoff()}}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _, _ = b.Read(make([]byte, 8)) }()
	go func() { defer wg.Done(); _ = b.Close() }()
	_ = b.Close()
	wg.Wait()
	st := pool.states["synthetic"]
	if st.inFlight != 0 || st.consecutive != 0 {
		t.Fatalf("close penalized/leaked reservation: %+v", st)
	}
}

func TestSelectedBodyTruncationRotatesEveryProvider(t *testing.T) {
	for _, name := range []string{"opencode-go", "opencode-zen", "aws-bedrock", "openrouter", "commandcode", "cline-pass", "workbuddy"} {
		for _, stream := range []bool{false, true} {
			t.Run(name+fmtStream(stream), func(t *testing.T) {
				var used []string
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					used = append(used, r.Header.Get("Authorization"))
					if stream || name == "cline-pass" {
						_, _ = io.WriteString(w, "data: {\"model\":\"synthetic\",\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n")
					} else {
						_, _ = io.WriteString(w, `{"choices":[`)
					}
				}))
				defer upstream.Close()
				p, pool := selectedTestProvider(name, upstream.URL)
				_, _, req := cancellationTestProvider("")
				model := config.ModelConfig{Provider: name, ModelID: "synthetic", WireFormat: "openai"}
				for i := 0; i < 4; i++ {
					if stream {
						body, err := p.Stream(context.Background(), req, model)
						if err != nil {
							t.Fatal(err)
						}
						_, _ = io.Copy(io.Discard, body)
						_ = body.Close()
					} else if _, err := p.Execute(context.Background(), req, model); err == nil {
						t.Fatal("truncated response succeeded")
					}
				}
				if strings.Join(used, ",") != "Bearer synthetic-a,Bearer synthetic-a,Bearer synthetic-a,Bearer synthetic-b" {
					t.Fatalf("truncation did not affect health: %v", used)
				}
				for _, st := range pool.states {
					if st.inFlight != 0 {
						t.Fatal("reservation leaked")
					}
				}
			})
		}
	}
}

func TestSelectedBodyLateSuccessPreservesNewQuarantine(t *testing.T) {
	for _, status := range []int{401, 429} {
		pool := newKeyPool()
		old := pool.Reserve([]string{"synthetic"})
		newer := pool.Reserve([]string{"synthetic"})
		newer.Settle(&client.APIError{StatusCode: status, RetryAfter: "600"})
		until := pool.blockedUntil("synthetic")
		old.Settle(nil)
		if !pool.blockedUntil("synthetic").Equal(until) || until.Before(time.Now()) {
			t.Fatalf("late success erased %d quarantine", status)
		}
	}
}

type errorEventCanceledBody struct {
	sameRead bool
	read     bool
}

func (b *errorEventCanceledBody) Read(p []byte) (int, error) {
	if b.read {
		return 0, context.Canceled
	}
	b.read = true
	n := copy(p, "event: error\ndata: {\"error\":{\"message\":\"synthetic source failure\"}}\n\n")
	if b.sameRead {
		return n, context.Canceled
	}
	return n, nil
}
func (b *errorEventCanceledBody) Close() error { return nil }

func TestSelectedBodySourceErrorWinsLaterCancellation(t *testing.T) {
	for _, sameRead := range []bool{false, true} {
		pool := newKeyPool()
		sel := pool.Reserve([]string{"synthetic"})
		ctx, cancel := context.WithCancel(context.Background())
		b := &selectedBody{ReadCloser: &errorEventCanceledBody{sameRead: sameRead}, outcome: &keyOutcome{ctx: ctx, sel: sel.handoff()}}
		if sameRead {
			cancel()
		}
		_, _ = b.Read(make([]byte, 1024))
		cancel()
		_, _ = b.Read(make([]byte, 1024))
		_ = b.Close()
		if got := pool.states["synthetic"].consecutive; got != 1 {
			t.Fatalf("sameRead=%t: source failure became neutral: %d", sameRead, got)
		}
	}
}

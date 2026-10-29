package metrics

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/routatic/proxy/internal/core"
)

func generationFrame(json string) string { return "data: " + json + "\n\n" }

type generationScript struct {
	chunks []string
	closed bool
}

func (s *generationScript) Read(p []byte) (int, error) {
	if len(s.chunks) == 0 {
		time.Sleep(5 * time.Second) // HTTP cleanup is not generation.
		return 0, io.EOF
	}
	time.Sleep(time.Second)
	n := copy(p, s.chunks[0])
	s.chunks[0] = s.chunks[0][n:]
	if s.chunks[0] == "" {
		s.chunks = s.chunks[1:]
	}
	return n, nil
}

func (s *generationScript) Close() error { s.closed = true; return nil }

func TestSourceGenerationSample(t *testing.T) {
	for _, tc := range []struct {
		name   string
		format core.WireFormat
		chunks []string
		want   *GenerationSample
	}{
		{"chat", core.WireFormatOpenAIChat, []string{
			generationFrame(`{"choices":[{"delta":{"role":"assistant","content":""}}]}`),
			generationFrame(`{"choices":[{"delta":{"reasoning_content":"thinking"}}]}`),
			generationFrame(`{"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"completion_tokens":200,"completion_tokens_details":{"reasoning_tokens":180}}}`),
			generationFrame(`[DONE]`), generationFrame(`{"choices":[],"usage":{"completion_tokens":9999}}`),
		}, &GenerationSample{2000, 2000, 200}},
		{"responses", core.WireFormatOpenAIResponses, []string{
			generationFrame(`{"type":"response.created"}`),
			generationFrame(`{"type":"response.function_call_arguments.delta","delta":"{"}`),
			generationFrame(`{"type":"response.completed","response":{"status":"completed","usage":{"output_tokens":100}}}`),
			generationFrame(`[DONE]`),
		}, &GenerationSample{2000, 1000, 100}},
		{"chat refusal", core.WireFormatOpenAIChat, []string{
			generationFrame(`{"choices":[{"delta":{"role":"assistant","refusal":""}}]}`),
			generationFrame(`{"choices":[{"delta":{"refusal":"declined"}}]}`),
			generationFrame(`{"choices":[{"delta":{"content":"text"},"finish_reason":"stop"}],"usage":{"completion_tokens":100}}`),
			generationFrame(`[DONE]`),
		}, &GenerationSample{2000, 2000, 100}},
		{"responses refusal", core.WireFormatOpenAIResponses, []string{
			generationFrame(`{"type":"response.refusal.delta","delta":""}`),
			generationFrame(`{"type":"response.refusal.delta","delta":"declined"}`),
			generationFrame(`{"type":"response.output_text.delta","delta":"text"}`),
			generationFrame(`{"type":"response.completed","response":{"status":"completed","usage":{"output_tokens":100}}}`),
		}, &GenerationSample{2000, 2000, 100}},
		{"messages", core.WireFormatAnthropic, []string{
			generationFrame(`{"type":"message_start","message":{"usage":{"output_tokens":1}}}`),
			generationFrame(`{"type":"content_block_delta","delta":{"type":"thinking_delta","thinking":"hmm"}}`),
			generationFrame(`{"type":"message_delta","usage":{"output_tokens":150}}`),
			generationFrame(`{"type":"message_stop","usage":{"output_tokens":0}}`),
		}, &GenerationSample{2000, 2000, 150}},
		{"provisional usage", core.WireFormatAnthropic, []string{
			generationFrame(`{"type":"message_start","message":{"usage":{"output_tokens":1}}}`),
			generationFrame(`{"type":"content_block_delta","delta":{"type":"text_delta","text":"a"}}`),
			generationFrame(`{"type":"message_stop"}`),
		}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, observation := WithGenerationObservation(context.Background(), time.Now(), tc.format)
				body := &generationScript{chunks: append([]string(nil), tc.chunks...)}
				resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream; charset=utf-8"}}, Body: body}
				ObserveGenerationResponse(ctx, resp)
				got, err := io.ReadAll(resp.Body)
				if err != nil || string(got) != strings.Join(tc.chunks, "") {
					t.Fatalf("forwarded bytes/error changed: %s %v", got, err)
				}
				if err := resp.Body.Close(); err != nil || !body.closed {
					t.Fatal("body close not preserved")
				}
				if sample := observation.Sample(); !reflect.DeepEqual(sample, tc.want) {
					t.Fatalf("sample = %+v, want %+v", sample, tc.want)
				}
			})
		})
	}
}

func TestGenerationRateBoundaries(t *testing.T) {
	for _, tc := range []struct {
		duration, tokens int64
		want             *float64
	}{
		{99, 100, nil}, {0, 0, nil}, {100, 0, new(float64)}, {100, 1, generationRatePtr(10)},
		{1000, -1, nil}, {1000, 1 << 53, nil},
	} {
		t.Run(fmt.Sprintf("%d/%d", tc.tokens, tc.duration), func(t *testing.T) {
			s := &GenerationSample{DurationMs: tc.duration, OutputTokens: tc.tokens}
			if got := s.TokensPerSecond(); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("rate %v, want %v", got, tc.want)
			}
		})
	}
}

func generationRatePtr(v float64) *float64 { return &v }

func TestGenerationOutputKinds(t *testing.T) {
	for _, group := range []struct {
		format   core.WireFormat
		outputs  []string
		terminal string
	}{
		{core.WireFormatAnthropic, []string{
			`{"type":"content_block_start","content_block":{"type":"tool_use","id":"t","name":"read","input":{}}}`,
			`{"type":"content_block_start","content_block":{"type":"text","text":" "}}`,
			`{"type":"content_block_start","content_block":{"type":"thinking","thinking":"think"}}`,
			`{"type":"content_block_delta","delta":{"type":"input_json_delta","partial_json":"{"}}`,
		}, generationFrame(`{"type":"message_delta","usage":{"output_tokens":10}}`) + generationFrame(`{"type":"message_stop"}`)},
		{core.WireFormatOpenAIResponses, []string{
			`{"type":"response.reasoning_summary_text.delta","delta":"think"}`,
			`{"type":"response.reasoning_text.delta","delta":"think"}`,
			`{"type":"response.reasoning.delta","delta":"think"}`,
			`{"type":"response.custom_tool_call_input.delta","delta":"code"}`,
			`{"type":"response.output_item.added","item":{"type":"function_call","name":"read","arguments":""}}`,
			`{"type":"response.output_item.added","item":{"type":"custom_tool_call","input":"code"}}`,
		}, generationFrame(`{"type":"response.done","response":{"status":"completed","usage":{"output_tokens":10}}}`)},
		{core.WireFormatOpenAIChat, []string{
			`{"choices":[{"delta":{"content":" "}}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"function":{"name":"read"}}]}}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"function":{"arguments":"{"}}]}}]}`,
		}, generationFrame(`{"choices":[],"usage":{"completion_tokens":10}}`) + generationFrame(`[DONE]`)},
	} {
		for _, output := range group.outputs {
			t.Run(output, func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					_, o := WithGenerationObservation(t.Context(), time.Now(), group.format)
					body := &generationBody{ReadCloser: &generationScript{chunks: []string{generationFrame(output), group.terminal}}, observation: o}
					if _, err := io.Copy(io.Discard, body); err != nil {
						t.Fatal(err)
					}
					if sample := o.Sample(); !reflect.DeepEqual(sample, &GenerationSample{1000, 1000, 10}) {
						t.Fatalf("sample=%+v", sample)
					}
				})
			})
		}
	}
}

func TestGenerationUnusableStreams(t *testing.T) {
	const output = `{"type":"response.output_text.delta","delta":"a"}`
	const completed = `{"type":"response.completed","response":{"usage":{"output_tokens":10}}}`
	for _, tc := range []struct{ name, first, middle, terminal string }{
		{"missing output", `{"type":"response.created"}`, "", completed},
		{"unknown output", `{"type":"future.delta","delta":"a"}`, "", completed},
		{"empty output", `{"type":"response.output_text.delta","delta":""}`, "", completed},
		{"missing terminal", output, "", ""},
		{"missing usage", output, "", `{"type":"response.completed"}`},
		{"failed", output, "", `{"type":"response.failed","response":{"usage":{"output_tokens":10}}}`},
		{"incomplete", output, "", `{"type":"response.incomplete","response":{"usage":{"output_tokens":10}}}`},
		{"cancelled", output, "", `{"type":"response.completed","response":{"status":"cancelled","usage":{"output_tokens":10}}}`},
		{"negative", output, "", `{"type":"response.completed","usage":{"output_tokens":-1}}`},
		{"fractional", output, "", `{"type":"response.completed","usage":{"output_tokens":1.5}}`},
		{"string count", output, "", `{"type":"response.completed","usage":{"output_tokens":"10"}}`},
		{"null count", output, "", `{"type":"response.completed","usage":{"output_tokens":null}}`},
		{"bad JSON", output, "data: {broken\n\n", completed},
		{"null frame", output, generationFrame(`null`), completed},
		{"conflicting type", output, "event: response.completed\ndata: {\"type\":\"response.created\"}\n\n", completed},
		{"oversized", output, "data: " + strings.Repeat("x", 1<<20) + "\n\n", completed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, o := WithGenerationObservation(t.Context(), time.Now(), core.WireFormatOpenAIResponses)
			raw := generationFrame(tc.first) + tc.middle
			if tc.terminal != "" {
				raw += generationFrame(tc.terminal)
			}
			body := &generationBody{ReadCloser: io.NopCloser(strings.NewReader(raw)), observation: o}
			got, err := io.ReadAll(body)
			if err != nil || string(got) != raw {
				t.Fatal("optional observation changed forwarding")
			}
			if o.Sample() != nil {
				t.Fatalf("invalid stream produced a sample: %+v", o.Sample())
			}
		})
	}
}

func TestGenerationSSEFraming(t *testing.T) {
	for _, newline := range []string{"\n", "\r\n", "\r"} {
		t.Run(fmt.Sprintf("%q", newline), func(t *testing.T) {
			for _, fragmented := range []bool{false, true} {
				start := time.Now()
				_, o := WithGenerationObservation(t.Context(), start, core.WireFormatOpenAIResponses)
				body := &generationBody{observation: o}
				feed := func(raw string, at time.Time) {
					raw = strings.ReplaceAll(raw, "\n", newline)
					if fragmented {
						for _, c := range []byte(raw) {
							body.observe([]byte{c}, at)
						}
					} else {
						body.observe([]byte(raw), at)
					}
				}
				feed(": ping\n\nevent: ping\ndata:\n\n", start)
				feed("event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\ndata: \"delta\":\"你好\"}\n\n", start.Add(time.Second))
				feed(generationFrame(`{"type":"response.completed","usage":{"output_tokens":0}}`), start.Add(2*time.Second))
				if sample := o.Sample(); !reflect.DeepEqual(sample, &GenerationSample{1000, 1000, 0}) {
					t.Fatalf("fragmented=%t sample=%+v", fragmented, sample)
				}
			}
		})
	}
}

func TestGenerationResponseEligibility(t *testing.T) {
	for _, tc := range []struct {
		name, contentType, encoding string
		status                      int
		format                      core.WireFormat
		eligible                    bool
	}{
		{"sse", "text/event-stream", "", 200, core.WireFormatOpenAIChat, true},
		{"error", "text/event-stream", "", 429, core.WireFormatOpenAIChat, false},
		{"json", "application/json", "", 200, core.WireFormatOpenAIChat, false},
		{"unknown", "", "", 200, core.WireFormatOpenAIChat, false},
		{"encoded", "text/event-stream", "gzip", 200, core.WireFormatOpenAIChat, false},
		{"gemini", "text/event-stream", "", 200, core.WireFormatGemini, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, o := WithGenerationObservation(t.Context(), time.Now(), tc.format)
			o.sample = &GenerationSample{OutputTokens: 999}
			body := io.NopCloser(strings.NewReader(""))
			resp := &http.Response{StatusCode: tc.status, Header: http.Header{"Content-Type": {tc.contentType}, "Content-Encoding": {tc.encoding}}, Body: body}
			ObserveGenerationResponse(ctx, resp)
			if (body != resp.Body) != tc.eligible {
				t.Fatal("incorrect response eligibility")
			}
			if o.Sample() != nil {
				t.Fatal("new response retained another source's sample")
			}
			resp.Body = body
			ObserveGenerationResponse(context.Background(), resp)
			if resp.Body != body {
				t.Fatal("unobserved request wrapped")
			}
		})
	}
}

func TestGenerationReadErrorAndIncompleteTerminal(t *testing.T) {
	start := time.Now()
	_, observation := WithGenerationObservation(t.Context(), start, core.WireFormatOpenAIResponses)
	body := &generationBody{observation: observation}
	body.observe([]byte(generationFrame(`{"type":"response.output_text.delta","delta":"hello"}`)), start)
	// EOF cannot manufacture a frame boundary, even if the partial JSON parses.
	body.ReadCloser = &generationErrorReader{data: `data: {"type":"response.completed","usage":{"output_tokens":10}}`}
	got, err := io.ReadAll(body)
	if err != io.ErrUnexpectedEOF || string(got) != body.ReadCloser.(*generationErrorReader).data || observation.Sample() != nil {
		t.Fatalf("read error/data lost or incomplete terminal accepted: %s %v %+v", got, err, observation.Sample())
	}
}

type generationErrorReader struct{ data string }

func (r *generationErrorReader) Read(p []byte) (int, error) {
	return copy(p, r.data), io.ErrUnexpectedEOF
}
func (r *generationErrorReader) Close() error { return nil }

func BenchmarkGenerationObservation(b *testing.B) {
	output := strings.Repeat(generationFrame(`{"choices":[{"delta":{"content":"hello"}}]}`), 500)
	data := []byte(output + generationFrame(`{"choices":[],"usage":{"completion_tokens":500}}`) + generationFrame(`[DONE]`))
	at := time.Now()
	b.ReportAllocs()
	b.SetBytes(int64(len(data)))
	for range b.N {
		_, o := WithGenerationObservation(context.Background(), at, core.WireFormatOpenAIChat)
		body := &generationBody{observation: o}
		body.observe(data, at)
		if o.Sample() == nil {
			b.Fatal("sample missing")
		}
	}
}

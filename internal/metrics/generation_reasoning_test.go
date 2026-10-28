package metrics

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/routatic/proxy/internal/core"
)

func TestGenerationChatReasoningStartAndRate(t *testing.T) {
	for _, tc := range []struct {
		name, first, later string
		firstMS            int64
	}{
		{"canonical", `{"reasoning_content":"think"}`, `{"content":"answer"}`, 1000},
		{"alias", `{"reasoning":"think"}`, `{"content":"answer"}`, 1000},
		{"both", `{"reasoning_content":"canonical","reasoning":"alias"}`, `{"content":"answer"}`, 1000},
		{"empty canonical", `{"reasoning_content":"","reasoning":"think"}`, `{"content":"answer"}`, 1000},
		{"null canonical", `{"reasoning_content":null,"reasoning":"think"}`, `{"content":"answer"}`, 1000},
		{"empty alias", `{"reasoning_content":"think","reasoning":""}`, `{"content":"answer"}`, 1000},
		{"both empty", `{"reasoning_content":"","reasoning":""}`, `{"content":"answer"}`, 10000},
		{"both null", `{"reasoning_content":null,"reasoning":null}`, `{"content":"answer"}`, 10000},
		{"absent", `{"role":"assistant"}`, `{"content":"answer"}`, 10000},
		{"whitespace", `{"reasoning":" "}`, `{"content":"answer"}`, 1000},
		{"reasoning only", `{"reasoning":"think"}`, `{}`, 1000},
		{"reasoning then tool", `{"reasoning":"think"}`, `{"tool_calls":[{"function":{"name":"read","arguments":"{}"}}]}`, 1000},
		{"tool then reasoning", `{"tool_calls":[{"function":{"name":"read"}}]}`, `{"reasoning":"think"}`, 1000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				chunks := []string{generationFrame(fmt.Sprintf(`{"choices":[{"delta":%s}]}`, tc.first))}
				for range 8 {
					chunks = append(chunks, generationFrame(`{"choices":[{"delta":{}}]}`))
				}
				chunks = append(chunks,
					generationFrame(fmt.Sprintf(`{"choices":[{"delta":%s,"finish_reason":"stop"}]}`, tc.later)),
					generationFrame(`{"choices":[],"usage":{"completion_tokens":1100,"completion_tokens_details":{"reasoning_tokens":1000}}}`),
					generationFrame(`[DONE]`))
				ctx, observation := WithGenerationObservation(t.Context(), time.Now(), core.WireFormatOpenAIChat)
				body := &generationScript{chunks: append([]string(nil), chunks...)}
				resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: body}
				ObserveGenerationResponse(ctx, resp)
				var raw strings.Builder
				_, err := io.Copy(&raw, resp.Body)
				if err != nil || raw.String() != strings.Join(chunks, "") {
					t.Fatalf("forwarded bytes/error changed: %v", err)
				}
				if err := resp.Body.Close(); err != nil || !body.closed {
					t.Fatal("body close not preserved")
				}
				want := GenerationSample{FirstOutputMs: tc.firstMS, DurationMs: 12000 - tc.firstMS, OutputTokens: 1100}
				sample := observation.Sample()
				if sample == nil || *sample != want {
					t.Fatalf("sample = %+v, want %+v", sample, want)
				}
				if rate := sample.TokensPerSecond(); rate == nil || *rate != 1100*1000/float64(want.DurationMs) {
					t.Fatalf("unexpected rate: %v", rate)
				}
			})
		})
	}
}

func TestGenerationChatReasoningFramingAndProtocolBoundary(t *testing.T) {
	for _, format := range []core.WireFormat{core.WireFormatOpenAIChat, core.WireFormatAnthropic, core.WireFormatOpenAIResponses} {
		for _, newline := range []string{"\n", "\r\n", "\r"} {
			t.Run(fmt.Sprintf("%s/%q", format, newline), func(t *testing.T) {
				start := time.Now()
				_, observation := WithGenerationObservation(t.Context(), start, format)
				body := &generationBody{observation: observation}
				feed := func(raw string, at time.Time) {
					for _, c := range []byte(strings.ReplaceAll(raw, "\n", newline)) {
						body.observe([]byte{c}, at)
					}
				}
				feed(": ping\n\ndata:\n\n", start)
				feed("data: {\"choices\":[{\"delta\":\ndata: {\"reasoning\":\"思考\"}}]}\n\n", start.Add(time.Second))
				terminal := generationFrame(`{"choices":[],"usage":{"completion_tokens":1100}}`) + generationFrame(`[DONE]`)
				if format == core.WireFormatAnthropic {
					terminal = generationFrame(`{"type":"message_delta","usage":{"output_tokens":1100}}`) + generationFrame(`{"type":"message_stop"}`)
				} else if format == core.WireFormatOpenAIResponses {
					terminal = generationFrame(`{"type":"response.completed","response":{"status":"completed","usage":{"output_tokens":1100}}}`)
				}
				feed(terminal, start.Add(12*time.Second))
				sample := observation.Sample()
				if format != core.WireFormatOpenAIChat {
					if sample != nil {
						t.Fatalf("Chat alias was observed as %s output: %+v", format, sample)
					}
				} else if sample == nil || *sample != (GenerationSample{1000, 11000, 1100}) {
					t.Fatalf("fragmented Chat reasoning sample = %+v", sample)
				}
			})
		}
	}
}

func TestGenerationChatReasoningTerminalAndUsage(t *testing.T) {
	const usage = `{"choices":[],"usage":{"completion_tokens":1100}}`
	for _, tc := range []struct {
		name, tail string
		want       *GenerationSample
	}{
		{"missing usage", generationFrame(`[DONE]`), nil},
		{"missing terminal", generationFrame(usage), nil},
		{"error", generationFrame(`{"error":{"message":"failed"}}`) + generationFrame(usage) + generationFrame(`[DONE]`), nil},
		{"invalid alias", generationFrame(`{"choices":[{"delta":{"reasoning":{}}}]}`) + generationFrame(usage) + generationFrame(`[DONE]`), nil},
		{"zero output", generationFrame(`{"choices":[],"usage":{"completion_tokens":0}}`) + generationFrame(`[DONE]`), &GenerationSample{1000, 11000, 0}},
		{"repeated zero", generationFrame(usage) + generationFrame(`{"choices":[],"usage":{"completion_tokens":0}}`) + generationFrame(`[DONE]`), &GenerationSample{1000, 11000, 1100}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			start := time.Now()
			_, observation := WithGenerationObservation(t.Context(), start, core.WireFormatOpenAIChat)
			body := &generationBody{observation: observation}
			body.observe([]byte(generationFrame(`{"choices":[{"delta":{"reasoning":"think"}}]}`)), start.Add(time.Second))
			body.observe([]byte(tc.tail), start.Add(12*time.Second))
			sample := observation.Sample()
			if tc.want == nil {
				if sample != nil {
					t.Fatalf("invalid stream produced sample: %+v", sample)
				}
			} else if sample == nil || *sample != *tc.want {
				t.Fatalf("sample = %+v, want %+v", sample, tc.want)
			}
		})
	}
}

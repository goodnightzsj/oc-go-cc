package provider

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/internal/sse"
	"github.com/routatic/proxy/internal/transformer"
	"github.com/routatic/proxy/pkg/types"
)

// maxAggregatedStreamBytes bounds a collected stream. A non-streaming client
// asks for one document, so an upstream that keeps emitting has to be cut off
// rather than buffered without limit.
const maxAggregatedStreamBytes = 64 << 20

// aggregateChatStream reassembles a Chat Completions SSE stream into the single
// response object a non-streaming caller expects.
//
// This exists because the Cline API does not reliably serve non-streaming
// requests: independent clients report an empty body or "generateText is not
// implemented" for stream:false, while stream:true works. The flag this proxy
// sends upstream is therefore not the flag the client asked for - see
// ClinePassProvider.Execute.
//
// Deltas are accumulated rather than parsed as whole messages, because the
// upstream sends one choice split across many chunks: content arrives in
// pieces, tool arguments arrive as fragments that only form JSON once
// concatenated, and usage arrives in the final chunk (usually with no choices).
func aggregateChatStream(r io.Reader, modelID string) (_ *types.ChatCompletionResponse, err error) {
	out := &types.ChatCompletionResponse{
		Object:  "chat.completion",
		Model:   modelID,
		Choices: []types.Choice{{Index: 0}},
	}
	defer func() {
		if err != nil {
			err = &core.ResponseError{Err: err, Usage: transformer.ChatUsageToAnthropic(out.Usage)}
		}
	}()
	var content strings.Builder
	var reasoning strings.Builder
	toolCalls := map[int]*types.ToolCall{}
	var toolOrder []int
	seen := false
	complete := false

	limited := &io.LimitedReader{R: r, N: maxAggregatedStreamBytes + 1}
	var decoder sse.Decoder
	consume := func(event string, data []byte) error {
		payload := strings.TrimSpace(string(data))
		if payload == "" {
			return nil
		}
		if payload == "[DONE]" {
			complete = true
			return nil
		}
		var chunk types.ChatCompletionChunk
		previousReasoning := 0
		if out.Usage != nil {
			previousReasoning = transformer.ChatUsageToAnthropic(out.Usage).ReasoningTokens
			next := *out.Usage
			next.CompletionTokensDetails, next.CompletionThinkingTokens = nil, 0
			chunk.Usage = &next
		}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			return fmt.Errorf("decode cline-pass stream chunk: %w", err)
		}
		if chunk.Usage != nil {
			if transformer.ChatUsageToAnthropic(chunk.Usage).ReasoningTokens == 0 {
				chunk.Usage.CompletionThinkingTokens = previousReasoning
			}
			out.Usage = chunk.Usage
		}
		if event == "error" || len(chunk.Error) != 0 && string(chunk.Error) != "null" {
			return fmt.Errorf("cline-pass stream reported an upstream error")
		}
		seen = seen || len(chunk.Choices) > 0
		if chunk.ID != "" {
			out.ID = chunk.ID
		}
		if chunk.Created != 0 {
			out.Created = chunk.Created
		}
		if chunk.Model != "" {
			out.Model = chunk.Model
		}
		for _, choice := range chunk.Choices {
			if choice.FinishReason != "" {
				out.Choices[0].FinishReason = choice.FinishReason
			}
			delta := choice.Delta
			if delta.Content != nil {
				var piece string
				if err := json.Unmarshal(delta.Content, &piece); err == nil {
					content.WriteString(piece)
				}
			}
			reasoning.WriteString(delta.ReasoningText())
			for _, tc := range delta.ToolCalls {
				// Argument fragments are concatenated per index; the index is
				// what identifies one call across chunks, so a chunk that omits
				// it is attributed to the call it names or to the last one.
				idx := tc.Index
				if _, ok := toolCalls[idx]; !ok {
					toolCalls[idx] = &types.ToolCall{Index: idx, Type: tc.Type, ID: tc.ID}
					toolOrder = append(toolOrder, idx)
				}
				merged := toolCalls[idx]
				if tc.ID != "" {
					merged.ID = tc.ID
				}
				if tc.Type != "" {
					merged.Type = tc.Type
				}
				if tc.Function.Name != "" {
					merged.Function.Name = tc.Function.Name
				}
				merged.Function.Arguments += tc.Function.Arguments
			}
		}
		return nil
	}
	buf := make([]byte, 4096)
	for !complete {
		n, err := limited.Read(buf)
		if parseErr := decoder.Feed(buf[:n], consume); parseErr != nil {
			return nil, parseErr
		}
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("read cline-pass stream: %w", err)
		}
		if err == io.EOF {
			break
		}
	}
	if err := decoder.Finish(); err != nil {
		return nil, err
	}
	if limited.N == 0 {
		return nil, fmt.Errorf("cline-pass stream exceeds %d bytes", maxAggregatedStreamBytes)
	}
	if !seen {
		// An upstream that produced no readable chunk is a failure, not an
		// empty completion: reporting it as the latter would show a successful
		// request with no output.
		return nil, fmt.Errorf("cline-pass stream carried no data chunks")
	}
	// finish_reason ends the choice, not the wire stream: usage and routing
	// metadata may still follow. Only [DONE] confirms the complete stream.
	if !complete {
		return nil, fmt.Errorf("cline-pass stream ended before [DONE]: %w", io.ErrUnexpectedEOF)
	}
	if content.Len() > 0 {
		encoded, err := json.Marshal(content.String())
		if err != nil {
			return nil, err
		}
		out.Choices[0].Message.Content = encoded
	}
	if reasoning.Len() > 0 {
		text := reasoning.String()
		out.Choices[0].Message.ReasoningContent = &text
	}
	for _, idx := range toolOrder {
		out.Choices[0].Message.ToolCalls = append(out.Choices[0].Message.ToolCalls, *toolCalls[idx])
	}
	out.Choices[0].Message.Role = "assistant"
	return out, nil
}

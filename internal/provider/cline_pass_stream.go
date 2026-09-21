package provider

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"

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
func aggregateChatStream(r io.Reader, modelID string) (*types.ChatCompletionResponse, error) {
	out := &types.ChatCompletionResponse{
		Object:  "chat.completion",
		Model:   modelID,
		Choices: []types.Choice{{Index: 0}},
	}
	var content strings.Builder
	var reasoning strings.Builder
	toolCalls := map[int]*types.ToolCall{}
	var toolOrder []int
	seen := false

	limited := io.LimitReader(r, maxAggregatedStreamBytes+1)
	scanner := bufio.NewScanner(limited)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		payload, ok := strings.CutPrefix(line, "data:")
		if !ok {
			continue
		}
		payload = strings.TrimSpace(payload)
		if payload == "[DONE]" {
			break
		}
		var chunk types.ChatCompletionChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			// A chunk that is not JSON is not ours to interpret; a stream that
			// carries no readable chunk at all is caught by the seen guard.
			continue
		}
		seen = true
		if chunk.ID != "" {
			out.ID = chunk.ID
		}
		if chunk.Created != 0 {
			out.Created = chunk.Created
		}
		if chunk.Model != "" {
			out.Model = chunk.Model
		}
		if chunk.Usage != nil {
			out.Usage = *chunk.Usage
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
			if delta.ReasoningContent != nil {
				reasoning.WriteString(*delta.ReasoningContent)
			}
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
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read cline-pass stream: %w", err)
	}
	if !seen {
		// An upstream that produced no readable chunk is a failure, not an
		// empty completion: reporting it as the latter would show a successful
		// request with no output.
		return nil, fmt.Errorf("cline-pass stream carried no data chunks")
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

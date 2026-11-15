package transformer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/internal/sse"
	"github.com/routatic/proxy/pkg/types"
)

// StreamTermination observes source completion for key health without emitting
// output or coupling health to usage/token measurements. Framing and accepted
// finish reasons share the same validators as the protocol consumers.
type StreamTermination struct {
	Format         core.WireFormat
	RequireDone    bool
	decoder        sse.Decoder
	responses      ResponsesObservation
	terminal, done bool
	err            error
}

func (s *StreamTermination) Feed(p []byte) error {
	if s.err == nil {
		s.err = s.decoder.Feed(p, s.event)
	}
	return s.err
}

func (s *StreamTermination) Complete() bool {
	return s.err != nil || (s.terminal && (!s.RequireDone || s.done)) || s.responses.Done()
}

func (s *StreamTermination) Finish() error {
	if s.err != nil {
		return s.err
	}
	if err := s.decoder.Finish(); err != nil {
		return err
	}
	if !s.Complete() {
		return io.ErrUnexpectedEOF
	}
	return nil
}

func (s *StreamTermination) event(event string, data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil
	}
	if s.Format == core.WireFormatOpenAIResponses {
		_, err := s.responses.ReadEvent(event, data)
		return err
	}
	if bytes.Equal(data, []byte("[DONE]")) {
		s.done = true
		return nil
	}
	switch s.Format {
	case core.WireFormatAnthropic:
		var chunk struct {
			Type  string          `json:"type"`
			Error json.RawMessage `json:"error"`
		}
		if err := json.Unmarshal(data, &chunk); err != nil {
			return err
		}
		if err := upstreamJSONError("Anthropic", chunk.Error); err != nil {
			return err
		}
		if event == "error" || chunk.Type == "error" {
			return fmt.Errorf("upstream Anthropic error event")
		}
		if chunk.Type == "message_stop" || (chunk.Type == "" && event == "message_stop") {
			s.terminal = true
		}
	case core.WireFormatGemini:
		var chunk types.GeminiStreamChunk
		if err := json.Unmarshal(data, &chunk); err != nil {
			return err
		}
		if err := upstreamJSONError("Gemini", chunk.Error); err != nil {
			return err
		}
		if event == "error" {
			return upstreamJSONError("Gemini", data)
		}
		reason := ""
		if chunk.PromptFeedback != nil {
			reason = chunk.PromptFeedback.BlockReason
		}
		if len(chunk.Candidates) > 0 && chunk.Candidates[0].FinishReason != "" {
			reason = chunk.Candidates[0].FinishReason
		}
		if reason != "" {
			if _, err := geminiFinishReason(reason); err != nil {
				return err
			}
			s.terminal = true
		}
	default:
		var chunk types.ChatCompletionChunk
		if err := json.Unmarshal(data, &chunk); err != nil {
			return err
		}
		if err := upstreamJSONError("Chat Completions", chunk.Error); err != nil {
			return err
		}
		if event == "error" {
			return upstreamJSONError("Chat Completions", data)
		}
		if len(chunk.Choices) > 0 && chunk.Choices[0].FinishReason != "" {
			if _, err := NewResponseTransformer().mapFinishReason(chunk.Choices[0].FinishReason); err != nil {
				return err
			}
			s.terminal = true
		}
	}
	return nil
}

package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/internal/transformer"
	"github.com/routatic/proxy/pkg/types"
)

func (h *MessagesHandler) responsesChain(metadata core.RequestMetadata, requested string, chain []config.ModelConfig) ([]config.ModelConfig, error) {
	if len(metadata.ResponsesBody) == 0 || len(chain) == 0 {
		return chain, nil
	}
	native := func(model config.ModelConfig) bool {
		provider, ok := h.providerRegistry.Get(client.Provider(model))
		return ok && core.ModelWireFormat(provider, model) == core.WireFormatOpenAIResponses
	}
	if metadata.ResponsesBridgeError != nil && !native(chain[0]) {
		return nil, fmt.Errorf("request requires a native Responses target: %w", metadata.ResponsesBridgeError)
	}
	if metadata.ResponsesReplay && !native(chain[0]) {
		// Public summaries may use the existing bridge, but its fallback must
		// not suddenly replay the original source IDs on a native endpoint.
		out := make([]config.ModelConfig, 0, len(chain))
		for _, model := range chain {
			if !native(model) {
				out = append(out, model)
			}
		}
		return out, nil
	}
	if metadata.ResponsesReplay && native(chain[0]) {
		if requested != chain[0].ModelID || metadata.ResponsesSource == nil || len(metadata.ResponsesSource.ProviderAPIKeys(client.Provider(chain[0]))) != 1 {
			return nil, fmt.Errorf("opaque Responses replay requires an explicit unchanged model and exactly one upstream credential")
		}
		primary := chain[0]
		primary.WireFormat = "responses"
		return []config.ModelConfig{primary}, nil
	}
	if metadata.ResponsesBridgeError != nil {
		out := make([]config.ModelConfig, 0, len(chain))
		for _, model := range chain {
			if native(model) {
				out = append(out, model)
			}
		}
		return out, nil
	}
	return chain, nil
}

func (w *responseWriter) setSourceFormat(format core.WireFormat, native bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.sourceModel = ""
	// Converted message_start may contain a synthetic route model, not evidence.
	w.sourceAnthropic = format == core.WireFormatAnthropic
	if native {
		w.nativeResponses = &transformer.ResponsesObservation{}
	} else {
		w.nativeResponses = nil
	}
	if target, ok := w.ResponseWriter.(interface{ SetNativeResponses(bool) }); ok {
		target.SetNativeResponses(native)
	}
}

// Called under responseWriter.mu. Source errors are recorded but forwarded;
// the caller must see the original failed/incomplete response, not a replacement.
func (w *responseWriter) observeNativeResponses(data []byte) (streamMilestones, error) {
	var observed streamMilestones
	err := w.nativeDecoder.Feed(data, func(event string, raw []byte) error {
		chunk, err := w.nativeResponses.ReadEvent(event, raw)
		w.setUsage(w.nativeResponses.ReportedUsage())
		if w.nativeResponses.Model != "" {
			w.sourceModel = w.nativeResponses.Model
		}
		if err != nil && w.streamErr == nil {
			w.streamErr = err
		}
		if chunk == nil {
			return nil
		}
		observed.event = true
		observed.end = w.nativeResponses.Done()
		switch chunk.Type {
		case "response.output_text.delta", "response.reasoning_summary_text.delta", "response.refusal.delta", "response.function_call_arguments.delta", "response.custom_tool_call_input.delta":
			observed.content = chunk.Delta != "" || observed.content
		}
		if item := chunk.Item; item != nil && (item.Type == "function_call" || item.Type == "custom_tool_call") {
			observed.content = true
		}
		if chunk.Response != nil {
			for _, item := range chunk.Response.Output {
				if item.Type == "function_call" || item.Type == "custom_tool_call" {
					observed.content = true
				}
				for _, part := range item.Content {
					if part.Text != "" || part.Refusal != "" {
						observed.content = true
					}
				}
				for _, part := range item.Summary {
					if part.Text != "" {
						observed.content = true
					}
				}
			}
		}
		return nil
	})
	w.contentWritten = w.contentWritten || observed.content
	return observed, err
}

func (w *responseWriter) writeNativeError(message string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.nativeResponses.ErrorReported {
		return nil
	}
	data, _ := json.Marshal(map[string]any{"type": "error", "code": "upstream_error", "message": message, "param": nil})
	if _, err := fmt.Fprintf(w.ResponseWriter, "\n\nevent: error\ndata: %s\n\n", data); err != nil {
		return err
	}
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
	return nil
}

func nativeResponseUsage(raw []byte) *types.Usage {
	var response types.ResponsesResponse
	if json.Unmarshal(raw, &response) != nil {
		return nil
	}
	return transformer.ResponsesUsageToAnthropic(response.Usage)
}

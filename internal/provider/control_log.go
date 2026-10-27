package provider

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"github.com/routatic/proxy/internal/core"
)

// Debug-only: inspect a replay reader, never consume the live request body or
// log prompts, schemas, tool arguments, cache keys, headers or opaque state.
func logRequestControls(req *http.Request, logger *slog.Logger) {
	if !logger.Enabled(req.Context(), slog.LevelDebug) || req.GetBody == nil {
		return
	}
	metadata := core.RequestMetadataFromContext(req.Context())
	source, sourceFormat := metadata.Body, core.WireFormatAnthropic
	if len(metadata.ResponsesBody) > 0 {
		source, sourceFormat = metadata.ResponsesBody, core.WireFormatOpenAIResponses
	}
	if len(source) == 0 {
		return
	}
	body, err := req.GetBody()
	if err != nil {
		logger.Warn("cannot inspect upstream controls", "request_id", metadata.RequestID, "error", err)
		return
	}
	defer func() { _ = body.Close() }()
	requested, requestErr := controlSummary(source)
	raw, readErr := io.ReadAll(body)
	emitted, emittedErr := controlSummary(raw)
	if requestErr != nil || readErr != nil || emittedErr != nil {
		logger.Warn("cannot decode request controls", "request_id", metadata.RequestID)
		return
	}
	logger.Debug("upstream request controls", "request_id", metadata.RequestID,
		"source_format", sourceFormat.String(), "target_format", metadata.TargetFormat.String(),
		"requested", requested, "emitted", emitted,
		"policy", "wire_mapping_and_model_configuration", "effect_verified", false)
}

func controlSummary(raw []byte) (map[string]any, error) {
	type thinking struct {
		Type    string `json:"type"`
		Budget  *int   `json:"budget_tokens"`
		GBudget *int   `json:"thinkingBudget"`
		Level   string `json:"thinkingLevel"`
	}
	type output struct {
		Effort string                `json:"effort"`
		Format struct{ Type string } `json:"format"`
	}
	var in struct {
		Temperature     *float64                         `json:"temperature"`
		TopP            *float64                         `json:"top_p"`
		MaxTokens       *int                             `json:"max_tokens"`
		MaxOutputTokens *int                             `json:"max_output_tokens"`
		ReasoningEffort string                           `json:"reasoning_effort"`
		Thinking        thinking                         `json:"thinking"`
		OutputConfig    output                           `json:"output_config"`
		Text            output                           `json:"text"`
		Reasoning       struct{ Effort, Summary string } `json:"reasoning"`
		ResponseFormat  struct{ Type string }            `json:"response_format"`
		ToolChoice      json.RawMessage                  `json:"tool_choice"`
		ParallelTools   *bool                            `json:"parallel_tool_calls"`
		PromptCacheKey  string                           `json:"prompt_cache_key"`
		StopSequences   []string                         `json:"stop_sequences"`
		Stop            json.RawMessage                  `json:"stop"`
		Generation      *struct {
			Temperature *float64 `json:"temperature"`
			TopP        *float64 `json:"topP"`
			MaxTokens   *int     `json:"maxOutputTokens"`
			Thinking    thinking `json:"thinkingConfig"`
			Stop        []string `json:"stopSequences"`
			MIMEType    string   `json:"responseMimeType"`
		} `json:"generationConfig"`
		ToolConfig struct {
			FunctionCalling struct{ Mode string } `json:"functionCallingConfig"`
		} `json:"toolConfig"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, err
	}
	result := map[string]any{}
	if g := in.Generation; g != nil {
		in.Temperature, in.TopP, in.MaxTokens, in.Thinking = g.Temperature, g.TopP, g.MaxTokens, g.Thinking
		in.StopSequences = g.Stop
		if g.MIMEType != "" {
			result["response_mime_type"] = g.MIMEType
		}
	}
	if in.MaxOutputTokens != nil {
		in.MaxTokens = in.MaxOutputTokens
	}
	if in.Temperature != nil {
		result["temperature"] = *in.Temperature
	}
	if in.TopP != nil {
		result["top_p"] = *in.TopP
	}
	if in.MaxTokens != nil {
		result["max_tokens"] = *in.MaxTokens
	}
	if in.Thinking.Type != "" {
		result["thinking_mode"] = in.Thinking.Type
	}
	if in.Thinking.Budget != nil {
		result["thinking_budget"] = *in.Thinking.Budget
	}
	if in.Thinking.GBudget != nil {
		result["thinking_budget"] = *in.Thinking.GBudget
	}
	for _, effort := range []string{in.ReasoningEffort, in.OutputConfig.Effort, in.Reasoning.Effort, in.Thinking.Level} {
		if effort != "" {
			result["reasoning_effort"] = effort
		}
	}
	if in.Reasoning.Summary != "" {
		result["reasoning_summary"] = in.Reasoning.Summary
	}
	for _, format := range []string{in.ResponseFormat.Type, in.OutputConfig.Format.Type, in.Text.Format.Type} {
		if format != "" {
			result["output_format"] = format
		}
	}
	if in.ParallelTools != nil {
		result["parallel_tools"] = *in.ParallelTools
	}
	var choice string
	if json.Unmarshal(in.ToolChoice, &choice) != nil {
		var object struct {
			Type            string
			DisableParallel *bool `json:"disable_parallel_tool_use"`
		}
		if json.Unmarshal(in.ToolChoice, &object) == nil {
			choice = object.Type
			if object.DisableParallel != nil {
				result["parallel_tools"] = !*object.DisableParallel
			}
		}
	}
	if in.ToolConfig.FunctionCalling.Mode != "" {
		choice = in.ToolConfig.FunctionCalling.Mode
	}
	if choice != "" {
		result["tool_choice"] = choice
	}
	if in.StopSequences != nil {
		result["stop_count"] = len(in.StopSequences)
	}
	var stops []string
	if json.Unmarshal(in.Stop, &stops) == nil && stops != nil {
		result["stop_count"] = len(stops)
	} else {
		var stop string
		if json.Unmarshal(in.Stop, &stop) == nil && stop != "" {
			result["stop_count"] = 1
		}
	}
	result["prompt_cache_key_present"] = in.PromptCacheKey != ""
	return result, nil
}

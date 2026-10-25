// Package transformer handles request and response format conversion
// between Anthropic Messages API and OpenAI Chat Completions API,
// including streaming SSE transformation, token counting, and
// protocol bridging for Responses and Gemini endpoints.
package transformer

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/internal/models"
	"github.com/routatic/proxy/pkg/types"
)

// contentText is a convenience wrapper around types.TextContent for brevity
// at call sites that construct ChatMessage values.
func contentText(s string) json.RawMessage {
	return types.TextContent(s)
}

// RequestTransformer converts Anthropic requests to OpenAI format.
type RequestTransformer struct{}

// NewRequestTransformer creates a new request transformer.
func NewRequestTransformer() *RequestTransformer {
	return &RequestTransformer{}
}

// isThinkingDisabled checks if the thinking JSON config explicitly sets type to "disabled".
func isThinkingDisabled(thinking json.RawMessage) bool {
	var m map[string]interface{}
	if err := json.Unmarshal(thinking, &m); err != nil {
		return false
	}
	t, ok := m["type"].(string)
	return ok && t == "disabled"
}

// modelFamily reduces an upstream model id to the bare family name. The
// normaliser lives in internal/models so the peak-billing rules and these
// request-shaping rules cannot drift apart on how an id is spelled.
func modelFamily(modelID string) string {
	return models.ModelFamily(models.BareModelID(modelID))
}

// isDeepSeekModel returns true for DeepSeek models that require thinking mode handling.
func isDeepSeekModel(modelID string) bool {
	return strings.HasPrefix(modelFamily(modelID), "deepseek")
}

// isOpenAIReasoningModel returns true for OpenAI o1 and o3 models.
func isOpenAIReasoningModel(modelID string) bool {
	family := modelFamily(modelID)
	return strings.HasPrefix(family, "o1-") || strings.HasPrefix(family, "o3-")
}

// isMoonshotModel returns true for Moonshot/Kimi models, which reject
// cache_control on the system block and only accept temperature=1.
func isMoonshotModel(modelID string) bool {
	return strings.HasPrefix(modelFamily(modelID), "kimi")
}

// needsPlaceholderReasoning returns true for providers whose validators require
// a non-empty reasoning_content field on assistant tool-call messages.
func needsPlaceholderReasoning(modelID string) bool {
	// Moonshot's validator treats an empty string as missing.
	return isMoonshotModel(modelID)
}

// constrainTemperature overrides model-specific temperature constraints.
// Some models require specific temperature values — return the constrained
// value or the original if no constraint applies.
func constrainTemperature(modelID string, temp float64) float64 {
	// Moonshot AI (kimi-k2.7-code) only allows temperature=1.
	if modelFamily(modelID) == "kimi-k2.7-code" {
		return 1.0
	}
	return temp
}

// stripCacheControl removes cache_control from all messages in the list.
// The caller must not hold references to the slice elements.
func stripCacheControl(messages []types.ChatMessage) {
	for i := range messages {
		messages[i].CacheControl = nil
	}
}

// TransformRequest converts an Anthropic MessageRequest to OpenAI ChatCompletionRequest.
func (t *RequestTransformer) TransformRequest(
	anthropicReq *types.MessageRequest,
	model config.ModelConfig,
) (*types.ChatCompletionRequest, error) {
	// Transform messages
	messages, err := t.transformMessages(anthropicReq, model.ModelID, imageHandlingFor(model))
	if err != nil {
		return nil, fmt.Errorf("failed to transform messages: %w", err)
	}

	// Strip cache_control for models that don't support it
	if !isDeepSeekModel(model.ModelID) {
		stripCacheControl(messages)
	}

	// Build OpenAI request
	openaiReq := &types.ChatCompletionRequest{
		Model:    model.ModelID,
		Messages: messages,
		Stream:   anthropicReq.Stream,
	}
	if anthropicReq.Stream != nil && *anthropicReq.Stream {
		openaiReq.StreamOptions = &types.StreamOptions{IncludeUsage: true}
	}

	// Copy optional parameters from Anthropic request
	if anthropicReq.Temperature != nil {
		openaiReq.Temperature = anthropicReq.Temperature
	}
	if anthropicReq.TopP != nil {
		openaiReq.TopP = anthropicReq.TopP
	}

	// Map max_tokens
	if anthropicReq.MaxTokens > 0 {
		maxTokens := anthropicReq.MaxTokens
		openaiReq.MaxTokens = &maxTokens
	}

	// Apply model-specific overrides and temperature constraints
	if model.Temperature > 0 {
		openaiReq.Temperature = &model.Temperature
	}
	if openaiReq.Temperature != nil {
		temp := constrainTemperature(model.ModelID, *openaiReq.Temperature)
		openaiReq.Temperature = &temp
	}
	if model.MaxTokens > 0 {
		maxTokens := model.MaxTokens
		openaiReq.MaxTokens = &maxTokens
	}

	// Parse output_config once, before the thinking decision. Both fields it
	// carries feed decisions that used to be made in two places: the effort is
	// the caller's own words about strength, and it must be resolved together
	// with `thinking` rather than written over it afterwards. Writing it
	// afterwards is what let one payload carry `thinking` and
	// `reasoning_effort` at the same time - two controls for one decision,
	// which gateways such as OpenCode Go reject outright.
	var statedEffort string
	if len(anthropicReq.OutputConfig) > 0 {
		var output struct {
			Effort string          `json:"effort"`
			Format json.RawMessage `json:"format"`
		}
		if err := json.Unmarshal(anthropicReq.OutputConfig, &output); err != nil {
			return nil, fmt.Errorf("invalid output_config: %w", err)
		}
		if len(output.Format) > 0 && string(output.Format) != "null" {
			responseFormat, err := openAIResponseFormat(output.Format)
			if err != nil {
				return nil, err
			}
			openaiReq.ResponseFormat = responseFormat
		}
		// MessageRequest also carries Responses input; OpenAI output preserves
		// its effort level. Only native Anthropic output needs a mapping.
		//
		// "adaptive" and "auto" are modes, not rungs: they say "let the model
		// decide", which no reasoning_effort field can express. Treating them
		// as an error rejected a request the caller had phrased legitimately;
		// treating them as a level would invent one. They resolve to "no
		// stated level" instead, so the model's own default applies.
		if mode := strings.ToLower(strings.TrimSpace(output.Effort)); mode == "adaptive" || mode == "auto" {
			statedEffort = ""
		} else {
			effort, ok := core.NormalizeEffort(output.Effort)
			if !ok {
				return nil, fmt.Errorf("unsupported output_config.effort %q", output.Effort)
			}
			statedEffort = effort
		}
	}

	// Determine thinking and reasoning_effort for the upstream request.
	// Priority: explicit config → history continuity → safety guard.
	//
	// The safety guard (thinking: disabled) only engages when the history
	// contains assistant messages that lack thinking blocks — DeepSeek
	// validates reasoning_content on every assistant message in thinking
	// mode and will 400 if any are missing.  On a first turn (no assistant
	// messages) or when the user explicitly opts in via config, we send
	// thinking: enabled so the model can produce reasoning.
	resolveThinkingAndEffort(anthropicReq, model, openaiReq, statedEffort)

	// Transform tools if present
	if len(anthropicReq.Tools) > 0 {
		openaiReq.Tools = t.transformTools(anthropicReq.Tools)
	}
	if len(anthropicReq.ToolChoice) > 0 {
		var choice struct {
			Type            string `json:"type"`
			Name            string `json:"name"`
			DisableParallel *bool  `json:"disable_parallel_tool_use"`
		}
		if err := json.Unmarshal(anthropicReq.ToolChoice, &choice); err != nil {
			return nil, fmt.Errorf("invalid tool_choice: %w", err)
		}
		switch choice.Type {
		case "auto", "none":
			openaiReq.ToolChoice = choice.Type
		case "any":
			openaiReq.ToolChoice = "required"
		case "tool":
			if strings.TrimSpace(choice.Name) == "" {
				return nil, fmt.Errorf("tool_choice.name is required for a named tool")
			}
			openaiReq.ToolChoice = map[string]any{"type": "function", "function": map[string]string{"name": choice.Name}}
		default:
			return nil, fmt.Errorf("unsupported tool_choice type %q", choice.Type)
		}
		if choice.DisableParallel != nil {
			parallel := !*choice.DisableParallel
			openaiReq.ParallelToolCalls = &parallel
		}
	}

	return openaiReq, nil
}

// openAIResponseFormat maps Anthropic's output_config.format onto the OpenAI
// Chat Completions response_format the upstream actually receives.
//
// Anthropic defines a single variant, {"type":"json_schema","schema":{...}},
// and requires the schema to be closed (additionalProperties:false with every
// property required), which is exactly what OpenAI's strict mode demands - so
// the mapping is lossless. Anything else is rejected instead of dropped, so a
// caller never silently loses the structured-output guarantee it asked for.
func openAIResponseFormat(format json.RawMessage) (json.RawMessage, error) {
	var parsed struct {
		Type   string          `json:"type"`
		Schema json.RawMessage `json:"schema"`
	}
	if err := json.Unmarshal(format, &parsed); err != nil {
		return nil, fmt.Errorf("invalid output_config.format: %w", err)
	}
	if parsed.Type != "json_schema" || len(parsed.Schema) == 0 || string(parsed.Schema) == "null" {
		return nil, fmt.Errorf("unsupported output_config.format type %q", parsed.Type)
	}
	return json.Marshal(map[string]any{
		"type": "json_schema",
		"json_schema": map[string]any{
			"name":   "response",
			"schema": parsed.Schema,
			"strict": true,
		},
	})
}

// HasThinkingBlocks returns true if any assistant message contains
// thinking content — either as a dedicated `thinking`-typed block, or
// attached as a non-empty `thinking` field on a `tool_use` block.
//
// Claude Code emits both shapes: dedicated thinking blocks for text-only
// reasoning, and tool_use blocks with an inline `thinking` field when the
// assistant turn ends in a tool call. Both forms must mark the
// conversation as having thinking history so the proxy enables thinking
// mode on subsequent upstream calls (DeepSeek defaults to thinking mode
// and demands `reasoning_content` once it's been engaged).
func HasThinkingBlocks(messages []types.Message) bool {
	for _, msg := range messages {
		if msg.Role != "assistant" {
			continue
		}
		for _, block := range msg.ContentBlocks() {
			if block.Type == "thinking" {
				return true
			}
			if block.Type == "tool_use" && block.Thinking != "" {
				return true
			}
		}
	}
	return false
}

// parseBudgetTokens extracts budget_tokens from a thinking JSON field.
func parseBudgetTokens(thinking json.RawMessage) int {
	var m struct {
		BudgetTokens int `json:"budget_tokens"`
	}
	if err := json.Unmarshal(thinking, &m); err != nil {
		return 0
	}
	return m.BudgetTokens
}

// resolveThinkingAndEffort applies thinking/reasoning_effort to the OpenAI
// request. Decision priority:
//
//  1. Client request — anthropicReq.Thinking set and not disabled
//     → forward thinking config; map budget_tokens to reasoning_effort.
//  2. History continuity — a prior turn used thinking → keep it enabled.
//  3. Explicit config — model.Thinking set → use it verbatim.
//  4. Config intent — model.ReasoningEffort set without model.Thinking
//     → enable on first turn (no assistant messages), disable only when
//     safety guard fires (DeepSeek + history assistant msgs lack thinking).
//  5. No config, no history → leave both unset (safety guard for DeepSeek).
func resolveThinkingAndEffort(
	anthropicReq *types.MessageRequest,
	model config.ModelConfig,
	openaiReq *types.ChatCompletionRequest,
	statedEffort string,
) {
	hasThinking := HasThinkingBlocks(anthropicReq.Messages)
	hasAssistant := hasAssistantMessages(anthropicReq.Messages)
	explicitThinking := len(model.Thinking) > 0
	explicitEffort := model.ReasoningEffort != ""
	isDeepSeek := isDeepSeekModel(model.ModelID)
	isOpenAIReasoning := isOpenAIReasoningModel(model.ModelID)
	requestThinkingDisabled := isThinkingDisabled(anthropicReq.Thinking)
	requestThinking := !requestThinkingDisabled && len(anthropicReq.Thinking) > 0

	// The capability table decides whether the `thinking` object may be sent,
	// not the family name. Until 2026-10-26 this read `isDeepSeek ||
	// explicitThinking`, so every non-DeepSeek model silently lost the client's
	// switch - GLM, Qwen and MiMo all accept it, and the table says so.
	//
	// An unknown model keeps the old behaviour on purpose: the table's zero
	// value has TakesThinking false, and forwarding a field to an upstream
	// nobody has measured is the failure this table exists to prevent. The
	// operator can declare it.
	capability := core.ReasoningSupportFor(model)
	allowThinkingParam := capability.TakesThinking || explicitThinking
	allowEffortParam := isOpenAIReasoning || isDeepSeek || explicitEffort || len(capability.Levels) > 0

	// A caller who turns thinking off gets nothing else. The effort that used
	// to be written after this function could resurrect a level here, so the
	// request said "disabled" and the payload said "high" - the opposite of
	// what was asked.
	if requestThinkingDisabled {
		if allowThinkingParam {
			openaiReq.Thinking = thinkingForCapability(anthropicReq.Thinking, capability, isDeepSeek)
		}
		return
	}

	// DeepSeek validates reasoning_content on every assistant message in
	// thinking mode, so a history without it must run with thinking off. This
	// outranks a stated level: the guard exists because the upstream refuses
	// the request otherwise, and honouring the level here produced a payload
	// that only set reasoning_effort and left thinking on - the exact
	// combination that 400s.
	if isDeepSeek && hasAssistant && !hasThinking {
		if allowThinkingParam {
			openaiReq.Thinking = json.RawMessage(`{"type":"disabled"}`)
		}
		return
	}

	// A level the caller stated is the strongest signal this request carries,
	// and it replaces the budget rather than joining it: one decision, one
	// control. Sending both is what makes gateways such as OpenCode Go answer
	// 400, and it lets the two disagree - a 8192 budget is "medium" by this
	// proxy's own ladder while the stated level may say "high".
	//
	// The gate is a deny-list, not an allow-list: a model this proxy has never
	// heard of still gets the level the caller asked for (unknown is not
	// "no"), while the one family measured to reject the field outright
	// (Moonshot) never receives it. A per-model capability table replaces this
	// deny-list once the families that accept a level are measured one by one.
	if statedEffort != "" {
		// "off" is not a rung. A model whose ladder has no "none" must be told
		// through the switch, because the closest level would say "think a
		// little" where the caller said "stop" - the opposite instruction.
		if isOffEffort(statedEffort) && allowThinkingParam {
			openaiReq.Thinking = json.RawMessage(`{"type":"disabled"}`)
			return
		}
		// A model with no switch at all (the GPT family) cannot be told to
		// stop through either control. Its lowest rung is the closest honest
		// answer, and the clamp below produces it.
		if !rejectsEffort(model.ModelID) {
			// A model whose ladder is known gets the nearest rung it actually
			// has: every vendor that documents a ladder also documents that an
			// unsupported value is an error. A model this proxy knows nothing
			// about keeps the caller's own words.
			effort := statedEffort
			support := core.ReasoningSupportFor(model)
			if clamped, ok := support.ClampLevel(statedEffort); ok {
				effort = clamped
			}
			// A level the caller stated and did not get is the single most
			// confusing thing this proxy can do quietly: they asked for max,
			// the model ran at high, and nothing said so. One line makes it
			// answerable.
			logEffectiveEffort(model.ModelID, statedEffort, effort, support)
			openaiReq.ReasoningEffort = &effort
		}
		return
	}

	switch {
	case requestThinking:
		// Client explicitly opted into thinking mode via the request
		// (e.g., effortLevel in Claude Code sends thinking: {type:"enabled", budget_tokens:N}).
		//
		// The budget is the precise statement of intent and it is forwarded
		// verbatim. A derived effort level is only sent when the upstream does
		// not take a budget: sending both hands the upstream two controls for
		// one decision, and whichever it honours, the other is a contradiction.
		// Deriving a level also loses resolution - a 63999-token budget becomes
		// one of five strings.
		if allowThinkingParam {
			openaiReq.Thinking = thinkingForCapability(anthropicReq.Thinking, capability, isDeepSeek)
		}
		// On an effort-driven gateway a budget saturates below the top rung:
		// measured 2026-10-26, budgets of 8192 / 32000 / 64000 averaged 216 /
		// 306 / 295 reasoning tokens while reasoning_effort low / high / max
		// averaged 250 / 305 / 462. The budget is honoured and then flattens,
		// so a caller who states a large one still needs the level to reach
		// max. The thinking block rides along because it carries the on/off
		// switch - the same request with thinking disabled produced zero
		// reasoning tokens.
		support := core.ReasoningSupportFor(model)
		if allowEffortParam && (!allowThinkingParam || support.EffortDriven) {
			if budget := parseBudgetTokens(anthropicReq.Thinking); budget > 0 {
				derived := core.EffortFromBudget(budget)
				effort := derived
				if clamped, ok := support.ClampLevel(derived); ok {
					effort = clamped
				}
				logEffectiveEffort(model.ModelID, "budget:"+strconv.Itoa(budget)+"->"+derived, effort, support)
				openaiReq.ReasoningEffort = &effort
			}
		}

	case hasThinking:
		// History has thinking blocks — maintain continuity.
		if allowThinkingParam {
			if explicitThinking {
				openaiReq.Thinking = model.Thinking
			} else {
				openaiReq.Thinking = json.RawMessage(`{"type":"enabled"}`)
			}
		}
		if allowEffortParam {
			if !isThinkingDisabled(openaiReq.Thinking) || !isDeepSeek {
				setReasoningEffort(openaiReq, model)
			}
		}

	case explicitThinking:
		// Config explicitly sets thinking — respect it.
		if allowThinkingParam {
			openaiReq.Thinking = model.Thinking
		}
		if allowEffortParam {
			if !isThinkingDisabled(openaiReq.Thinking) || !isDeepSeek {
				setReasoningEffort(openaiReq, model)
			}
		}

	case explicitEffort:
		// User set reasoning_effort but not thinking. Intent is clear.
		if allowThinkingParam {
			openaiReq.Thinking = json.RawMessage(`{"type":"enabled"}`)
		}
		if allowEffortParam {
			setReasoningEffort(openaiReq, model)
		}

	default:
		// No config, no history: leave both unset.
	}
}

// logEffectiveEffort records a level the proxy actually sent when it differs
// from what the request asked for, or when the model needed the level at all.
//
// It logs only on a change. A line per request would bury the signal; a line
// per rewrite answers the question people actually ask - "I set max, why does
// this look like high" - without adding noise to the common case.
func logEffectiveEffort(modelID, requested, effective string, support core.ThinkingSupport) {
	if requested == effective {
		return
	}
	slog.Info("reasoning level adjusted",
		"model", modelID,
		"requested", requested,
		"effective", effective,
		"supported", strings.Join(support.Levels, ","),
	)
}

// isOffEffort reports whether a level means "do not think". "minimal" is not
// one of them: it asks for the weakest real effort, which is a different
// instruction with a different outcome.
func isOffEffort(level string) bool {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "none", "off", "disabled":
		return true
	default:
		return false
	}
}

// rejectsEffort reports whether the model is one that answers 400 for the
// reasoning_effort field itself.
//
// The list is deliberately short and endpoint-specific. Moonshot's own API
// rejects the field, but a kimi model reached through a gateway is a different
// endpoint that accepts it - which is why this asks about the model *and* the
// realm rather than about the family alone.
func rejectsEffort(modelID string) bool {
	if !isMoonshotModel(modelID) {
		return false
	}
	// Only the bare id means api.moonshot.ai, which rejects the field. A prefix
	// of either kind - a pool ("cline-pass/") or a realm ("global:") - means the
	// request reaches the model through a gateway, and those gateways accept it.
	// Testing only the colon form is what left cline-pass/kimi-k3 with no
	// reasoning control at all: nothing was sent, so the caller's choice was
	// silently dropped by a deny-list written for a different endpoint.
	return !strings.ContainsAny(modelID, "/:")
}

// thinkingForCapability adapts a thinking object to what the target defines.
//
// Two edits, both because the client's object can carry more than the target
// knows:
//   - a token budget is dropped when the model takes the switch but not the
//     budget. Kimi K2.6 is the case: its thinking object defines type and keep,
//     and a budget_tokens riding along is a field it never declared.
//   - "adaptive" becomes "enabled" for DeepSeek, whose thinking object defines
//     only enabled/disabled.
//
// Everything else is forwarded untouched. This is not a normaliser.
func thinkingForCapability(raw json.RawMessage, support core.ThinkingSupport, isDeepSeek bool) json.RawMessage {
	if len(raw) == 0 {
		return raw
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return raw
	}
	changed := false
	if !support.Budget {
		if _, ok := fields["budget_tokens"]; ok {
			delete(fields, "budget_tokens")
			changed = true
		}
	}
	if isDeepSeek {
		var probe struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &probe); err == nil && strings.EqualFold(probe.Type, "adaptive") {
			fields["type"] = json.RawMessage(`"enabled"`)
			changed = true
		}
	}
	if !changed {
		return raw
	}
	converted, err := json.Marshal(fields)
	if err != nil {
		return raw
	}
	return converted
}

// setReasoningEffort sets reasoning_effort on the request, defaulting to
// "high" when the config value is empty.
func setReasoningEffort(openaiReq *types.ChatCompletionRequest, model config.ModelConfig) {
	effort := model.ReasoningEffort
	if effort == "" {
		defaultEffort := "high"
		openaiReq.ReasoningEffort = &defaultEffort
		return
	}
	// A configuration typo must not become an upstream field: the model would
	// answer 400 and nothing in the request would point back at the config.
	// Dropping it leaves the request at the model's own default, which is the
	// same place an unset value lands.
	if _, ok := core.NormalizeEffort(effort); !ok {
		return
	}
	if clamped, ok := core.ReasoningSupportFor(model).ClampLevel(effort); ok {
		effort = clamped
	}
	openaiReq.ReasoningEffort = &effort
}

// hasAssistantMessages returns true when the conversation contains at least
// one assistant message.
func hasAssistantMessages(messages []types.Message) bool {
	for _, msg := range messages {
		if msg.Role == "assistant" {
			return true
		}
	}
	return false
}

// imagePolicy says what to do with an image block for a given model.
//
// It replaced a plain "vision bool", which could not distinguish "this model
// refuses images" from "nothing here says" - and the zero value fell on the
// refusing side. That is how a vision-capable model ended up sent a "[Image]"
// placeholder instead of the picture: the request succeeded and the model
// answered about seven characters of text.
type imagePolicy int

const (
	// imagesForward sends the bytes upstream. Both a known-capable and an
	// unknown model get this: the platform is the only party that actually
	// knows, and a wrong guess costs a 400 the client can read, where a
	// placeholder costs a confident answer about nothing.
	imagesForward imagePolicy = iota
	// imagesPlaceholder replaces the image with "[Image]" for a model known to
	// refuse them, so the rest of the conversation still reaches the model.
	imagesPlaceholder
)

func imageHandlingFor(model config.ModelConfig) imagePolicy {
	if vision, known := config.ModelHasVision(model); known && !vision {
		return imagesPlaceholder
	}
	return imagesForward
}

// transformMessages converts Anthropic messages to OpenAI format.
func (t *RequestTransformer) transformMessages(anthropicReq *types.MessageRequest, modelID string, images imagePolicy) ([]types.ChatMessage, error) {
	hasThinking := HasThinkingBlocks(anthropicReq.Messages)

	var result []types.ChatMessage

	// Add system message from top-level field if present, preserving cache_control.
	// DeepSeek V3.x / V4 reorders all system-role messages to the front internally.
	// When Claude Code injects periodic system reminders mid-conversation, any
	// extra {"role": "system"} in the messages array would shift every subsequent
	// user/assistant/tool message after the reorder, blowing the prefix cache.
	systemText := anthropicReq.SystemText()
	if systemText != "" {
		systemMsg := types.ChatMessage{
			Role:    "system",
			Content: contentText(systemText),
		}
		if !isMoonshotModel(modelID) && len(anthropicReq.System) > 0 {
			var blocks []types.SystemContentBlock
			if err := json.Unmarshal(anthropicReq.System, &blocks); err == nil {
				for _, b := range blocks {
					if b.Type == "text" && b.CacheControl != nil {
						systemMsg.CacheControl = b.CacheControl
						break
					}
				}
			}
		}
		result = append(result, systemMsg)
	}

	// Transform remaining messages.
	//
	// DeepSeek V3.x / V4 internally reorders all system-role messages to the
	// front of the effective prompt.  When Claude Code injects periodic system
	// reminders mid-conversation (e.g. "task tools haven't been used recently"),
	// forwarding them as {"role": "system"} would cause DeepSeek's reordering
	// to shift every user/assistant/tool message, invalidating the prefix cache
	// from the insertion point onward.
	//
	// For DeepSeek models only, convert non-top-level system messages into user
	// messages wrapped in <system-reminder> tags.  This preserves the semantic
	// intent while preventing DeepSeek from reordering them past the
	// conversational history.
	rewriteSystem := isDeepSeekModel(modelID)
	for _, msg := range anthropicReq.Messages {
		if msg.Role == "system" && rewriteSystem {
			blocks := msg.ContentBlocks()
			var sb strings.Builder
			for _, b := range blocks {
				if b.Type == "text" {
					sb.WriteString(b.Text)
				}
			}
			text := sb.String()
			if text == "" {
				continue
			}
			// Deduplicate: skip if this text is already part of the top-level
			// system prompt (matched after trimming whitespace on both sides).
			if canonicalSystem := strings.TrimSpace(systemText); canonicalSystem != "" {
				canonicalText := strings.TrimSpace(text)
				if strings.Contains(canonicalSystem, canonicalText) {
					continue
				}
			}
			result = append(result, types.ChatMessage{
				Role:    "user",
				Content: contentText("<system-reminder>\n" + text + "\n</system-reminder>"),
			})
			continue
		}
		openaiMsgs, err := t.transformMessage(msg, modelID, hasThinking, images)
		if err != nil {
			return nil, err
		}
		result = append(result, openaiMsgs...)
	}

	return result, nil
}

// transformMessage converts a single Anthropic message to one or more OpenAI messages.
// Tool_use and tool_result require special handling to map to OpenAI's function calling format.
func (t *RequestTransformer) transformMessage(msg types.Message, modelID string, hasThinkingInHistory bool, images imagePolicy) ([]types.ChatMessage, error) {
	blocks := msg.ContentBlocks()

	switch msg.Role {
	case "user":
		return t.transformUserMessage(blocks, images)
	case "assistant":
		return t.transformAssistantMessage(blocks, modelID, hasThinkingInHistory)
	default:
		// Fallback: concatenate all text
		var text string
		for _, b := range blocks {
			if b.Type == "text" {
				text += b.Text
			}
		}
		return []types.ChatMessage{{Role: msg.Role, Content: contentText(text)}}, nil
	}
}

// transformUserMessage converts a user message with potential tool_result and image blocks.
// Image blocks are converted to OpenAI's multimodal content format (content array
// with image_url parts) so that vision-capable models receive the actual image data.
// For models without vision support, image blocks are replaced with a "[Image]" text
// placeholder to prevent upstream 400 errors from unsupported image_url parts.
func (t *RequestTransformer) transformUserMessage(blocks []types.ContentBlock, images imagePolicy) ([]types.ChatMessage, error) {
	var result []types.ChatMessage
	var textParts []string
	var parts []types.ChatContentPart
	visionImage := false
	hasImage := false

	for _, block := range blocks {
		switch block.Type {
		case "text":
			textParts = append(textParts, block.Text)
			parts = append(parts, types.ChatContentPart{Type: "text", Text: block.Text})
		case "tool_result":
			// Only the text parts of a tool result can be sent over Chat
			// Completions. Anything else has no representation, so it is dropped
			// the same way every other unmapped block type is: TextContent
			// concatenates the text parts, and a result carrying only
			// unrepresentable parts (Claude Code's ToolSearch returns a
			// tool_reference block) becomes an empty tool message rather than
			// failing the request.
			//
			// This mirrors the reference CommandCode proxy, which maps the same
			// block to "". The trade is deliberate: the request succeeds and the
			// model proceeds without the tool's content, rather than the whole
			// chain failing on a block that cannot be expressed.
			toolContent := block.TextContent()
			result = append(result, types.ChatMessage{
				Role:       "tool",
				Content:    contentText(toolContent),
				ToolCallID: block.GetToolID(),
			})
		case "image":
			if block.Source == nil {
				return nil, fmt.Errorf("image source is required")
			}
			switch images {
			case imagesForward:
				imageURL, err := imageSourceURL(block.Source)
				if err != nil {
					return nil, err
				}
				visionImage = true
				parts = append(parts, types.ChatContentPart{
					Type: "image_url",
					ImageURL: &types.ImageURL{
						URL: imageURL,
					},
				})
			default:
				hasImage = true
			}
		}
	}

	// If there's text or image content, add it as a user message.
	// OpenAI-compatible tool calling requires tool responses to appear
	// immediately after the assistant message that emitted tool_calls.
	// If the Anthropic user turn also includes free-form text and/or images,
	// emit it as a subsequent user message after all tool results.
	if len(textParts) > 0 || visionImage || hasImage {
		if visionImage {
			// Keep image/text interleaving, which determines what each caption describes.
			contentJSON, err := json.Marshal(parts)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal multimodal content: %w", err)
			}
			result = append(result, types.ChatMessage{Role: "user", Content: contentJSON})
		} else {
			// Text-only message (possibly with image placeholder for non-vision models)
			text := strings.Join(textParts, "")
			if hasImage {
				if text != "" {
					text += "\n\n[Image]"
				} else {
					text = "[Image]"
				}
			}
			result = append(result, types.ChatMessage{
				Role:    "user",
				Content: contentText(text),
			})
		}
	}

	return result, nil
}

func imageSourceURL(source *types.ImageSource) (string, error) {
	if source == nil {
		return "", fmt.Errorf("image source is required")
	}
	switch source.Type {
	case "url":
		if source.URL == "" {
			return "", fmt.Errorf("image source URL is required")
		}
		return source.URL, nil
	case "base64", "":
		if source.MediaType == "" || source.Data == "" {
			return "", fmt.Errorf("base64 image media_type and data are required")
		}
		return fmt.Sprintf("data:%s;base64,%s", source.MediaType, source.Data), nil
	default:
		return "", fmt.Errorf("unsupported image source type %q", source.Type)
	}
}

// transformAssistantMessage converts an assistant message with potential tool_use blocks.
func (t *RequestTransformer) transformAssistantMessage(blocks []types.ContentBlock, modelID string, hasThinkingInHistory bool) ([]types.ChatMessage, error) {
	var textParts []string
	var thinkingParts []string
	var toolCalls []types.ToolCall

	for _, block := range blocks {
		switch block.Type {
		case "text":
			textParts = append(textParts, block.Text)
		case "thinking":
			// Preserve chain-of-thought so it can be forwarded back to providers
			// that require reasoning_content to be preserved across turns.
			if block.Thinking != "" {
				thinkingParts = append(thinkingParts, block.Thinking)
			}
		case "tool_use":
			// Claude Code can attach reasoning directly to the tool_use block
			// (instead of emitting a separate thinking-typed block) when the
			// assistant turn ends in a tool call. Extract that here so it
			// round-trips back to upstream as reasoning_content — otherwise
			// DeepSeek (which always operates in thinking mode after the
			// first reasoning response) returns 400 on the next request.
			if block.Thinking != "" {
				thinkingParts = append(thinkingParts, block.Thinking)
			}
			arguments := "{}"
			if len(block.Input) > 0 {
				arguments = string(block.Input)
			}
			toolCalls = append(toolCalls, types.ToolCall{
				ID:   block.ID,
				Type: "function",
				Function: types.FunctionCall{
					Name:      block.Name,
					Arguments: arguments,
				},
			})
		}
	}

	// Build the assistant message
	content := ""
	for _, p := range textParts {
		content += p
	}
	reasoningContent := ""
	for _, p := range thinkingParts {
		reasoningContent += p
	}

	var reasoningContentPtr *string
	if reasoningContent != "" {
		// Real thinking content from the upstream history — preserve it.
		reasoningContentPtr = &reasoningContent
	} else if hasThinkingInHistory && isDeepSeekModel(modelID) {
		// DeepSeek in thinking mode requires reasoning_content on EVERY
		// assistant message — text-only continuation turns and tool_use
		// turns alike — whenever the conversation was opened in thinking
		// mode. Without this, upstream returns:
		//   400 invalid_request_error: "The `reasoning_content` in the
		//   thinking mode must be passed back to the API."
		// Use a single-space placeholder for assistant turns whose original
		// thinking blocks were stripped by Claude Code (compact summaries,
		// dropped reasoning blocks, etc.) — DeepSeek checks for the field's
		// presence and non-empty content, not its semantic value.
		placeholder := " "
		reasoningContentPtr = &placeholder
	} else if len(toolCalls) > 0 && needsPlaceholderReasoning(modelID) {
		// Moonshot's validator treats an empty string as missing, so use a
		// non-empty placeholder when we must provide the field.
		placeholder := " "
		reasoningContentPtr = &placeholder
	}

	msg := types.ChatMessage{
		Role:             "assistant",
		Content:          contentText(content),
		ReasoningContent: reasoningContentPtr,
		ToolCalls:        toolCalls,
	}

	return []types.ChatMessage{msg}, nil
}

// transformTools converts Anthropic tools to OpenAI tools.
func (t *RequestTransformer) transformTools(tools []types.Tool) []types.ToolDef {
	var result []types.ToolDef

	for _, tool := range tools {
		if strings.TrimSpace(tool.Name) == "" {
			continue
		}
		// InputSchema is already json.RawMessage, use it directly
		schema := tool.InputSchema
		switch {
		case len(schema) == 0, string(schema) == "null", string(schema) == "{}":
			schema = []byte(`{"type":"object","properties":{},"additionalProperties":false}`)
		default:
			var schemaObj map[string]interface{}
			if err := json.Unmarshal(schema, &schemaObj); err != nil {
				schema = []byte(`{"type":"object","properties":{},"additionalProperties":false}`)
			} else {
				// Valid JSON " null " unmarshals to a nil map, which would panic
				// on the field assignments below.
				if schemaObj == nil {
					schema = []byte(`{"type":"object","properties":{},"additionalProperties":false}`)
				} else {
					// Validate type field is "object" — otherwise OpenAI rejects the
					// tool. A schema like {"type":"string"} passes unmarshal but
					// produces a 400 from the upstream OpenAI-compatible endpoint.
					schemaType, _ := schemaObj["type"].(string)
					if schemaType != "object" {
						schemaObj["type"] = "object"
					}

					// Validate properties is an object — wrong shapes like arrays
					// or primitives also produce 400 errors upstream.
					if props, ok := schemaObj["properties"]; ok {
						if _, valid := props.(map[string]interface{}); !valid {
							schemaObj["properties"] = map[string]interface{}{}
						}
					} else {
						schemaObj["properties"] = map[string]interface{}{}
					}

					if fixed, err := json.Marshal(schemaObj); err == nil {
						schema = fixed
					}
				}
			}
		}

		result = append(result, types.ToolDef{
			Type: "function",
			Function: types.FunctionDef{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  json.RawMessage(schema),
			},
		})
	}

	return result
}

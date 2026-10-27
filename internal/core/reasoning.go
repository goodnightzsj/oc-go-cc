package core

import (
	"strings"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/site"
)

// Reasoning control vocabulary.
//
// Upstreams do not share one vocabulary for "how hard should the model think".
// Some take a token budget (Gemini, older Claude), some take a discrete level
// (OpenAI, Kimi K3, DeepSeek's newer surface), some take only an on/off switch,
// and some take a level next to a budget. This file owns the two conversions
// and the per-family facts those conversions need, so no caller invents its own
// threshold or its own idea of which family accepts what.
//
// It deliberately does NOT model every provider nuance the large gateways do.
// The table below is the set of families this proxy actually routes to, and a
// family absent from it is "unknown": callers forward what the client sent
// rather than guessing, because unknown is not the same as "no".

// splitRealm separates a realm prefix from the model name. WorkBuddy names its
// models "global:..." and "cn:...", and the two realms do not publish the same
// effort ladder for the same model, so the realm is part of the capability
// lookup rather than decoration on the id.
func splitRealm(modelID string) (family, realm string) {
	name := strings.ToLower(strings.TrimSpace(modelID))
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		name = name[idx+1:]
	}
	if prefix, rest, found := strings.Cut(name, ":"); found && rest != "" {
		if prefix == "global" || prefix == "cn" {
			return rest, prefix
		}
	}
	return name, ""
}

// platformReasoningSupport answers for platforms whose forwarding path decides
// the control, regardless of what the underlying model accepts when called
// directly.
//
// ClinePass is the measured case: it accepts both fields, answers 200, and
// neither moves the reasoning-token count beyond the platform's own natural
// variance (144-439 across four control samples). Saying "no levels, no budget"
// is more honest than forwarding a control that does nothing - and it is what
// lets an operator override it once a future version starts honouring one.
func platformReasoningSupport(provider string) (ThinkingSupport, bool) {
	switch site.Normalize(provider) {
	case site.ClinePass:
		// No ladder. Returning the family table here would clamp a caller's
		// "max" to "high" on a platform where neither value was measured to
		// change the outcome - it would discard the caller's word for a claim
		// nobody measured. An empty capability makes ClampLevel decline, which
		// means the level is forwarded as stated.
		return ThinkingSupport{}, true
	default:
		return ThinkingSupport{}, false
	}
}

// BudgetForEffort is the inverse of EffortFromBudget: the token budget that
// represents a level. It exists because a level-only client talking to a
// budget-only upstream has no other way to express its intent - without this
// the level is simply dropped.
//
// The values are the floor of each band in EffortFromBudget, so a budget that
// is converted to a level and back lands on a number that reads back as the
// same level. They are not a claim about what any particular model does with
// the number; that is the model's business.
func BudgetForEffort(level string) int {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "minimal", "low":
		return 2048
	case "medium":
		return 8192
	case "high":
		return 32768
	case "xhigh", "max":
		return 64000
	default:
		return 0
	}
}

// ThinkingSupport describes what one model accepts for reasoning control.
//
// The zero value means "nothing is known", which is why every field is phrased
// as a permission rather than a restriction: a caller that cannot answer
// forwards the client's own fields.
type ThinkingSupport struct {
	// Levels the model accepts, low to high. Empty means the model does not
	// take a discrete level.
	Levels []string
	// Budget means the model takes thinking.budget_tokens. When both Budget
	// and Levels are set the model accepts either, not both at once.
	Budget bool
	// TakesThinking means the model accepts a `thinking` object on the Chat
	// Completions wire at all - the switch that turns reasoning on or off.
	// It is separate from Budget because a model can take the switch without
	// taking a token budget: GLM reads thinking.type and nothing else, and
	// conflating the two is what made the old gate forward thinking to
	// DeepSeek only.
	TakesThinking bool
	// CannotDisable marks a model that always thinks: sending a disable
	// request is an error rather than a preference (GLM-5.3 and the Gemini 4
	// family both reject it).
	CannotDisable bool
	// EffortDriven marks a model whose budget saturates below the top rung, so
	// the level is the only way to reach it.
	//
	// Measured 2026-10-26 against workbuddy2api-panel, same reasoning-heavy
	// prompt, three samples per cell: budget 8192 averaged 216 reasoning
	// tokens, 32000 averaged 306, and 64000 averaged 295 - the budget does
	// move the needle, then saturates. reasoning_effort=high averaged 305 and
	// reasoning_effort=max averaged 462. No budget value reached max, so a
	// caller who states a large budget still needs the level to get there.
	//
	// The gateway itself never reads budget_tokens on the chat path (grep in
	// workbuddy2api-panel finds it only in the /v1/messages handler); the
	// field is forwarded and interpreted upstream.
	EffortDriven bool
	// NoOffLevel marks a model whose ladder has no "off" rung: asking for none
	// or minimal must become the lowest real level (GPT-6 family).
	NoOffLevel bool
	// MinBudget and MaxBudget bound thinking.budget_tokens. Zero means the
	// bound is unknown. Gemini publishes 1..32768 for the models that take a
	// budget, and forwarding a larger number is a request the model cannot
	// honour.
	MinBudget int
	MaxBudget int
}

// ClampBudget fits a token budget inside the model's documented range. A
// non-positive budget is returned unchanged: 0 and -1 mean "off" and "decide
// per request" in the protocols that use them, and are not out-of-range
// values.
func (s ThinkingSupport) ClampBudget(budget int) int {
	if budget <= 0 {
		return budget
	}
	if s.MinBudget > 0 && budget < s.MinBudget {
		budget = s.MinBudget
	}
	if s.MaxBudget > 0 && budget > s.MaxBudget {
		budget = s.MaxBudget
	}
	return budget
}

// ReasoningSupport returns what the model is known to accept. An unknown model
// yields the zero value.
//
// The entries here are the ones measured or documented; see
// llmdoc/architecture/reasoning-capability-design.md for the sources. Keeping
// the list short is deliberate - a wrong entry is worse than a missing one,
// because a missing entry forwards the client's own words while a wrong entry
// overrides them.
func ReasoningSupport(modelID string) ThinkingSupport {
	return ReasoningSupportFor(config.ModelConfig{ModelID: modelID})
}

// ReasoningSupportFor answers the same question, letting an operator's
// declaration in the model config override the built-in table.
//
// Overrides are additive per field, not whole-object replacements: an operator
// who lists a ladder has not said anything about the budget bounds, and
// dropping them because the field was omitted would turn a correction into a
// regression. The config is the more specific statement, so it wins wherever it
// speaks.
func ReasoningSupportFor(model config.ModelConfig) ThinkingSupport {
	support := builtinReasoningSupport(model.Provider, model.ModelID)
	if override := model.Reasoning; override != nil {
		if len(override.Levels) > 0 {
			support.Levels = override.Levels
		}
		if override.Budget != nil {
			support.Budget = *override.Budget
		}
		if override.CannotDisable != nil {
			support.CannotDisable = *override.CannotDisable
		}
		if override.NoOffLevel != nil {
			support.NoOffLevel = *override.NoOffLevel
		}
		if override.EffortDriven != nil {
			support.EffortDriven = *override.EffortDriven
		}
		if override.MinBudget != nil {
			support.MinBudget = *override.MinBudget
		}
		if override.MaxBudget != nil {
			support.MaxBudget = *override.MaxBudget
		}
	}
	return support
}

func builtinReasoningSupport(provider, modelID string) ThinkingSupport {
	family, realm := splitRealm(modelID)

	// A platform can decide the control regardless of the model, because what
	// the model accepts through that platform is not what it accepts directly.
	// Measured 2026-10-26 with the same reasoning-heavy prompt, interleaved
	// samples: deepseek-v4.1-flash reaches max through the WorkBuddy gateway
	// (reasoning_effort max averaged 462 tokens against high's 305) while the
	// same family through ClinePass showed no separation at all - the
	// platform's forwarding path is the variable, not the model.
	if capability, ok := platformReasoningSupport(provider); ok {
		return capability
	}

	switch {
	case strings.HasPrefix(family, "deepseek"):
		// The same model id carries a different ladder per realm: WorkBuddy's
		// international gateway advertises five rungs where the domestic one
		// advertises three. A realm-blind table collapses xhigh to high and
		// medium to low on the international side, which is what it did before
		// this split.
		if realm == "global" {
			return ThinkingSupport{Levels: []string{"low", "medium", "high", "xhigh", "max"}, Budget: true, TakesThinking: true, EffortDriven: true}
		}
		if realm == "cn" {
			return ThinkingSupport{Levels: []string{"low", "high", "max"}, Budget: true, TakesThinking: true, EffortDriven: true}
		}
		return ThinkingSupport{Levels: []string{"low", "high", "max"}, Budget: true, TakesThinking: true}
	case strings.HasPrefix(family, "glm-5.3"), strings.HasPrefix(family, "glm-5.3-flash"):
		// GLM-5.3 rejects thinking.type "disabled" outright.
		return ThinkingSupport{Levels: []string{"low", "high", "max"}, CannotDisable: true, TakesThinking: true}
	case strings.HasPrefix(family, "glm-5.2"):
		// Zhipu documents high/xhigh for 5.2, and the WorkBuddy roster copies
		// that in both realms. The previous entry listed the 5.3 ladder here.
		return ThinkingSupport{Levels: []string{"high", "xhigh"}, TakesThinking: true}
	case strings.HasPrefix(family, "glm-"):
		// Older GLM: an on/off switch, no level.
		return ThinkingSupport{TakesThinking: true}
	case strings.HasPrefix(family, "kimi-k3"):
		// K3 removed the thinking object; the level is the whole control. The
		// international gateway advertises five rungs.
		if realm == "global" {
			return ThinkingSupport{Levels: []string{"low", "medium", "high", "xhigh", "max"}, CannotDisable: true}
		}
		return ThinkingSupport{Levels: []string{"low", "high", "max"}, CannotDisable: true}
	case strings.HasPrefix(family, "kimi"):
		// K2.x and the code variants. The vendor documents a thinking object,
		// but this proxy has no live measurement of it and the existing guards
		// assert the opposite, so the field is not forwarded until someone
		// measures it. A wrong guess here is a 400 on every kimi request.
		return ThinkingSupport{}
	case strings.HasPrefix(family, "gpt-6"), strings.HasPrefix(family, "gpt-6.1"):
		return ThinkingSupport{Levels: []string{"low", "medium", "high", "xhigh", "max"}, NoOffLevel: true}
	case strings.HasPrefix(family, "gpt-5"), strings.HasPrefix(family, "o1"), strings.HasPrefix(family, "o3"):
		return ThinkingSupport{Levels: []string{"none", "minimal", "low", "medium", "high", "xhigh", "max"}}
	case strings.HasPrefix(family, "gemini-4-argon"), strings.HasPrefix(family, "gemini-3.8-flash"):
		// Unlike earlier Flash models, these reject minimal.
		// https://ai.google.dev/gemini-api/docs/generate-content/thinking#thinking-levels
		return ThinkingSupport{Levels: []string{"low", "medium", "high"}, CannotDisable: true}
	case strings.HasPrefix(family, "gemini-3.1-flash-lite-image"):
		return ThinkingSupport{Levels: []string{"minimal", "high"}, CannotDisable: true}
	case strings.HasPrefix(family, "gemini-4"), strings.HasPrefix(family, "gemini-3"):
		// Gemini 3 and later take a discrete level. Sending a budget to them is
		// the conflict Gemini documents as a 400, so Budget stays false.
		return ThinkingSupport{Levels: []string{"minimal", "low", "medium", "high"}, CannotDisable: true}
	case strings.HasPrefix(family, "gemini-2.5-flash-lite"):
		return ThinkingSupport{Budget: true, MinBudget: 512, MaxBudget: 24576}
	case strings.HasPrefix(family, "gemini-2.5-pro"):
		return ThinkingSupport{Budget: true, CannotDisable: true, MinBudget: 128, MaxBudget: 32768}
	case strings.HasPrefix(family, "gemini-2.5"):
		// Flash and the leftover 2.5 variants: 1..24576. A floor of 1 rather
		// than 0 because 0 means "do not think", which is a switch this proxy
		// expresses elsewhere; a floor of 0 would silently accept "off" as a
		// budget and send the model a number instead of the instruction.
		return ThinkingSupport{Budget: true, MinBudget: 1, MaxBudget: 24576}
	case strings.HasPrefix(family, "grok-4.6"):
		return ThinkingSupport{Levels: []string{"low", "medium", "high", "xhigh"}}
	case strings.HasPrefix(family, "grok-4.5"):
		return ThinkingSupport{Levels: []string{"low", "medium", "high"}, NoOffLevel: true}
	case strings.HasPrefix(family, "grok-4.3"):
		return ThinkingSupport{Levels: []string{"none", "low", "medium", "high"}}
	case strings.HasPrefix(family, "minimax-m3.1"):
		return ThinkingSupport{Levels: []string{"low", "medium", "high", "xhigh", "max"}, CannotDisable: true}
	case strings.HasPrefix(family, "hy4"), strings.HasPrefix(family, "hy3"):
		return ThinkingSupport{Levels: []string{"low", "high"}}
	case strings.HasPrefix(family, "step-5"):
		return ThinkingSupport{Levels: []string{"low", "medium", "high"}}
	case strings.HasPrefix(family, "mimo"):
		// Xiaomi documents only the switch; a level is accepted but its effect
		// is unverified, so it is not listed.
		return ThinkingSupport{TakesThinking: true}
	default:
		return ThinkingSupport{}
	}
}

// ClampLevel returns the level a model should be sent for a requested level.
// It reports false when the model declares no levels at all, which means the
// caller should not send a level.
//
// A level outside the model's ladder is moved to the nearest one rather than
// forwarded: every vendor that documents its ladder also documents that an
// unsupported value is an error, and the caller's intent ("think hard") is
// preserved better by the nearest rung than by a failed request.
func (s ThinkingSupport) ClampLevel(requested string) (string, bool) {
	if len(s.Levels) == 0 {
		return "", false
	}
	level := strings.ToLower(strings.TrimSpace(requested))
	if level == "" {
		return "", false
	}
	// "off" is a request to stop, not a rung - and "minimal" is not the same
	// request: it asks for the weakest real effort, which is why it falls
	// through to the nearest-rung path below.
	//
	// Three outcomes, and they are not interchangeable:
	//   - the ladder has "none": say none;
	//   - the model cannot stop thinking at all (GLM-5.3, GPT-6): the lowest
	//     real rung is the closest honest answer;
	//   - the model can stop, but not through a level (DeepSeek's low/high/max):
	//     a level cannot express this, so report "no level" and let the caller
	//     send the switch instead. Returning "low" here would turn "stop
	//     thinking" into "think a little", which is the opposite instruction.
	if level == "off" || level == "disabled" || level == "none" {
		if s.has("none") {
			return "none", true
		}
		if s.CannotDisable || s.NoOffLevel {
			return lowestPositive(s.Levels), true
		}
		return "", false
	}
	if s.has(level) {
		return level, true
	}
	// Nearest rung by rank. Rank comes from a fixed ladder so that a model
	// listing a subset still orders correctly.
	target := levelRank(level)
	best, bestDist := "", 1<<30
	for _, candidate := range s.Levels {
		dist := levelRank(candidate) - target
		if dist < 0 {
			dist = -dist
		}
		if dist < bestDist {
			best, bestDist = candidate, dist
		}
	}
	return best, best != ""
}

func (s ThinkingSupport) has(level string) bool {
	for _, candidate := range s.Levels {
		if candidate == level {
			return true
		}
	}
	return false
}

func lowestPositive(levels []string) string {
	for _, level := range levels {
		if level != "none" {
			return level
		}
	}
	return levels[0]
}

// levelRank orders the levels every vendor shares. Values a vendor invents are
// ranked above "max" so they clamp to the top rather than the bottom.
func levelRank(level string) int {
	switch level {
	case "none":
		return 0
	case "minimal":
		return 1
	case "low":
		return 2
	case "medium":
		return 3
	case "high":
		return 4
	case "xhigh":
		return 5
	case "max":
		return 6
	default:
		return 7
	}
}

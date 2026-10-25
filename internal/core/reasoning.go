package core

import "strings"

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
	// CannotDisable marks a model that always thinks: sending a disable
	// request is an error rather than a preference (GLM-5.3 and the Gemini 4
	// family both reject it).
	CannotDisable bool
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
	family := strings.ToLower(modelID)
	if idx := strings.LastIndex(family, "/"); idx >= 0 {
		family = family[idx+1:]
	}
	if idx := strings.IndexByte(family, ':'); idx >= 0 {
		family = family[idx+1:]
	}

	switch {
	case strings.HasPrefix(family, "deepseek"):
		// DeepSeek's documented surface is thinking.type plus a level; the
		// budget is this proxy's own arithmetic, so the level is what it takes.
		return ThinkingSupport{Levels: []string{"low", "high", "max"}, Budget: true}
	case strings.HasPrefix(family, "glm-5.3"), strings.HasPrefix(family, "glm-5.3-flash"):
		// GLM-5.3 rejects thinking.type "disabled" outright.
		return ThinkingSupport{Levels: []string{"low", "high", "max"}, CannotDisable: true}
	case strings.HasPrefix(family, "glm-5.2"):
		return ThinkingSupport{Levels: []string{"low", "high", "max"}}
	case strings.HasPrefix(family, "glm-"):
		// Older GLM: an on/off switch, no level.
		return ThinkingSupport{}
	case strings.HasPrefix(family, "kimi-k3"):
		// K3 removed the thinking object; the level is the whole control.
		return ThinkingSupport{Levels: []string{"low", "high", "max"}, CannotDisable: true}
	case strings.HasPrefix(family, "kimi"):
		// K2.x and the code variants take a switch, never a level.
		return ThinkingSupport{}
	case strings.HasPrefix(family, "gpt-6"), strings.HasPrefix(family, "gpt-6.1"):
		return ThinkingSupport{Levels: []string{"low", "medium", "high", "xhigh", "max"}, NoOffLevel: true}
	case strings.HasPrefix(family, "gpt-5"), strings.HasPrefix(family, "o1"), strings.HasPrefix(family, "o3"):
		return ThinkingSupport{Levels: []string{"none", "minimal", "low", "medium", "high", "xhigh", "max"}}
	case strings.HasPrefix(family, "gemini-4"), strings.HasPrefix(family, "gemini-3"):
		return ThinkingSupport{Levels: []string{"minimal", "low", "medium", "high"}, CannotDisable: true, Budget: true, MinBudget: 1, MaxBudget: 32768}
	case strings.HasPrefix(family, "gemini-2.5"):
		return ThinkingSupport{Budget: true, MinBudget: 1, MaxBudget: 32768}
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
		return ThinkingSupport{}
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

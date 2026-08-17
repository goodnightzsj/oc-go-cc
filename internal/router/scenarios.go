package router

import (
	"strings"

	"github.com/routatic/proxy/internal/config"
)

// Scenario represents the routing scenario for model selection.
type Scenario string

const (
	ScenarioDefault           Scenario = "default"
	ScenarioBackground        Scenario = "background"
	ScenarioThink             Scenario = "think"
	ScenarioComplex           Scenario = "complex"
	ScenarioLongContext       Scenario = "long_context"
	ScenarioFast              Scenario = "fast"
	ScenarioOverride          Scenario = "override"
	ScenarioVision            Scenario = "vision"
	ScenarioVisionComplex     Scenario = "vision_complex"
	ScenarioVisionLongContext Scenario = "vision_long_context"
)

// MessageContent represents a single message in a conversation.
type MessageContent struct {
	Role        string
	Content     string
	HasImage    bool
	ImageHashes []string
}

// RequestFacts summarizes relevant properties of the request for scenario
// detection — specifically whether the latest user message contains an image,
// whether the text suggests complex intent, and the raw text for pattern
// matching. This enables scenario detection to make routing decisions without
// re-parsing the full message history.
type RequestFacts struct {
	LatestUserText          string
	LatestUserHasImage      bool
	LatestTextComplexIntent bool
	NeedsVision             bool
}

// DetectScenario analyzes a request to determine which model to use.
// Routing priority:
//  1. Long Context (> threshold)
//  2. Complex (architectural patterns or tool-heavy operations)
//  3. Think (reasoning patterns)
//  4. Background (simple operations with NO tools)
//  5. Default
//
// For streaming requests, consider using RouteForStreaming() to prefer faster models.
func DetectScenario(messages []MessageContent, tokenCount int, cfg *config.Config) Scenario {
	facts := AnalyzeRequestFacts(messages)
	// 1. Check for long context first (most important)
	if tokenCount > getLongContextThreshold(cfg) {
		if facts.NeedsVision {
			return ScenarioVisionLongContext
		}
		return ScenarioLongContext
	}

	if facts.NeedsVision {
		if facts.LatestTextComplexIntent {
			return ScenarioVisionComplex
		}
		return ScenarioVision
	}

	// 2. Check for complex tasks (architectural OR tool-related)
	latestUser := latestUserMessages(messages)
	if hasComplexPattern(latestUser) {
		return ScenarioComplex
	}

	// 3. Check for thinking/reasoning patterns
	if hasThinkingPattern(latestUser) {
		return ScenarioThink
	}

	// 4. Check for background task patterns (truly simple operations)
	if hasBackgroundPattern(messages) {
		return ScenarioBackground
	}

	// 5. Default
	return ScenarioDefault
}

// AnalyzeRequestFacts extracts routing-relevant facts from the message history.
// It identifies the latest user message, checks for new images (avoiding
// false positives from historical images), and flags complex or vision-related
// intent. The result feeds into DetectScenario and is also useful for logging
// and debugging routing decisions.
func AnalyzeRequestFacts(messages []MessageContent) RequestFacts {
	facts := RequestFacts{}
	latestIdx := -1
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			latestIdx = i
			break
		}
	}
	if latestIdx == -1 {
		return facts
	}

	latest := messages[latestIdx]
	facts.LatestUserText = latest.Content
	facts.LatestUserHasImage = latest.HasImage && imageHashesAreNewForLatest(messages, latestIdx)
	facts.LatestTextComplexIntent = hasComplexPattern([]MessageContent{latest}) || hasThinkingPattern([]MessageContent{latest})

	// Trigger vision routing only when the latest user message actually
	// contains a new image. The previous heuristic also fired on historical
	// images + visual-intent keywords in the latest text, but that produces
	// false positives on ordinary prose that happens to mention "image" /
	// "screen" / "ui" / "layout" (e.g. "fix the UI layout", "check this
	// Docker image"), forcing long-running sessions off the requested model
	// onto a vision-capable one (and onto the larger-context vision
	// scenario once tokens exceed the long-context threshold) for no
	// reason. If a user genuinely wants to ask about a previously-attached
	// image, they can re-attach it; the proxy's job is to route based on
	// what the latest request actually contains.
	facts.NeedsVision = facts.LatestUserHasImage
	return facts
}

func imageHashesAreNewForLatest(messages []MessageContent, latestIdx int) bool {
	latest := messages[latestIdx]
	if len(latest.ImageHashes) == 0 {
		return latest.HasImage
	}
	seen := map[string]bool{}
	for i := 0; i < latestIdx; i++ {
		for _, hash := range messages[i].ImageHashes {
			seen[hash] = true
		}
	}
	for _, hash := range latest.ImageHashes {
		if !seen[hash] {
			return true
		}
	}
	return false
}

func latestUserMessages(messages []MessageContent) []MessageContent {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return []MessageContent{messages[i]}
		}
	}
	return nil
}

// hasComplexPattern looks for truly complex or architectural operations that need
// the most capable models.  It is intentionally narrow: common coding/debugging
// tasks ("build", "debug", "create file", "bash") are NOT complex, because they
// appear constantly in tool results and ordinary conversation and would otherwise
// route every turn to the complex model.
func hasComplexPattern(messages []MessageContent) bool {
	complexKeywords := []string{
		// Architectural / large-scale design
		"architect", "architecture", "refactor", "redesign",
		"complex", "difficult", "challenging",
		"optimize", "performance", "efficiency",
		"design pattern", "best practice",
	}

	for _, msg := range messages {
		if msg.Role == "system" || msg.Role == "user" {
			lower := strings.ToLower(msg.Content)
			for _, kw := range complexKeywords {
				if strings.Contains(lower, kw) {
					return true
				}
			}
		}
	}
	return false
}

// hasThinkingPattern looks for system prompts mentioning reasoning keywords
// or content containing thinking/reasoning markers.
func hasThinkingPattern(messages []MessageContent) bool {
	thinkingKeywords := []string{
		"think", "thinking", "plan", "reason", "reasoning",
		"analyze", "analysis", "step by step",
	}

	for _, msg := range messages {
		if msg.Role == "system" || msg.Role == "user" {
			lower := strings.ToLower(msg.Content)
			for _, kw := range thinkingKeywords {
				if strings.Contains(lower, kw) {
					return true
				}
			}
		}
		// Check for thinking content blocks
		if strings.Contains(msg.Content, "antThinking") {
			return true
		}
	}
	return false
}

// hasBackgroundPattern checks for VERY simple background tasks.
// IMPORTANT: This should be conservative - returns true only for truly trivial requests.
// If there's any mention of tools, functions, or writing, it's NOT background.
func hasBackgroundPattern(messages []MessageContent) bool {
	// If ANY tool keywords appear, it's NOT a background task
	toolBlockers := []string{
		"tool", "function", "execute", "run command",
		"write", "edit", "create", "delete", "remove",
		"implement", "build", "add", "modify",
	}

	for _, msg := range messages {
		lower := strings.ToLower(msg.Content)
		for _, kw := range toolBlockers {
			if strings.Contains(lower, kw) {
				return false
			}
		}
	}

	// Only truly simple operations are background tasks
	backgroundKeywords := []string{
		"list directory", "ls -", "dir",
		"show file", "view file", "cat file",
		"what is", "what's", "tell me about",
		"check status", "show status",
	}

	for _, msg := range messages {
		lower := strings.ToLower(msg.Content)
		for _, kw := range backgroundKeywords {
			if strings.Contains(lower, kw) {
				return true
			}
		}
	}
	return false
}

// getLongContextThreshold returns the configured threshold or a sensible default.
// Default is 100K tokens to trigger long-context models (1M context) vs regular models (128-256K).
func getLongContextThreshold(cfg *config.Config) int {
	if cfg == nil {
		return 100000 // Default: 100K tokens
	}
	if lc, ok := cfg.Models["long_context"]; ok && lc.ContextThreshold > 0 {
		return lc.ContextThreshold
	}
	return 100000 // Default: 100K tokens
}

// RouteForStreaming selects a model optimized for streaming latency.
// For streaming, we prioritize fast TTFT (time-to-first-token) over capability.
// This may return a less capable model but one that streams faster.
func RouteForStreaming(messages []MessageContent, tokenCount int, cfg *config.Config) Scenario {
	facts := AnalyzeRequestFacts(messages)
	// For streaming, use simpler models that have better TTFT
	// Complex models (GLM, Kimi) are too slow for streaming with many tools

	if tokenCount > getLongContextThreshold(cfg) {
		if facts.NeedsVision {
			return ScenarioVisionLongContext
		}
		return ScenarioLongContext
	}

	if facts.NeedsVision {
		if facts.LatestTextComplexIntent {
			return ScenarioVisionComplex
		}
		return ScenarioVision
	}

	// Everything else streams on the fast scenario, complex prompts included:
	// GLM-5 and Kimi are too slow for streaming.
	return ScenarioFast
}

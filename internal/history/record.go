// Package history maintains an in-memory ring buffer of recent proxy requests.
package history

import (
	"strings"
	"time"
)

// RequestRecord holds metadata for a single completed proxy request.
type RequestRecord struct {
	ID                  string        // unique request ID
	Model               string        // actual upstream model used (e.g. "kimi-k2.6")
	Provider            string        // provider name (e.g. "opencode-go")
	Scenario            string        // routing scenario (e.g. "default", "complex")
	StartTime           time.Time     // when the request started
	Duration            time.Duration // total latency
	InputTokens         int           // raw (non-cache) input tokens from SSE usage event
	OutputTokens        int           // output tokens from SSE usage event
	CacheReadTokens     int           // prompt-cache read tokens
	CacheCreationTokens int           // prompt-cache write tokens
	CostUSD             float64       // provider cost in USD, estimated when CostSource is "estimated"
	CostKnown           bool          // whether CostUSD is available
	CostSource          string        // "estimated" or "provider"
	DetailsKnown        bool          // whether routing/status/latency fields were observed locally
	Streaming           bool          // whether this was a streaming request

	Success  bool   // whether it completed successfully
	ErrorMsg string // error message if failed
	Attempt  int    // attempt number in fallback chain (1 = primary, >1 = fallback)

	// PeakMultiplier is the billing multiplier applied by the upstream
	// (1 = off-peak base rate, 2 = deepseek weekday peak). 0 means unpriced.
	PeakMultiplier float64 `json:"peak_multiplier"`
}

// PeakMultiplier returns the billing multiplier OpenCode Go applies for a
// model at time t. DeepSeek models (V4 Flash / Pro / Flash Vision Exp) are
// billed at 2x during weekday peak windows: Mon-Fri 01:00-04:00 and
// 06:00-10:00 UTC (opencode.ai/docs/zh-cn/go). Weekends and other models are
// always off-peak (multiplier 1). A zero time degrades to 1.
func PeakMultiplier(model string, t time.Time) float64 {
	if t.IsZero() || !strings.Contains(strings.ToLower(model), "deepseek") {
		return 1
	}
	wd := t.Weekday()
	if wd == time.Saturday || wd == time.Sunday {
		return 1
	}
	h := t.UTC().Hour()
	if (h >= 1 && h < 4) || (h >= 6 && h < 10) {
		return 2
	}
	return 1
}

// DisplayInputTokens is the total input a user consumed (raw + cache), used
// for UI display. Billing uses the split fields separately.
func (r RequestRecord) DisplayInputTokens() int {
	return r.InputTokens + r.CacheReadTokens + r.CacheCreationTokens
}

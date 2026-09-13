// Package history maintains an in-memory ring buffer of recent proxy requests.
package history

import (
	"strings"
	"time"

	"github.com/routatic/proxy/internal/models"
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
	// (1 = off-peak base rate, 2 = deepseek weekday peak). 0 means unspecified.
	PeakMultiplier float64 `json:"peak_multiplier"`
}

// PeakMultiplier returns the billing multiplier OpenCode Go applies for a
// model at time t. DeepSeek models (V4 Flash / Pro / Flash Vision Exp) are
// billed at 2x during weekday peak windows: Mon-Fri 01:00-04:00 and
// 06:00-10:00 UTC (opencode.ai/docs/zh-cn/go). Weekends and other models are
// always off-peak (multiplier 1). A zero time degrades to 1.
func PeakMultiplier(model string, t time.Time) float64 {
	return ProviderPeakMultiplier("opencode-go", model, t)
}

// commandCodePeakModels lists the CommandCode models that carry a peak rate,
// keyed by family so a vendor-prefixed id matches like a flat one. CommandCode
// prints the peak sub-line on each model's row rather than covering a family as
// a whole, and variants such as "deepseek-v4-flash-fast" do not carry it, so the
// set is listed instead of matched by substring.
// Source: commandcode.ai/docs/plans/goat (peak 01-04 & 06-10 UTC, Mon-Fri).
var commandCodePeakModels = map[string]bool{
	"deepseek-v4.1-flash":          true,
	"deepseek-v4-flash":            true,
	"deepseek-v4-flash-vision-exp": true,
	"deepseek-v4-pro":              true,
}

// ProviderPeakMultiplier answers, for one provider's request, whether the
// platform billed it at its peak rate. OpenCode Go and CommandCode publish the
// same window (Mon-Fri 01:00-04:00 and 06:00-10:00 UTC) and the same 2x rate, so
// the schedule is stated once and each platform only answers the model question.
// A platform with no peak pricing - and an empty provider, which retains the
// interpretation of legacy OpenCode Go records - is always off-peak.
func ProviderPeakMultiplier(provider, model string, t time.Time) float64 {
	if t.IsZero() {
		return 1
	}
	family := models.ModelFamily(model)
	switch provider {
	case "", "opencode-go":
		if !strings.Contains(family, "deepseek") {
			return 1
		}
	case "commandcode":
		if !commandCodePeakModels[family] {
			return 1
		}
	default:
		return 1
	}
	utc := t.UTC()
	if wd := utc.Weekday(); wd == time.Saturday || wd == time.Sunday {
		return 1
	}
	if h := utc.Hour(); (h >= 1 && h < 4) || (h >= 6 && h < 10) {
		return 2
	}
	return 1
}

// DisplayInputTokens is the total input a user consumed (raw + cache), used
// for UI display. Billing uses the split fields separately.
func (r RequestRecord) DisplayInputTokens() int {
	return r.InputTokens + r.CacheReadTokens + r.CacheCreationTokens
}

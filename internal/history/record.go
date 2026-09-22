// Package history maintains an in-memory ring buffer of recent proxy requests.
package history

import (
	"strings"
	"time"

	"github.com/routatic/proxy/internal/models"
	"github.com/routatic/proxy/internal/site"
)

// RequestRecord holds metadata for a single completed proxy request.
type RequestRecord struct {
	ID                  string        // unique request ID
	Model               string        // actual upstream model used (e.g. "kimi-k2.6")
	RequestedModel      string        // model the client asked for, when it differs from Model
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

// RequestedModelDiffers reports whether the client asked for a model other than
// the one that served the request, which is what makes the pair worth storing.
//
// A client that sends a Claude model id (claude-sonnet-4-5) to a proxy that
// routes on scenarios always differs, but a request routed to the exact model it
// named does not. Storing equality would fill the column with the same string
// twice and make "was this rerouted?" unanswerable without comparing anyway.
//
// Comparison is case-insensitive and whitespace-trimmed because model ids are
// matched that way elsewhere; a difference in case is not a reroute.
func RequestedModelDiffers(requested, served string) bool {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return false
	}
	return !strings.EqualFold(requested, strings.TrimSpace(served))
}

// peakModelFamilies is the set of model families the peak-pricing platforms
// bill at their peak rate, keyed by ModelFamily so a vendor-prefixed id matches
// like a flat one. Each platform names its covered models in its own pricing
// table rather than covering a DeepSeek family as a whole, so the set is listed
// instead of matched by substring - "deepseek-v4-flash-fast" and the older
// deepseek-v3/r1/chat families carry a single rate and must stay off-peak.
//
// A bare family is not enough either: upstream publishes dated snapshots
// ("deepseek-v4-flash-0423", "deepseek-v4-pro-0813") of these same models, and
// those bill at the snapshot's rate, which the pricing tables list separately.
var peakModelFamilies = map[string]bool{
	"deepseek-v4.1-flash":          true,
	"deepseek-v4-flash":            true,
	"deepseek-v4-flash-vision-exp": true,
	"deepseek-v4-pro":              true,
}

// PeakWindow is one billing window as a half-open UTC hour range [Start, End)
// on the days the schedule names. Exported so callers that mirror this rule -
// the dashboard's badge - can compare against it instead of restating hours.
type PeakWindow struct{ Start, End int }

// peakSchedule is one platform's published peak-pricing rule.
type peakSchedule struct {
	models     map[string]bool
	windows    []PeakWindow
	multiplier float64
}

// peakSchedules is each platform's published peak-pricing rule: which models it
// covers, when it applies, and the multiplier those hours bill at. Off-peak is
// every other hour, including weekends; a platform with no entry here has no
// peak pricing at all and is always off-peak.
//
// The schedule is stated per platform rather than shared, even where two
// platforms agree today: this feeds cost estimation, so if one platform moves a
// window or adds a model, this table must make that a one-line change to that
// platform instead of silently moving the other one's money too.
//
// Sources, all re-verified 2026-09-22 against the live pages:
//   - opencode.ai/docs/zh-cn/go - "DeepSeek V4.1 Flash / V4 Pro / V4 Flash /
//     V4 Flash Vision Exp: Peak 时段为周一至周五的 01:00-04:00 和 06:00-10:00
//     UTC；其他所有时段（包括周末）均为 Off-Peak."
//   - commandcode.ai/models - those same four rows carry the peak sub-line
//     "Off-peak shown (17h/day) · peak $X / $Y 01–04 & 06–10 UTC, Mon–Fri".
//   - api-docs.deepseek.com/quick_start/pricing - "Off-peak rates are half of
//     the peak rates. Peak hours are 01:00 - 04:00 and 06:00 - 10:00 UTC,
//     Monday through Friday, excluding Chinese public holidays." ClinePass's
//     page footnotes DeepSeek's pricing page for its two DeepSeek rows, so this
//     is the window that governs them. All three platforms therefore share one
//     window today, and each still states it separately for the reason above.
//
// Chinese public holidays are excluded by DeepSeek's rule and are NOT modelled
// here: the holiday calendar is not published as data, and a lookup table that
// goes stale would move money on the days it is wrong. Peak is therefore
// over-applied on those handful of weekdays. That is a known, bounded
// inaccuracy in the conservative direction - it can over-price, never
// under-price - and it is recorded rather than hidden. ponytail: add a holiday
// table only if the error is ever observed to matter.
var peakSchedules = map[string]peakSchedule{
	"opencode-go": {
		models:     peakModelFamilies,
		windows:    []PeakWindow{{1, 4}, {6, 10}},
		multiplier: 2,
	},
	"commandcode": {
		models:     peakModelFamilies,
		windows:    []PeakWindow{{1, 4}, {6, 10}},
		multiplier: 2,
	},
	// ClinePass's pricing table carries a Peak column for its two DeepSeek rows,
	// so it belongs here for the same reason the other two do. The window is the
	// same one: its table footnotes DeepSeek's pricing page, and that page states
	// "Peak hours are 01:00 - 04:00 and 06:00 - 10:00 UTC, Monday through Friday,
	// excluding Chinese public holidays" - the same hours CommandCode and OpenCode
	// Go publish. An earlier comment here claimed the window was unknown and
	// unverified; it was verifiable from the source ClinePass itself cites.
	//
	// The covered set needs both DeepSeek Flash spellings, and that is not
	// belt-and-braces. ClinePass's table names the row "DeepSeek V4 Flash", while
	// its live roster serves cline-pass/deepseek-v4.1-flash and has no plain
	// v4-flash at all. DeepSeek's own page resolves the two names - "the legacy
	// names deepseek-v4-flash ... are still accepted, but the corresponding models
	// have been retired, their requests are served by the DeepSeek-V4.1-Flash
	// model and billed at the Flash price" - so the table's single Flash row
	// describes the model this instance actually serves, under its other name.
	// Both are listed because either spelling may arrive on a record.
	"cline-pass": {
		models: map[string]bool{
			"deepseek-v4-flash":   true,
			"deepseek-v4.1-flash": true,
			"deepseek-v4-pro":     true,
		},
		windows:    []PeakWindow{{1, 4}, {6, 10}},
		multiplier: 2,
	},
}

// PeakMultiplier returns the billing multiplier OpenCode Go applies for a
// model at time t. A zero time degrades to 1.
func PeakMultiplier(model string, t time.Time) float64 {
	return ProviderPeakMultiplier("opencode-go", model, t)
}

// PeakSchedule returns one platform's published peak windows and multiplier,
// and the multiplier is 1 (with no windows) for a platform that publishes
// none. Callers outside this package use it to check their own copy of the
// rule against this one rather than restating the hours.
func PeakSchedule(provider string) (windows []PeakWindow, multiplier float64) {
	s, ok := peakSchedules[site.Normalize(provider)]
	if !ok {
		return nil, 1
	}
	return s.windows, s.multiplier
}

// PeakScheduledProviders names the platforms that publish peak pricing.
func PeakScheduledProviders() map[string]bool {
	out := make(map[string]bool, len(peakSchedules))
	for id := range peakSchedules {
		out[id] = true
	}
	return out
}

// PeakModelFamilies names the model families the peak-pricing platforms cover.
func PeakModelFamilies() map[string]bool {
	out := make(map[string]bool, len(peakModelFamilies))
	for family := range peakModelFamilies {
		out[family] = true
	}
	return out
}

// ProviderPeakMultiplier answers, for one provider's request, whether the
// platform billed it at its peak rate. The provider is normalised first, so
// legacy spellings resolve and an empty one means the default platform - which
// is how records stored before the provider column existed are read.
func ProviderPeakMultiplier(provider, model string, t time.Time) float64 {
	if t.IsZero() {
		return 1
	}
	schedule, ok := peakSchedules[site.Normalize(provider)]
	if !ok || !schedule.models[models.ModelFamily(model)] {
		return 1
	}
	utc := t.UTC()
	if wd := utc.Weekday(); wd == time.Saturday || wd == time.Sunday {
		return 1
	}
	hour := utc.Hour()
	for _, w := range schedule.windows {
		if hour >= w.Start && hour < w.End {
			return schedule.multiplier
		}
	}
	return 1
}

// DisplayInputTokens is the total input a user consumed (raw + cache), used
// for UI display. Billing uses the split fields separately.
func (r RequestRecord) DisplayInputTokens() int {
	return r.InputTokens + r.CacheReadTokens + r.CacheCreationTokens
}

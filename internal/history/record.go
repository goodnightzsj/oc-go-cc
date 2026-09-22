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
	// (1 = off-peak base rate; 2 = the DeepSeek weekday peak the three
	// DeepSeek-priced platforms share; 1.6 = OpenRouter's tencent/hy3 window).
	// 0 means unspecified. Read it as a figure, not as a flag: the multiplier is
	// per platform, and 1.6 is what makes "peak" a set of values rather than a
	// boolean.
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

// deepseekPeakFamilies is the set of DeepSeek families that carry a second rate
// on the platforms publishing DeepSeek's own peak rule, keyed by ModelFamily so
// a vendor-prefixed id matches like a flat one. Each platform names its covered
// models in its own pricing table rather than covering a DeepSeek family as a
// whole, so the set is listed instead of matched by substring -
// "deepseek-v4-flash-fast" and the older deepseek-v3/r1/chat families carry a
// single rate and must stay off-peak.
//
// A bare family is not enough either: upstream publishes dated snapshots
// ("deepseek-v4-flash-0423", "deepseek-v4-pro-0813") of these same models, and
// those bill at the snapshot's rate, which the pricing tables list separately.
//
// This is a function, not a shared map value, because each platform's entry
// must own its own set: one map handed to two platforms means adding a model
// for one silently prices it at peak on the other, which is exactly the
// coupling the per-platform table exists to prevent.
// TestEachScheduleOwnsItsCoveredModels holds the result.
func deepseekPeakFamilies() map[string]bool {
	return map[string]bool{
		"deepseek-v4.1-flash":          true,
		"deepseek-v4-flash":            true,
		"deepseek-v4-flash-vision-exp": true,
		"deepseek-v4-pro":              true,
	}
}

// PeakWindow is one billing window as a half-open UTC hour range [Start, End)
// on the days the rule names.
type PeakWindow struct{ Start, End int }

// peakRule is one model set's peak-pricing rule: which models it covers, when
// the higher rate applies, and how much higher it is.
//
// It is per rule rather than per platform because OpenRouter's windows differ
// between the models on it - DeepSeek's own models peak on weekday mornings
// while tencent/hy3 peaks across most of the day, every day - and a single
// platform-wide window cannot state both. The other platforms publish one rule
// each, which is the same shape with a single entry.
type peakRule struct {
	models     map[string]bool
	windows    []PeakWindow
	multiplier float64
	// allDays lifts the weekday restriction for a rule whose window runs every
	// day of the week. The zero value keeps the restriction, so a rule that
	// forgets it behaves like every other one rather than silently gaining
	// weekend peak hours - the failure that would overstate a bill.
	allDays bool
}

// inForceAt reports the multiplier this rule applies at t, or 1 when the rule is
// not in force. Order is day, then holiday, then window: the day and holiday
// conditions exempt the whole day, so neither can be recovered by the hour.
func (r peakRule) inForceAt(t time.Time) float64 {
	utc := t.UTC()
	if !r.allDays {
		if wd := utc.Weekday(); wd == time.Saturday || wd == time.Sunday {
			return 1
		}
	}
	hour := utc.Hour()
	for _, w := range r.windows {
		if hour >= w.Start && hour < w.End {
			return r.multiplier
		}
	}
	return 1
}

// peakSchedule is one platform's published peak-pricing rule.
//
// excludesHolidays is per platform because the platforms do not agree on it,
// and the difference is in what each one states about itself:
//
//   - DeepSeek publishes the rule and states the exemption: "Peak hours are
//     01:00 - 04:00 and 06:00 - 10:00 UTC, Monday through Friday, excluding
//     Chinese public holidays. All other hours are off-peak, including weekends
//     and Chinese public holidays in full."
//   - ClinePass states no window of its own; its table footnotes that page, so
//     the rule it inherits is the one that carries the exemption.
//   - OpenCode Go states the window without the exemption ("...Monday through
//     Friday; all other hours, including weekends, are Off-Peak") but links
//     that very sentence to DeepSeek's pricing page. The link is read here as
//     inheriting the rule it points at, which is conservative: a holiday is
//     priced off-peak. If Go is ever observed billing a holiday at peak, this
//     is the one bool to flip.
//   - CommandCode states the full rule itself and never references DeepSeek:
//     "Peak runs Monday to Friday only, so it is never charged at the weekend."
//     No exemption is stated, so none is applied, and its holidays bill at peak.
//   - OpenRouter publishes its windows as data, with the weekdays named and no
//     holiday carve-out anywhere in the payload, so none is applied - the same
//     reading CommandCode gets, for the same reason.
type peakSchedule struct {
	rules            []peakRule
	excludesHolidays bool
}

// schedule returns the multiplier this platform applies to one model family at
// t. A family no rule covers is off-peak; so is one whose rule is not in force
// at that instant. Rules within a platform must cover disjoint families, so the
// first match is the only match.
func (s peakSchedule) multiplierAt(family string, t time.Time) float64 {
	utc := t.UTC()
	// Holidays exempt the whole day, not just the peak windows: the rule puts
	// them entirely off-peak. Checked once for the platform because every rule on
	// a platform shares the one published exemption statement.
	if s.excludesHolidays && IsChineseHoliday(utc) {
		return 1
	}
	for _, r := range s.rules {
		if r.models[family] {
			return r.inForceAt(utc)
		}
	}
	return 1
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
//   - opencode.ai/docs/go (en) and /docs/zh-cn/go, which agree verbatim - "Peak
//     hours are 01:00-04:00 and 06:00-10:00 UTC, Monday through Friday; all
//     other hours, including weekends, are Off-Peak." The same sentence's
//     "Learn more" link points at DeepSeek's pricing page; see
//     peakSchedule.excludesHolidays for why that is read as inheriting the
//     exemption. (The zh-tw page omits the sentence entirely while still
//     publishing both price columns, so a missing sentence on one locale is not
//     evidence a rule is absent.)
//   - commandcode.ai/models - those same four rows carry the peak sub-line
//     "Off-peak shown (17h/day) · peak $X / $Y 01–04 & 06–10 UTC, Mon–Fri", and
//     the model page states it out in full. No page on the site links to
//     DeepSeek, so nothing is inherited.
//   - api-docs.deepseek.com/quick_start/pricing - "Off-peak rates are half of
//     the peak rates. Peak hours are 01:00 - 04:00 and 06:00 - 10:00 UTC,
//     Monday through Friday, excluding Chinese public holidays." The zh-cn page
//     states the same window in Beijing time (09:00-12:00 and 14:00-18:00),
//     which is the conversion this table relies on. ClinePass's page footnotes
//     this page for its two DeepSeek rows, so this is the window that governs
//     them. All three platforms therefore share one window today, and each
//     still states it separately for the reason above.
//   - openrouter.ai/api/v1/models (public, no key) - the `pricing.overrides`
//     arrays. See the OpenRouter entry below for how each rule is read out of
//     them.
//
// Chinese public holidays are exempt for the platforms whose rule says so; see
// peakSchedule.excludesHolidays for which, and why. The calendar comes from
// holidays.go, which is seeded from the embedded copy at startup and refreshed
// daily - a holiday is never guessed at from a table that could have gone
// stale, and a failed refresh keeps the previous calendar rather than
// collapsing to "no holidays", which would silently restore the over-billing.
var peakSchedules = map[string]peakSchedule{
	"opencode-go": {
		excludesHolidays: true,
		rules: []peakRule{{
			models:     deepseekPeakFamilies(),
			windows:    []PeakWindow{{1, 4}, {6, 10}},
			multiplier: 2,
		}},
	},
	"commandcode": {
		// No exemption: CommandCode states its own rule and does not reference
		// DeepSeek's, so its holidays bill at peak.
		rules: []peakRule{{
			models:     deepseekPeakFamilies(),
			windows:    []PeakWindow{{1, 4}, {6, 10}},
			multiplier: 2,
		}},
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
		excludesHolidays: true,
		rules: []peakRule{{
			models: map[string]bool{
				"deepseek-v4-flash":   true,
				"deepseek-v4.1-flash": true,
				"deepseek-v4-pro":     true,
			},
			windows:    []PeakWindow{{1, 4}, {6, 10}},
			multiplier: 2,
		}},
	},
	// OpenRouter is the one platform here whose window is not its own statement
	// but the `pricing.overrides` array it publishes per model, so its rules are
	// read out of that payload rather than off a pricing page. Two shapes appear,
	// and they need separate rules because their multipliers and day sets differ:
	//
	//   - deepseek-v4.1-flash and deepseek-v4-pro-0813 carry a Monday-Friday
	//     window of 01:00-04:00 and 06:00-10:00 UTC at exactly twice the rate the
	//     rest of the week bills at, plus an explicit Saturday/Sunday entry at
	//     the base rate. That is DeepSeek's own rule, restated, so it is modelled
	//     the same way - and the named weekend entry is what `allDays: false`
	//     already does, rather than something extra to encode.
	//   - tencent/hy3 carries two windows and no day condition: 00:00-16:00 UTC at
	//     1.6x, and 16:00-24:00 at the base rate. Both facts are outside what the
	//     other platforms need - a multiplier that is not 2, and a window that
	//     runs every day of the week - which is why peakRule carries its own
	//     multiplier and day handling instead of the platform doing so.
	//
	// The multiplier is stated relative to the catalog's stored base rate, not
	// relative to the payload's headline `prompt` value, and for hy3 those are
	// two different bands: OpenRouter leads with 0.132/0.528 per million and
	// discounts to 0.0825/0.33, while models.dev records 0.0825/0.33 as the base
	// - so 1.6x is what reconciles the two. This holds only while models.dev keeps
	// the cheaper band as the base (it does for both models as of 2026-09-22,
	// and 0.15/0.6 equals the base band for the DeepSeek pair). If that source
	// ever adopts the headline band instead, these multipliers double-count and
	// must be re-derived - which TestOpenRouterPeakMatchesThePublishedOverrides
	// records the expected figures for.
	//
	// No holiday exemption: the payload names weekdays and weekends and never
	// mentions holidays, so none is applied - the same reading CommandCode gets,
	// for the same reason. A model with a time window this table does not cover
	// falls to its base rate, which understates the bill rather than inventing a
	// rule; docs/openrouter.md records how the set is kept current.
	"openrouter": {
		rules: []peakRule{
			{
				models: map[string]bool{
					"deepseek-v4.1-flash":   true,
					"deepseek-v4-pro-0813":  true,
				},
				windows:    []PeakWindow{{1, 4}, {6, 10}},
				multiplier: 2,
			},
			{
				models:     map[string]bool{"hy3": true},
				windows:    []PeakWindow{{0, 16}},
				multiplier: 1.6,
				allDays:    true,
			},
		},
	},
}

// PeakMultiplier returns the billing multiplier OpenCode Go applies for a
// model at time t. A zero time degrades to 1.
func PeakMultiplier(model string, t time.Time) float64 {
	return ProviderPeakMultiplier("opencode-go", model, t)
}

// PeakWindowsFor returns the hours one platform's rules charge a higher rate
// on, and the multipliers they charge, for reporting.
//
// Tests only. It existed to let the dashboard check its own copy of the rule
// against this one; that copy was deleted when the rule gained a holiday
// calendar the browser cannot reproduce, so nothing in the running program
// calls this now. Kept because it is how the parity tests assert the dashboard
// has not grown a second copy back.
//
// A platform's rules may differ in multiplier and day coverage, so the windows
// are returned per rule rather than flattened into one list.
func PeakWindowsFor(provider string) []PeakRuleView {
	s, ok := peakSchedules[site.Normalize(provider)]
	if !ok {
		return nil
	}
	out := make([]PeakRuleView, 0, len(s.rules))
	for _, r := range s.rules {
		out = append(out, PeakRuleView{
			Models:     r.models,
			Windows:    r.windows,
			Multiplier: r.multiplier,
			AllDays:    r.allDays,
		})
	}
	return out
}

// PeakRuleView is one peak rule as reported by PeakWindowsFor. Tests only.
type PeakRuleView struct {
	Models     map[string]bool
	Windows    []PeakWindow
	Multiplier float64
	AllDays    bool
}

// PeakScheduledProviders names the platforms that publish peak pricing.
// Tests only, for the reason on PeakWindowsFor.
func PeakScheduledProviders() map[string]bool {
	out := make(map[string]bool, len(peakSchedules))
	for id := range peakSchedules {
		out[id] = true
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
	if !ok {
		return 1
	}
	return schedule.multiplierAt(models.ModelFamily(model), t)
}

// DisplayInputTokens is the total input a user consumed (raw + cache), used
// for UI display. Billing uses the split fields separately.
func (r RequestRecord) DisplayInputTokens() int {
	return r.InputTokens + r.CacheReadTokens + r.CacheCreationTokens
}

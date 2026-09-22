package history

import (
	"slices"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/site"
)

// inside is a Monday 02:00 UTC instant: weekday, inside the 01-04 window.
var inside = time.Date(2026, 9, 7, 2, 0, 0, 0, time.UTC)

// Both platforms publish the same four peak-priced models and name them row by
// row, so a family-wide substring match is wrong in both directions: it would
// peak-price the older DeepSeek families and the "fast" variant, none of which
// carry a second rate. Re-verified against the live pages 2026-09-14.
func TestPeakCoversOnlyTheDocumentedFamilies(t *testing.T) {
	peakPriced := []string{
		"deepseek-v4.1-flash",
		"deepseek-v4-flash",
		"deepseek-v4-flash-vision-exp",
		"deepseek-v4-pro",
		// The same models as CommandCode namespaces them.
		"deepseek/deepseek-v4-flash",
		"deepseek/deepseek-v4.1-flash",
	}
	// Single-rate models: the older families, the "fast" variant the pricing
	// page lists without a peak sub-line, and dated snapshots.
	singleRate := []string{
		"deepseek-v3", "deepseek-v3.2", "deepseek-r1", "deepseek-chat",
		"deepseek-reasoner",
		"deepseek/deepseek-v4-flash-fast",
		"deepseek-v4-flash-0423", "deepseek-v4-pro-0813",
		"kimi-k2.6", "glm-5.2", "moonshotai/Kimi-K2.6",
	}
	for _, provider := range []string{"opencode-go", "commandcode"} {
		for _, model := range peakPriced {
			if got := ProviderPeakMultiplier(provider, model, inside); got != 2 {
				t.Errorf("%s/%s = %v, want 2", provider, model, got)
			}
		}
		for _, model := range singleRate {
			if got := ProviderPeakMultiplier(provider, model, inside); got != 1 {
				t.Errorf("%s/%s = %v, want 1 (not peak-priced)", provider, model, got)
			}
		}
	}
}

// ClinePass bills its DeepSeek rows at peak too, and its pricing table names
// one of them differently from the id it serves.
//
// This was a live defect: cline-pass had no entry in peakSchedules at all, so
// every ClinePass request billed flat at the off-peak rate. The instance
// happened to serve cline-pass/deepseek-v4.1-flash, which is exactly the model
// whose peak rate was being dropped, and no badge appeared.
//
// The alias is the subtle part. ClinePass's table lists a row named "DeepSeek
// V4 Flash" and lists no v4.1 row, while its live roster serves
// cline-pass/deepseek-v4.1-flash and has no plain v4-flash. DeepSeek's pricing
// page - the source ClinePass footnotes - resolves the two: the legacy
// deepseek-v4-flash name is still accepted but is served by DeepSeek-V4.1-Flash
// and billed at the Flash price. So both spellings must be covered, or the
// model actually being served would still be missed.
func TestClinePassPeakCoversBothFlashSpellings(t *testing.T) {
	for _, model := range []string{
		"cline-pass/deepseek-v4.1-flash", // what the roster serves
		"cline-pass/deepseek-v4-flash",   // what the pricing table calls it
		"cline-pass/deepseek-v4-pro",
	} {
		if got := ProviderPeakMultiplier("cline-pass", model, inside); got != 2 {
			t.Errorf("cline-pass/%s = %v, want 2: ClinePass publishes a peak rate for it", model, got)
		}
	}
	// Its other models carry a single reference rate, so they must stay off-peak.
	for _, model := range []string{
		"cline-pass/glm-5.3", "cline-pass/kimi-k3", "cline-pass/qwen3.7-max",
		"cline-pass/minimax-m3", "cline-pass/mimo-v2.5",
	} {
		if got := ProviderPeakMultiplier("cline-pass", model, inside); got != 1 {
			t.Errorf("cline-pass/%s = %v, want 1 (single published rate)", model, got)
		}
	}
}

// The window is weekday-only and half-open at both ends, so 04:00 and 06:00 are
// off-peak and 10:00 ends it. Read in UTC - the instant, not the wall clock of
// whoever is looking at the dashboard.
func TestPeakWindowBoundariesAreHalfOpenAndUTC(t *testing.T) {
	mk := func(day time.Weekday, hour int) time.Time {
		// 2026-09-07 is a Monday.
		base := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
		return base.AddDate(0, 0, int(day)-int(time.Monday)).Add(time.Duration(hour) * time.Hour)
	}
	west := time.FixedZone("UTC-05", -5*60*60)
	for _, c := range []struct {
		name string
		t    time.Time
		want float64
	}{
		{"mon 01:00 opens", mk(time.Monday, 1), 2},
		{"mon 03:59 inside", mk(time.Monday, 3), 2},
		{"mon 04:00 closes", mk(time.Monday, 4), 1},
		{"mon 06:00 opens", mk(time.Monday, 6), 2},
		{"mon 09:59 inside", mk(time.Monday, 9), 2},
		{"mon 10:00 closes", mk(time.Monday, 10), 1},
		{"mon 00:59 before", mk(time.Monday, 0), 1},
		{"fri 02:00 weekday", mk(time.Friday, 2), 2},
		{"sat 02:00 weekend", mk(time.Saturday, 2), 1},
		{"sun 08:00 weekend", mk(time.Sunday, 8), 1},
	} {
		for _, stamp := range []time.Time{c.t, c.t.In(west)} {
			if got := ProviderPeakMultiplier("opencode-go", "deepseek-v4-flash", stamp); got != c.want {
				t.Errorf("%s at %v = %v, want %v", c.name, stamp, got, c.want)
			}
		}
	}
	if got := ProviderPeakMultiplier("opencode-go", "deepseek-v4-flash", time.Time{}); got != 1 {
		t.Errorf("zero time = %v, want 1", got)
	}
}

// A platform with no published peak pricing is never peak-priced, and an empty
// provider keeps the pre-provider-column reading (the default platform).
//
// OpenRouter is deliberately not in the list below: it does publish peak
// pricing, for other models, and TestOpenRouterPeakMatchesThePublishedOverrides
// covers it. The deepseek-v4-flash checked here is one of the models it does not
// cover, so "1" is the right answer for a different reason - not that the
// platform lacks peak pricing.
func TestPeakIsScopedToPlatformsThatPublishIt(t *testing.T) {
	for _, provider := range []string{"opencode-zen", "aws-bedrock"} {
		if got := ProviderPeakMultiplier(provider, "deepseek-v4-flash", inside); got != 1 {
			t.Errorf("%s = %v, want 1 (platform publishes no peak pricing)", provider, got)
		}
	}
	// A platform that does publish peak pricing still prices a model it does not
	// cover at the base rate; the rule is per model, not per platform.
	if got := ProviderPeakMultiplier("openrouter", "deepseek/deepseek-v4-flash", inside); got != 1 {
		t.Errorf("openrouter/deepseek-v4-flash = %v, want 1 (no override on this model)", got)
	}
	if got := ProviderPeakMultiplier("", "deepseek-v4-flash", inside); got != 2 {
		t.Errorf("empty provider = %v, want 2 (legacy records mean the default platform)", got)
	}
	// Legacy underscore spelling normalises to the hyphenated id.
	if got := ProviderPeakMultiplier("opencode_go", "deepseek-v4-flash", inside); got != 2 {
		t.Errorf("opencode_go = %v, want 2", got)
	}
}

// Every schedule must be internally consistent: windows inside a day, and a
// multiplier that actually means "peak". A malformed entry would otherwise
// price whole windows at 1x while looking configured.
func TestEveryPeakScheduleIsWellFormed(t *testing.T) {
	for provider, s := range peakSchedules {
		if len(s.rules) == 0 {
			t.Errorf("%s: no rules", provider)
		}
		for i, r := range s.rules {
			if len(r.models) == 0 {
				t.Errorf("%s: rule %d lists no models", provider, i)
			}
			if r.multiplier <= 1 {
				t.Errorf("%s: rule %d has multiplier %v, which does not mark peak", provider, i, r.multiplier)
			}
			if len(r.windows) == 0 {
				t.Errorf("%s: rule %d has no windows", provider, i)
			}
			for _, w := range r.windows {
				if w.Start < 0 || w.End > 24 || w.Start >= w.End {
					t.Errorf("%s: rule %d window [%d,%d) is not a valid hour range", provider, i, w.Start, w.End)
				}
			}
		}
	}
}

// TestPeakRulesWithinAPlatformAreDisjoint. A family must be covered by one rule
// and no more: the lookup takes the first match, so a second rule covering the
// same family would be dead - and dead in the direction that hides a wrong
// price, since the rule that silently never applies is the one someone just
// added for a new window.
func TestPeakRulesWithinAPlatformAreDisjoint(t *testing.T) {
	for provider, s := range peakSchedules {
		seen := make(map[string]int)
		for i, r := range s.rules {
			for family := range r.models {
				if j, dup := seen[family]; dup {
					t.Errorf("%s: %q is covered by rules %d and %d; only the first can ever apply", provider, family, j, i)
				}
				seen[family] = i
			}
		}
	}
}

// TestPeakSchedulesAgreeWithTheRegistry closes the one omission that has already
// cost real money here. A platform that publishes two rates for one model needs
// an entry in peakSchedules; without it every one of its requests bills flat at
// the off-peak rate, the charge is simply wrong, and nothing reports it - the
// row renders perfectly well with no badge.
//
// ClinePass was exactly that: its pricing table carries a Peak column, it had no
// entry, and its traffic was under-billed until someone noticed no badge ever
// appeared. The absence of a schedule is indistinguishable from a platform that
// has no peak pricing, so the fact is now stated in the registry too and the two
// are checked against each other. Either direction being wrong is a failure:
// a PeakPriced platform with no rule over-bills nothing but under-charges its
// peak hours, and a schedule for a platform that does not declare peak pricing
// means the registry is out of date.
func TestPeakSchedulesAgreeWithTheRegistry(t *testing.T) {
	scheduled := PeakScheduledProviders()
	for _, d := range site.All() {
		_, hasSchedule := scheduled[d.ID]
		if d.PeakPriced && !hasSchedule {
			t.Errorf("platform %q declares PeakPriced but has no peakSchedules entry, so its peak hours would bill at the off-peak rate silently", d.ID)
		}
		if !d.PeakPriced && hasSchedule {
			t.Errorf("peakSchedules prices %q but the registry does not declare it PeakPriced", d.ID)
		}
	}
}

// TestEachScheduleOwnsItsCoveredModels. peakSchedules lists the covered models
// per platform on purpose (the comment on the table says why: a change to one
// platform must not move another platform's money). Go and CommandCode once
// pointed at one shared map value, so they were not in fact independent - a
// model added for one silently became peak-priced for the other, which is the
// coupling the per-platform layout exists to prevent. Each now builds its own
// set, and this asserts each platform's set explicitly so a divergence has to
// be recorded rather than absorbed.
func TestEachScheduleOwnsItsCoveredModels(t *testing.T) {
	want := map[string][]string{
		"opencode-go": {"deepseek-v4-flash", "deepseek-v4.1-flash", "deepseek-v4-flash-vision-exp", "deepseek-v4-pro"},
		"commandcode": {"deepseek-v4-flash", "deepseek-v4.1-flash", "deepseek-v4-flash-vision-exp", "deepseek-v4-pro"},
		"cline-pass":  {"deepseek-v4-flash", "deepseek-v4.1-flash", "deepseek-v4-pro"},
		"openrouter":  {"deepseek-v4.1-flash", "deepseek-v4-pro-0813", "hy3"},
	}
	for provider, models := range want {
		s, ok := peakSchedules[provider]
		if !ok {
			t.Errorf("%s has no schedule", provider)
			continue
		}
		covered := make(map[string]bool)
		for _, r := range s.rules {
			for m := range r.models {
				covered[m] = true
			}
		}
		for _, m := range models {
			if !covered[m] {
				t.Errorf("%s does not cover %q", provider, m)
			}
		}
		if len(covered) != len(models) {
			got := make([]string, 0, len(covered))
			for m := range covered {
				got = append(got, m)
			}
			slices.Sort(got)
			t.Errorf("%s covers %v, want %v", provider, got, models)
		}
	}

	// The sets must be distinct values, not one map shared three ways. Contents
	// alone cannot tell those apart: a shared map passes the checks above and
	// then leaks - a model added for Go would price CommandCode's traffic at
	// peak, which is the coupling the per-platform layout exists to prevent.
	goSet := peakSchedules["opencode-go"].rules[0].models
	ccSet := peakSchedules["commandcode"].rules[0].models
	goSet["__sentinel_probe__"] = true
	if ccSet["__sentinel_probe__"] {
		t.Error("opencode-go and commandcode share one models map; adding a model for one would move the other platform's money")
	}
	delete(goSet, "__sentinel_probe__")
}

// TestOpenRouterPeakMatchesThePublishedOverrides pins the two OpenRouter rules
// against the payload they were transcribed from
// (openrouter.ai/api/v1/models, read 2026-09-22).
//
// The multiplier is stated against the catalog's stored base rate, and for hy3
// that is the discounted band rather than the headline one OpenRouter leads
// with, so the expected figure below is what actually reconciles the two
// sources. If models.dev ever adopts the headline band as the base, these
// numbers become wrong together and this test is where that shows up.
//
// The window endpoints are the interesting part: OpenRouter writes them as
// HHMM integers, and hy3's second window uses utc_end 0 for midnight.
func TestOpenRouterPeakMatchesThePublishedOverrides(t *testing.T) {
	mk := func(day time.Weekday, hour, min int) time.Time {
		base := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
		return base.AddDate(0, 0, int(day)-int(time.Monday)).Add(time.Duration(hour)*time.Hour + time.Duration(min)*time.Minute)
	}
	for _, c := range []struct {
		name  string
		model string
		t     time.Time
		want  float64
	}{
		// DeepSeek: 2x across 01-04 and 06-10 UTC on weekdays, base otherwise.
		{"deepseek weekday inside first window", "deepseek/deepseek-v4.1-flash", mk(time.Monday, 2, 0), 2},
		{"deepseek weekday inside second window", "deepseek/deepseek-v4-pro-0813", mk(time.Wednesday, 8, 0), 2},
		{"deepseek weekday between windows", "deepseek/deepseek-v4.1-flash", mk(time.Monday, 5, 0), 1},
		{"deepseek weekend is base", "deepseek/deepseek-v4.1-flash", mk(time.Saturday, 2, 0), 1},
		// hy3: 1.6x for 00:00-16:00 UTC every day, base 16:00-24:00. No day
		// condition and no holiday condition, so a weekend hour inside the
		// window is still peak.
		{"hy3 before dawn", "tencent/hy3", mk(time.Monday, 3, 0), 1.6},
		{"hy3 at the 16:00 close", "tencent/hy3", mk(time.Monday, 16, 0), 1},
		{"hy3 late evening is base", "tencent/hy3", mk(time.Monday, 23, 0), 1},
		{"hy3 weekend inside the window", "tencent/hy3", mk(time.Sunday, 9, 0), 1.6},
		{"hy3-preview has no override", "tencent/hy3-preview", mk(time.Monday, 3, 0), 1},
		// A model whose id carries the vendor prefix still resolves by family.
		{"vendor-prefixed id", "openrouter/deepseek/deepseek-v4.1-flash", mk(time.Monday, 2, 0), 2},
	} {
		if got := ProviderPeakMultiplier("openrouter", c.model, c.t); got != c.want {
			t.Errorf("%s: %s at %v = %v, want %v", c.name, c.model, c.t, got, c.want)
		}
	}
}

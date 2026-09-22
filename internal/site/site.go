// Package site names the upstream platforms this proxy can talk to, and the
// order they are presented in.
//
// Platform identity used to be restated wherever it was needed: a
// supported-provider switch in config, a display-rank switch next to it, and a
// third copy of the id constants in the sending client. The same platform could
// be described differently in two of those places and nothing would catch it.
// This package is that one description.
//
// It imports nothing but the standard library on purpose. Config, client,
// router, history and the GUI all need this metadata, and depending on any of
// them would turn the edge back into config into an import cycle.
//
// The registry carries identity only. Capabilities that need code - the
// protocol adapter, the peak schedule, the account API, the model catalog -
// stay in their own packages and are reached by id, so this stays a plain data
// description rather than a place for behaviour to hide.
package site

import "strings"

// Platform ids. These are the values written to requests.provider and accepted
// as a model's configured provider, so they are part of the stored contract and
// must not be renamed.
const (
	OpenCodeGo  = "opencode-go"
	OpenCodeZen = "opencode-zen"
	AWSBedrock  = "aws-bedrock"
	OpenRouter  = "openrouter"
	CommandCode = "commandcode"
	// ClinePass is Cline's flat-rate subscription. The id is the model id's own
	// prefix, not the vendor name, because platform identity here is the first
	// segment of a model key: catalog.ProviderFromModelKey splits
	// "cline-pass/glm-5.3" and the cost selector compares the result for
	// equality, so an id of "cline" would match none of that platform's models.
	// Cline's other pools (cline-free, cline-cloud) are separate namespaces and
	// would each be their own descriptor, sharing this one's config block.
	ClinePass = "cline-pass"
)

// Descriptor is one platform's identity.
type Descriptor struct {
	ID string
	// DisplayName is the label the model listing, the CLI and the dashboard
	// show. The dashboard's colour for a platform is not part of its identity
	// and stays in the frontend.
	DisplayName string
	// Order is the presentation order shared by the model listing, the CLI and
	// the dashboard. It is a display concern only: routing precedence, fallback
	// chains and key rotation order remain owned by their configured order.
	Order int
	// Visible reports whether the dashboard offers this platform in its
	// selectors and settings. Hiding one changes only what the dashboard lists:
	// its adapter, credentials, routing and stored history all keep working,
	// and records already stored under it still resolve a label.
	Visible bool
	// Default marks the platform an empty provider name means. Exactly one
	// descriptor sets it; TestDefaultPlatformIsUnique holds that.
	Default bool

	// RateTable names the published price table this platform's models are
	// priced from, or "" when the platform publishes no per-token prices this
	// proxy can use.
	//
	// It is a table name rather than a yes/no flag because the same model costs
	// different money on different platforms: deepseek-v4-flash is 0.22/0.66 per
	// million tokens on OpenCode Go and 0.15/0.60 on CommandCode. A single
	// shared table would silently price one platform's traffic with another
	// platform's rates, and the row would look perfectly well-formed.
	RateTable string
	// CacheCreationBilledAsInput reports that this platform bills cache
	// creation tokens at its input rate. Without it a request that used cache
	// creation has no computable price and is reported as unknown rather than
	// estimated from a rate this proxy would be inventing.
	CacheCreationBilledAsInput bool
	// PeakPriced reports that this platform publishes two rates for the same
	// model, one of them for named hours of the day.
	//
	// It is declared here so that "this platform has peak pricing but no rule in
	// history.peakSchedules" is a failing test rather than a silent over-charge.
	// That exact omission already shipped once: ClinePass publishes a Peak
	// column, had no schedule entry, and every one of its requests billed flat
	// at the off-peak rate with no badge to show for it (see the note on
	// history.peakSchedules). A schedule entry is the only thing that prices
	// those hours, so its absence is indistinguishable from a platform that has
	// no peak pricing - which is why the fact is stated in both places and
	// checked against itself.
	//
	// The flag is in the registry because the registry is keyed by platform id,
	// which is what a schedule entry is keyed by too; the registry itself stays
	// a plain data description and does not carry the rules.
	PeakPriced bool
}

var registry = []Descriptor{
	{ID: OpenCodeGo, DisplayName: "OpenCode Go", Order: 0, Visible: true, Default: true,
		RateTable: OpenCodeGo, CacheCreationBilledAsInput: true, PeakPriced: true},
	{ID: CommandCode, DisplayName: "CommandCode", Order: 1, Visible: true,
		RateTable: CommandCode, PeakPriced: true},
	{ID: ClinePass, DisplayName: "ClinePass", Order: 2, Visible: true,
		RateTable: ClinePass, PeakPriced: true},
	{ID: OpenCodeZen, DisplayName: "OpenCode Zen", Order: 3},
	{ID: AWSBedrock, DisplayName: "AWS Bedrock", Order: 4},
	// OpenRouter prices two of its models by the clock. Its windows are not a
	// platform-wide rule - they come from per-model `pricing.overrides` on its
	// own /api/v1/models endpoint, and the two models covered so far disagree on
	// both hours and multiplier - which is why history.peakSchedules states them
	// per model rather than per platform. They are transcribed there, not read
	// at runtime: models.dev, the catalog this proxy syncs, carries no overrides
	// at all.
	{ID: OpenRouter, DisplayName: "OpenRouter", Order: 5, PeakPriced: true},
}

// All returns every descriptor in presentation order. Callers must not assume
// they may mutate the result.
func All() []Descriptor { return registry }

// Visible returns the platforms the dashboard offers, in presentation order. A
// hidden platform is still returned by All and still resolves through Lookup,
// so stored records keep their label and configured-but-hidden platforms keep
// working.
func Visible() []Descriptor {
	out := make([]Descriptor, 0, len(registry))
	for _, d := range registry {
		if d.Visible {
			out = append(out, d)
		}
	}
	return out
}

// Hidden returns the platforms the dashboard does not offer, in presentation
// order. The dashboard still needs their labels to render stored records.
func Hidden() []Descriptor {
	out := make([]Descriptor, 0, len(registry))
	for _, d := range registry {
		if !d.Visible {
			out = append(out, d)
		}
	}
	return out
}

// Lookup returns the descriptor for a name and whether it names a known
// platform. The name is normalized first, so the legacy spellings resolve too.
func Lookup(provider string) (Descriptor, bool) {
	id := Normalize(provider)
	// Linear over a handful of entries on a path taken once per routing
	// decision; a map would cost more to keep in sync than it saves.
	for _, d := range registry {
		if d.ID == id {
			return d, true
		}
	}
	return Descriptor{}, false
}

// IsKnown reports whether the runtime has a platform with this name.
func IsKnown(provider string) bool {
	_, ok := Lookup(provider)
	return ok
}

// Order returns the presentation rank of a platform. Unknown names sort after
// every known one, which is what the previous rank switch did.
func Order(provider string) int {
	if d, ok := Lookup(provider); ok {
		return d.Order
	}
	return len(registry)
}

// DefaultID is the platform an empty provider name means.
func DefaultID() string {
	for _, d := range registry {
		if d.Default {
			return d.ID
		}
	}
	// Unreachable while exactly one descriptor sets Default; the test below
	// keeps that true. Returning the first entry rather than "" keeps a broken
	// registry from silently routing an unset provider to nowhere.
	return registry[0].ID
}

// Normalize turns a configured provider name into its canonical id: an empty
// name means the default platform, and the legacy underscore spelling is
// accepted.
func Normalize(provider string) string {
	if provider == "" {
		return DefaultID()
	}
	return strings.ReplaceAll(provider, "_", "-")
}

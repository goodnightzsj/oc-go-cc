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
}

var registry = []Descriptor{
	{ID: OpenCodeGo, DisplayName: "OpenCode Go", Order: 0, Visible: true, Default: true},
	{ID: CommandCode, DisplayName: "CommandCode", Order: 1, Visible: true},
	{ID: OpenCodeZen, DisplayName: "OpenCode Zen", Order: 2},
	{ID: AWSBedrock, DisplayName: "AWS Bedrock", Order: 3},
	{ID: OpenRouter, DisplayName: "OpenRouter", Order: 4},
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

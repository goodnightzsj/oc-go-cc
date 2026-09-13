package config

import (
	"testing"

	"github.com/routatic/proxy/internal/site"
)

// The registry says which platforms exist; the binding says where each one's
// credentials live. Config owns the binding because it knows its own fields,
// and the registry owns identity because everything else needs it. That split
// only holds if the two lists match: a platform in one and not the other would
// silently receive no keys at all, which reads downstream as "not configured"
// rather than as a wiring mistake.
func TestProviderKeySourceCoversRegistry(t *testing.T) {
	known := make(map[string]bool, len(site.All()))
	for _, d := range site.All() {
		known[d.ID] = true
		if _, ok := providerKeySource[d.ID]; !ok {
			t.Errorf("platform %q is registered but has no credential binding", d.ID)
		}
	}
	for id := range providerKeySource {
		if !known[id] {
			t.Errorf("credential binding %q does not correspond to a registered platform", id)
		}
	}
}

// The global-key fallback is a per-platform decision, not a default. CommandCode
// was deliberately given none: a global key issued for another host must not be
// sent to a host it was never issued for.
func TestGlobalKeyFallbackIsPerPlatform(t *testing.T) {
	global := &Config{APIKey: "global-key"}
	for _, d := range site.All() {
		got := len(global.ProviderAPIKeys(d.ID))
		want := 0
		if providerKeySource[d.ID].globalFallback {
			want = 1
		}
		if got != want {
			t.Errorf("ProviderAPIKeys(%q) with only a global key = %d keys, want %d", d.ID, got, want)
		}
	}
}

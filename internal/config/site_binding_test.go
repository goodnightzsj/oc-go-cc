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

// The registry says which platforms exist; each binding says where that
// platform's credentials and timeouts live. A platform missing from a binding
// falls back to the default platform's values silently, so the two lists have
// to match rather than merely compile.
func TestProviderTimeoutSourceCoversRegistry(t *testing.T) {
	known := make(map[string]bool, len(site.All()))
	for _, d := range site.All() {
		known[d.ID] = true
		if _, ok := providerTimeoutSource[d.ID]; !ok {
			t.Errorf("platform %q is registered but has no timeout binding", d.ID)
		}
	}
	for id := range providerTimeoutSource {
		if !known[id] {
			t.Errorf("timeout binding %q does not correspond to a registered platform", id)
		}
	}
}

// Each value falls back to the platform's own overall timeout, and an unknown
// platform uses the default platform's block rather than none at all - which is
// what the per-platform switches this replaced did.
func TestProviderTimeoutsFallBackWithinThePlatform(t *testing.T) {
	cfg := &Config{
		CommandCode: CommandCodeConfig{TimeoutMs: 1000},
		OpenCodeGo:  OpenCodeGoConfig{TimeoutMs: 2000, StreamTimeoutMs: 300, StreamingTimeoutMs: 4000},
		OpenRouter:  OpenRouterConfig{TimeoutMs: 7000},
		OpenCodeZen: OpenCodeZenConfig{TimeoutMs: 100},
		AWSBedrock:  AWSBedrockConfig{TimeoutMs: 9000},
	}
	got := cfg.ProviderTimeouts(site.CommandCode)
	if got.RequestMs != 1000 || got.StreamIdleMs != 1000 || got.StreamingTotalMs != 1000 {
		t.Errorf("unset idle/total did not fall back to the platform timeout: %+v", got)
	}
	got = cfg.ProviderTimeouts(site.OpenCodeGo)
	if got.RequestMs != 2000 || got.StreamIdleMs != 300 || got.StreamingTotalMs != 4000 {
		t.Errorf("explicit values were not honoured: %+v", got)
	}
	if got := cfg.ProviderTimeouts("not-a-platform"); got.RequestMs != 2000 {
		t.Errorf("an unknown platform did not use the default platform's block: %+v", got)
	}
}

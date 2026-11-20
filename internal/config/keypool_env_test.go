package config

import (
	"testing"

	"github.com/routatic/proxy/internal/site"
)

// Every platform with a key block must appear in keyPoolFields, because that
// table is the only thing that reads the documented *_API_KEY environment
// variables, folds api_key into api_keys, and gives the dashboard a row per
// credential. WorkBuddy was missing, so `init --provider workbuddy` printed an
// export line that the loader ignored.
func TestKeyPoolFieldsCoversEveryPlatformWithCredentials(t *testing.T) {
	covered := make(map[string]bool, len(keyPoolFields))
	for _, row := range keyPoolFields {
		covered[row.provider] = true
	}
	for _, descriptor := range site.All() {
		if !providerHasCredentialBlock(descriptor.ID) {
			continue
		}
		if !covered[descriptor.ID] {
			t.Errorf("platform %q has a key block but no keyPoolFields row", descriptor.ID)
		}
	}
}

func providerHasCredentialBlock(provider string) bool {
	source, ok := providerKeySource[site.Normalize(provider)]
	return ok && source.keys != nil
}

func TestWorkBuddyEnvOverridesReachTheConfig(t *testing.T) {
	t.Setenv("ROUTATIC_PROXY_WORKBUDDY_API_KEY", "synthetic-gateway-key")
	cfg := &Config{}
	if err := applyEnvOverrides(cfg); err != nil {
		t.Fatalf("apply env overrides: %v", err)
	}
	if cfg.WorkBuddy.APIKey != "synthetic-gateway-key" {
		t.Fatalf("workbuddy key = %q, want the environment value", cfg.WorkBuddy.APIKey)
	}
	if keys := cfg.ProviderAPIKeys(site.WorkBuddy); len(keys) != 1 {
		t.Fatalf("pool = %v, want the single environment key", keys)
	}
}

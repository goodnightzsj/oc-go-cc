package config

import "testing"

func TestProviderCredentialsOverrideGlobalPool(t *testing.T) {
	cfg := &Config{
		APIKeys:    []string{"global-one", "global-two"},
		OpenCodeGo: OpenCodeGoConfig{APIKey: "go-only"},
	}
	if keys := cfg.ProviderAPIKeys("opencode_go"); len(keys) != 1 || keys[0] != "go-only" {
		t.Fatal("dedicated key must override the global pool")
	}
	if keys := cfg.ProviderAPIKeys("opencode-zen"); len(keys) != 2 {
		t.Fatal("legacy providers retain the global fallback")
	}
	if keys := cfg.ProviderAPIKeys("unknown"); len(keys) != 0 {
		t.Fatal("unknown provider must not receive credentials")
	}
	if err := validate(&Config{OpenCodeGo: OpenCodeGoConfig{APIKey: "go-only"}}); err != nil {
		t.Fatalf("provider-only config rejected: %v", err)
	}
}

func TestValidateAllRoutingTargets(t *testing.T) {
	for _, section := range []string{"models", "fallbacks", "model_overrides", "model_family_overrides"} {
		for _, model := range []ModelConfig{
			{Provider: "unknown", ModelID: "test"},
			{Provider: "opencode-go", ModelID: "test", WireFormat: "typo"},
			{Provider: "opencode-go"},
		} {
			t.Run(section+"/"+model.Provider+"/"+model.WireFormat+"/"+model.ModelID, func(t *testing.T) {
				cfg := &Config{APIKey: "test-key"}
				switch section {
				case "models":
					cfg.Models = map[string]ModelConfig{"default": model}
				case "fallbacks":
					cfg.Fallbacks = map[string][]ModelConfig{"default": {model}}
				case "model_overrides":
					cfg.ModelOverrides = map[string]ModelConfig{"alias": model}
				case "model_family_overrides":
					cfg.ModelFamilyOverrides = map[string]ModelConfig{"sonnet": model}
				}
				if err := validate(cfg); err == nil {
					t.Fatalf("invalid target accepted in %s", section)
				}
			})
		}
	}
}

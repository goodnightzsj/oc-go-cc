package catalog

import "testing"

func ptr(b bool) *bool { return &b }

func TestValidateEnabledProviders_Valid(t *testing.T) {
	cat := Catalog{
		Providers: map[string]Provider{
			"openai":    {Name: "openai", Enabled: nil}, // nil means enabled
			"anthropic": {Name: "anthropic", Enabled: ptr(true)},
			"disabled":  {Name: "disabled", Enabled: ptr(false)},
		},
		Models: map[string]Model{
			"openai/gpt-4":       {ID: "openai/gpt-4", Name: "gpt-4"},
			"anthropic/claude-3": {ID: "anthropic/claude-3", Name: "claude-3"},
		},
	}

	if err := validateEnabledProviders(cat); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateEnabledProviders_NoEnabledProviders(t *testing.T) {
	cat := Catalog{
		Providers: map[string]Provider{
			"a": {Name: "a", Enabled: ptr(false)},
			"b": {Name: "b", Enabled: ptr(false)},
		},
		Models: map[string]Model{"a/m": {ID: "a/m"}},
	}

	if err := validateEnabledProviders(cat); err == nil {
		t.Fatal("expected error when every provider is disabled")
	}
}

func TestValidateEnabledProviders_EmptyMaps(t *testing.T) {
	if err := validateEnabledProviders(Catalog{Models: map[string]Model{"a/m": {ID: "a/m"}}}); err == nil {
		t.Error("expected error for empty providers map")
	}
	if err := validateEnabledProviders(Catalog{Providers: map[string]Provider{"a": {Name: "a"}}}); err == nil {
		t.Error("expected error for empty models map")
	}
}

func TestValidateEnabledProviders_ModelsNoMatchEnabledProviders(t *testing.T) {
	cat := Catalog{
		Providers: map[string]Provider{
			"enabled":  {Name: "enabled", Enabled: ptr(true)},
			"disabled": {Name: "disabled", Enabled: ptr(false)},
		},
		// Every model belongs to the disabled provider.
		Models: map[string]Model{"disabled/m": {ID: "disabled/m"}},
	}

	if err := validateEnabledProviders(cat); err == nil {
		t.Fatal("expected error when no model references an enabled provider")
	}
}

package config

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestOpenRouterManagementKeyConfiguration(t *testing.T) {
	t.Setenv("ROUTATIC_PROXY_OPENROUTER_MANAGEMENT_API_KEY", "synthetic-management-env")
	cfg, err := LoadJSON([]byte(`{"api_key":"synthetic-global","openrouter":{"api_keys":["synthetic-inference"],"management_api_key":"synthetic-management-file"}}`))
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(cfg.OpenRouter)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	if fields["management_api_key"] != "synthetic-management-env" {
		t.Fatal("dedicated management-key environment override was not loaded")
	}
	keys := cfg.ProviderAPIKeys("openrouter")
	if len(keys) != 1 || keys[0] != "synthetic-inference" {
		t.Fatal("management key entered the inference key pool")
	}
	t.Setenv("ROUTATIC_PROXY_OPENROUTER_MANAGEMENT_API_KEY", "")
	if _, err := LoadJSON([]byte(`{"api_key":"synthetic-global","openrouter":{"management_api_key":"${UNSET_MANAGEMENT_TEST_KEY}"}}`)); err == nil || !strings.Contains(err.Error(), "openrouter.management_api_key") {
		t.Fatal("unresolved management-key placeholder was accepted")
	}
	if _, err := LoadJSON([]byte(`{"openrouter":{"management_api_key":"synthetic-management-only"}}`)); err == nil {
		t.Fatal("management-only credentials must not authorize inference")
	}
}

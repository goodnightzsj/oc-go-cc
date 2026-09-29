package config

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCommandCodeConfiguration(t *testing.T) {
	cfg, err := LoadJSON([]byte(`{"commandcode":{"api_key":"commandcode-only"},"models":{"default":{"provider":"commandcode","model_id":"claude-sonnet-4-6"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if !SupportedProvider("commandcode") || len(cfg.ProviderAPIKeys("commandcode")) != 1 {
		t.Fatal("CommandCode must be independently configured and supported")
	}
	encoded, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var data map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &data); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data["commandcode"]), "https://api.commandcode.ai/provider/v1/chat/completions") ||
		!strings.Contains(string(data["commandcode"]), "https://api.commandcode.ai/provider/v1/messages") {
		t.Fatal("missing official default endpoints")
	}
	global := &Config{APIKey: "other-platform"}
	if len(global.ProviderAPIKeys("commandcode")) != 0 {
		t.Fatal("CommandCode must not receive a legacy global key")
	}
}

func TestCommandCodeEnvironment(t *testing.T) {
	t.Setenv("ROUTATIC_PROXY_COMMANDCODE_API_KEY", "single-env")
	t.Setenv("ROUTATIC_PROXY_COMMANDCODE_API_KEYS", "first-env, second-env")
	t.Setenv("ROUTATIC_PROXY_COMMANDCODE_URL", "http://localhost:1234/chat/completions")
	t.Setenv("ROUTATIC_PROXY_COMMANDCODE_ANTHROPIC_URL", "http://localhost:1234/messages")
	cfg, err := LoadJSON([]byte(`{"commandcode":{"api_key":"file-key"}}`))
	if err != nil {
		t.Fatal(err)
	}
	// Both environment variables pool, single first, and the file's key is not
	// discarded either: three credentials, not two. The old rule dropped the
	// single env key whenever the array was set.
	keys := cfg.ProviderAPIKeys("commandcode")
	want := []string{"single-env", "first-env", "second-env"}
	if len(keys) != len(want) {
		t.Fatalf("provider key pool = %v, want %v", keys, want)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("provider key pool = %v, want %v", keys, want)
		}
	}
}

func TestCommandCodeRejectsInvalidConfiguration(t *testing.T) {
	for _, input := range []string{
		`{"commandcode":{"api_key":"${UNSET_COMMANDCODE_TEST_KEY}"}}`,
		`{"commandcode":{"api_key":"key","base_url":"ftp://example.test/chat"}}`,
		`{"commandcode":{"api_key":"key","anthropic_base_url":"/messages"}}`,
		`{"commandcode":{"api_key":"key","timeout_ms":-1}}`,
		`{"commandcode":{"api_key":"key"},"models":{"default":{"provider":"commandcode","model_id":"test","wire_format":"responses"}}}`,
		`{"commandcode":{"api_key":"key"},"models":{"default":{"provider":"commandcode","model_id":"test","wire_format":"gemini"}}}`,
	} {
		if _, err := LoadJSON([]byte(input)); err == nil {
			t.Fatalf("invalid config accepted: %s", input)
		}
	}
}

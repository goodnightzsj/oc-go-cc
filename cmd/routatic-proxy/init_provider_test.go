package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/site"
	"strings"
)

func TestInitCmd_ProviderFlag(t *testing.T) {
	tests := []struct {
		name           string
		provider       string
		wantProvider   string
		wantErr        bool
		wantErrContain string
	}{
		{
			name:         "openrouter provider",
			provider:     "openrouter",
			wantProvider: "openrouter",
		},
		{
			name:         "aws-bedrock provider",
			provider:     "aws-bedrock",
			wantProvider: "aws-bedrock",
		},
		{
			name:         "opencode-zen provider",
			provider:     "opencode-zen",
			wantProvider: "opencode-zen",
		},
		{
			name:         "opencode-go provider",
			provider:     "opencode-go",
			wantProvider: "opencode-go",
		},
		{
			name:         "commandcode provider",
			provider:     "commandcode",
			wantProvider: "commandcode",
		},
		{
			name:           "unknown provider returns error",
			provider:       "unknown-provider",
			wantErr:        true,
			wantErrContain: "unknown provider",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory
			tmpDir := t.TempDir()
			t.Setenv("HOME", tmpDir)
			t.Setenv("USERPROFILE", tmpDir)
			configPath := filepath.Join(tmpDir, "config.json")

			// Use ROUTATIC_PROXY_CONFIG to control config location
			t.Setenv("ROUTATIC_PROXY_CONFIG", configPath)

			// Create init command
			cmd := initCmd()
			cmd.SetArgs([]string{"--provider", tt.provider})

			// Run the command
			err := cmd.Execute()

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.wantErrContain)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			// Verify config file was created
			data, err := os.ReadFile(configPath)
			if err != nil {
				t.Errorf("failed to read config file: %v", err)
				return
			}

			// Parse and verify it's valid JSON
			var cfg map[string]any
			if err := json.Unmarshal(data, &cfg); err != nil {
				t.Errorf("config is not valid JSON: %v", err)
				return
			}

			// Check that models section exists
			models, ok := cfg["models"].(map[string]any)
			if !ok {
				t.Error("config missing 'models' section")
				return
			}

			// Verify default model uses the correct provider
			defaultModel, ok := models["default"].(map[string]any)
			if !ok {
				t.Error("config missing 'models.default' section")
				return
			}

			provider, _ := defaultModel["provider"].(string)
			if provider != tt.wantProvider {
				t.Errorf("default model provider = %q, want %q", provider, tt.wantProvider)
			}
		})
	}
}

func TestInitCmd_NoProviderFlag(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("USERPROFILE", tmpDir)
	configPath := filepath.Join(tmpDir, "config.json")

	// Use ROUTATIC_PROXY_CONFIG to control config location
	t.Setenv("ROUTATIC_PROXY_CONFIG", configPath)

	// Create init command without provider flag
	cmd := initCmd()

	// Run the command
	err := cmd.Execute()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
		return
	}

	// Verify config file was created
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Errorf("failed to read config file: %v", err)
		return
	}

	// Parse and verify it's valid JSON
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Errorf("config is not valid JSON: %v", err)
		return
	}

	// Default config should use opencode-go
	models, ok := cfg["models"].(map[string]any)
	if !ok {
		t.Error("config missing 'models' section")
		return
	}

	defaultModel, ok := models["default"].(map[string]any)
	if !ok {
		t.Error("config missing 'models.default' section")
		return
	}

	provider, _ := defaultModel["provider"].(string)
	if provider != "opencode-go" {
		t.Errorf("default model provider = %q, want 'opencode-go'", provider)
	}
}

func TestInitCmd_ConfigAlreadyExists(t *testing.T) {
	// Create temp directory with existing config
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("USERPROFILE", tmpDir)
	configPath := filepath.Join(tmpDir, "config.json")

	// Write existing config
	if err := os.WriteFile(configPath, []byte(`{"existing": true}`), 0600); err != nil {
		t.Fatalf("failed to write existing config: %v", err)
	}

	// Use ROUTATIC_PROXY_CONFIG to control config location
	t.Setenv("ROUTATIC_PROXY_CONFIG", configPath)

	// Create init command
	cmd := initCmd()

	// Run the command - should not error, just inform user
	err := cmd.Execute()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
		return
	}

	// Verify existing config was NOT overwritten
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Errorf("failed to read config file: %v", err)
		return
	}

	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Errorf("config is not valid JSON: %v", err)
		return
	}

	if _, ok := cfg["existing"]; !ok {
		t.Error("existing config was overwritten")
	}
}

func TestGetProviderConfig_UnknownProvider(t *testing.T) {
	_, err := getProviderConfig("invalid")
	if err == nil {
		t.Error("expected error for unknown provider")
	}
}

// TestGetProviderConfig_AllProviders runs over the registry rather than a
// hand-written list, which is what it used to be. That list had five names and
// omitted cline-pass, so `init --provider cline-pass` would have failed at
// runtime with "unknown provider" and no test would have said so.
// providerJSONBlock names the config block each preset's template fills.
var providerJSONBlock = map[string]string{
	"opencode-go":  "opencode_go",
	"opencode-zen": "opencode_zen",
	"aws-bedrock":  "aws_bedrock",
	"openrouter":   "openrouter",
	"commandcode":  "commandcode",
	"cline-pass":   "cline_pass",
	"workbuddy":    "workbuddy",
}

func TestGetProviderConfig_AllProviders(t *testing.T) {
	providers := make([]string, 0, len(site.All()))
	for _, d := range site.All() {
		providers = append(providers, d.ID)
		if _, ok := providerPresets[d.ID]; !ok {
			t.Errorf("platform %q is registered but has no provider preset, so init --provider %s would fail at runtime", d.ID, d.ID)
		}
	}

	for _, provider := range providers {
		t.Run(provider, func(t *testing.T) {
			config, err := getProviderConfig(provider)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			// Verify it's valid JSON
			var cfg map[string]any
			if err := json.Unmarshal([]byte(config), &cfg); err != nil {
				t.Errorf("config for %s is not valid JSON: %v", provider, err)
				return
			}

			// Verify models section exists
			models, ok := cfg["models"].(map[string]any)
			if !ok {
				t.Errorf("config for %s missing 'models' section", provider)
				return
			}

			// Verify default model uses correct provider
			defaultModel, ok := models["default"].(map[string]any)
			if !ok {
				t.Errorf("config for %s missing 'models.default' section", provider)
				return
			}

			actualProvider, _ := defaultModel["provider"].(string)
			if actualProvider != provider {
				t.Errorf("default model provider = %q, want %q", actualProvider, provider)
			}

			// Every template must agree on the listening port and on the
			// platform's published endpoints. They are literals in six places
			// while the runtime defaults live in internal/config, so nothing
			// but this comparison keeps the two from drifting apart.
			if port, _ := cfg["port"].(float64); port != 3456 {
				t.Errorf("port = %v, want 3456", port)
			}
			// Each template restates its platform's base_url as a literal; the
			// preset carries the same value. A typo here is only discovered by an
			// operator whose freshly generated config cannot reach anything.
			// WorkBuddy is the one platform whose config takes the gateway origin
			// rather than the chat path: the model roster and the request path are
			// both derived from it.
			want := providerPresets[provider].BaseURL
			if provider == "workbuddy" {
				want = strings.TrimSuffix(strings.TrimSuffix(want, "/v1/chat/completions"), "/")
			}
			block, _ := cfg[providerJSONBlock[provider]].(map[string]any)
			if got, _ := block["base_url"].(string); got != want {
				t.Errorf("%s block base_url = %q, want %q", provider, got, providerPresets[provider].BaseURL)
			}
		})
	}
}

// TestDefaultAndOpenCodeGoConfigsIdentical asserts the two default generators
// return byte-identical content. Both now read the single embedded
// templates/default_config.json, so this is a regression guard against anyone
// re-introducing a divergent hand-written copy.
func TestDefaultAndOpenCodeGoConfigsIdentical(t *testing.T) {
	if getDefaultConfig() != getOpenCodeGoConfig() {
		t.Error("getDefaultConfig() and getOpenCodeGoConfig() must return identical content (single source: templates/default_config.json)")
	}
}

// TestExampleConfigSharedModelsMatchDefault guards the remaining, deliberate
// duplication: configs/config.example.json is a documented superset of the
// embedded default (it adds cost_routing, aws_bedrock, extra models, etc.).
// For every model alias the two share, the definitions must be identical so
// the example never documents a stale shape (e.g. a missing vision flag).
func TestExampleConfigSharedModelsMatchDefault(t *testing.T) {
	def := mustParseConfig(t, getDefaultConfig())

	raw, err := os.ReadFile(filepath.Join("..", "..", "configs", "config.example.json"))
	if err != nil {
		t.Fatalf("read config.example.json: %v", err)
	}
	example := mustParseConfig(t, string(raw))

	defModels, _ := def["models"].(map[string]any)
	exModels, _ := example["models"].(map[string]any)

	for alias, defModel := range defModels {
		exModel, ok := exModels[alias]
		if !ok {
			t.Errorf("model %q exists in default config but is missing from config.example.json", alias)
			continue
		}
		defJSON, _ := json.Marshal(defModel)
		exJSON, _ := json.Marshal(exModel)
		if string(defJSON) != string(exJSON) {
			t.Errorf("model %q differs between default config and config.example.json:\n  default: %s\n  example: %s", alias, defJSON, exJSON)
		}
	}
}

// TestKimiK3InDefaultConfig ensures the kimi-k3 alias and its fallback chain
// are present in the embedded default (config.example.json is checked by
// TestExampleConfigSharedModelsMatchDefault).
func TestKimiK3InDefaultConfig(t *testing.T) {
	cfg := mustParseConfig(t, getDefaultConfig())
	models, _ := cfg["models"].(map[string]any)
	if _, ok := models["kimi-k3"]; !ok {
		t.Error("models.kimi-k3 missing from default config")
	}
	fallbacks, _ := cfg["fallbacks"].(map[string]any)
	if _, ok := fallbacks["kimi-k3"]; !ok {
		t.Error("fallbacks.kimi-k3 missing from default config")
	}
}

func mustParseConfig(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("invalid JSON config: %v", err)
	}
	return m
}

func TestCommandCodePresetIsIndependent(t *testing.T) {
	t.Setenv("ROUTATIC_PROXY_COMMANDCODE_API_KEY", "test-commandcode-key")
	t.Setenv("ROUTATIC_PROXY_API_KEY", "")
	t.Setenv("OC_GO_CC_API_KEY", "")
	raw, err := getProviderConfig("commandcode")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadJSON([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.EffectiveAPIKeys()) != 0 || len(cfg.ProviderAPIKeys("commandcode")) != 1 {
		t.Fatal("preset must use only CommandCode credentials")
	}
	for name, model := range cfg.Models {
		if model.Provider != "commandcode" {
			t.Fatalf("model %s points at another provider", name)
		}
	}
	if cfg.CommandCode.BaseURL != "https://api.commandcode.ai/provider/v1/chat/completions" ||
		cfg.CommandCode.AnthropicBaseURL != "https://api.commandcode.ai/provider/v1/messages" {
		t.Fatal("preset must call the official complete endpoints")
	}
	defaultVision, defaultVisionKnown := config.ModelHasVision(cfg.Models["default"])
	if cfg.Models["default"].ModelID != "claude-sonnet-4-6" || !defaultVisionKnown || !defaultVision {
		t.Fatal("default must use native Messages with image support")
	}
}

// Every preset must produce a file that its own printed export line satisfies.
// Three of them used to carry a top-level "${ROUTATIC_PROXY_API_KEY}" pool next
// to the provider block, so following the instructions left a placeholder that
// validate and start both reject.
func TestProviderPresetsValidateWithTheirOwnEnvVar(t *testing.T) {
	for name, preset := range providerPresets {
		t.Run(name, func(t *testing.T) {
			text := preset.Generator()
			t.Setenv(preset.EnvVarName, "synthetic-key-for-validation")
			cfg, err := config.LoadJSON([]byte(text))
			if err != nil {
				t.Fatalf("preset %s does not load with %s set: %v", name, preset.EnvVarName, err)
			}
			if len(cfg.ProviderAPIKeys(name)) == 0 {
				t.Fatalf("preset %s produced no credential for its own platform", name)
			}
		})
	}
}

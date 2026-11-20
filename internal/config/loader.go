package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

const (
	defaultConfigPath       = "~/.config/routatic-proxy/config.json"
	legacyConfigPath        = "~/.config/oc-go-cc/config.json"
	defaultHost             = "127.0.0.1"
	defaultPort             = 3456
	defaultBaseURL          = "https://opencode.ai/zen/go/v1/chat/completions"
	defaultAnthropicBaseURL = "https://opencode.ai/zen/go/v1/messages"
	defaultResponsesBaseURL = "https://opencode.ai/zen/go/v1/responses"
	defaultTimeoutMs        = 300000
	defaultLogLevel         = "info"
	defaultAnthropicAPIURL  = "https://api.anthropic.com"
	defaultCatalogMaxAge    = 24
	defaultCatalogSourceURL = "https://models.dev/catalog.json"

	defaultZenBaseURL          = "https://opencode.ai/zen/v1/chat/completions"
	defaultZenAnthropicBaseURL = "https://opencode.ai/zen/v1/messages"
	defaultZenResponsesBaseURL = "https://opencode.ai/zen/v1/responses"
	defaultZenGeminiBaseURL    = "https://opencode.ai/zen/v1/models"

	defaultOpenRouterBaseURL           = "https://openrouter.ai/api/v1/chat/completions"
	defaultCommandCodeBaseURL          = "https://api.commandcode.ai/provider/v1/chat/completions"
	defaultCommandCodeAnthropicBaseURL = "https://api.commandcode.ai/provider/v1/messages"
	defaultCommandCodeResponsesBaseURL = "https://api.commandcode.ai/provider/v1/responses"
	defaultClinePassBaseURL            = "https://api.cline.bot/api/v1/chat/completions"
	DefaultClinePassChannel            = "deepseek"
)

// envVarPattern matches ${ENV_VAR} placeholders in config values.
var envVarPattern = regexp.MustCompile(`\$\{([A-Za-z0-9_]+)\}`)

// parseCommaSeparatedKeys splits a comma-separated string of API keys.
// Empty entries are filtered out. Returns nil if no valid keys found.
func parseCommaSeparatedKeys(v string) []string {
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	var keys []string
	for _, part := range parts {
		key := strings.TrimSpace(part)
		if key != "" {
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return nil
	}
	return keys
}

var legacyEnvNames = map[string]string{
	"ROUTATIC_PROXY_CONFIG":           "OC_GO_CC_CONFIG",
	"ROUTATIC_PROXY_API_KEY":          "OC_GO_CC_API_KEY",
	"ROUTATIC_PROXY_HOST":             "OC_GO_CC_HOST",
	"ROUTATIC_PROXY_PORT":             "OC_GO_CC_PORT",
	"ROUTATIC_PROXY_OPENCODE_URL":     "OC_GO_CC_OPENCODE_URL",
	"ROUTATIC_PROXY_OPENCODE_ZEN_URL": "OC_GO_CC_OPENCODE_ZEN_URL",
	"ROUTATIC_PROXY_LOG_LEVEL":        "OC_GO_CC_LOG_LEVEL",
}

// Load reads configuration from a JSON file and applies environment variable overrides.
// Config path resolution:
//  1. ROUTATIC_PROXY_CONFIG env var (explicit override)
//  2. OC_GO_CC_CONFIG env var (legacy explicit override)
//  3. ~/.config/routatic-proxy/config.json (default)
//  4. ~/.config/oc-go-cc/config.json (legacy fallback when the new path is absent)
func Load() (*Config, error) {
	return LoadFromPath(ResolveConfigPath())
}

// LoadFromPath reads configuration from the given JSON file path.
//
// An unknown key is a warning here, not an error: refusing to start over a key
// this version does not know would turn an upgrade into an outage. `validate`
// fails on the same input, which is where an operator is asking the question.
func LoadFromPath(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("loading config from %s: %w", path, err)
	}
	cfg, err := LoadJSON(data)
	if err != nil {
		return nil, err
	}
	if unknown := UnknownFields(data); unknown != nil {
		slog.Warn("config has a key this version does not recognise; it is being ignored",
			"path", path, "error", unknown)
	}
	return cfg, nil
}

// stripCommentKeys removes "//"-prefixed keys at every level. The example
// config uses them inside blocks too (anthropic_first, workbuddy), and a
// top-level-only sweep made `validate` reject the repository's own reference
// file.
func stripCommentKeys(value map[string]json.RawMessage) map[string]json.RawMessage {
	for key, raw := range value {
		if strings.HasPrefix(key, "//") {
			delete(value, key)
			continue
		}
		var nested map[string]json.RawMessage
		if len(raw) > 0 && raw[0] == '{' && json.Unmarshal(raw, &nested) == nil {
			value[key], _ = json.Marshal(stripCommentKeys(nested))
		}
	}
	return value
}

// CheckUnknownFields reports an unrecognised key in the file at path. The
// validate command uses it; the running proxy only warns.
func CheckUnknownFields(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return UnknownFields(data)
}

// UnknownFields reports the first key in data that no Config field owns.
//
// Two other decoders in this repository already reject unknown fields, and the
// cost_routing.scenarios validator exists precisely because a misspelled key
// that does nothing is worse than a rejected one. The top level was the
// exception: "modles" was accepted in silence and the routing block simply
// vanished.
//
// Keys beginning with "//" are the project's way of writing commented-out
// entries in a JSON example; they are dropped before the check rather than
// reported.
func UnknownFields(data []byte) error {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return err
	}
	cleaned, err := json.Marshal(stripCommentKeys(top))
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(cleaned))
	decoder.DisallowUnknownFields()
	var probe Config
	if err := decoder.Decode(&probe); err != nil {
		return err
	}
	return nil
}

// endpointSettings lists every URL-valued setting, so one rule covers them all.
func endpointSettings(cfg *Config) []struct{ Field, Value string } {
	var out []struct{ Field, Value string }
	add := func(field, value string) {
		out = append(out, struct{ Field, Value string }{field, value})
	}
	add("host", cfg.Host)
	add("catalog.source_url", cfg.Catalog.SourceURL)
	add("anthropic_first.base_url", cfg.AnthropicFirst.BaseURL)
	add("opencode_go.base_url", cfg.OpenCodeGo.BaseURL)
	add("opencode_go.anthropic_base_url", cfg.OpenCodeGo.AnthropicBaseURL)
	add("opencode_go.responses_base_url", cfg.OpenCodeGo.ResponsesBaseURL)
	add("opencode_zen.base_url", cfg.OpenCodeZen.BaseURL)
	add("opencode_zen.anthropic_base_url", cfg.OpenCodeZen.AnthropicBaseURL)
	add("opencode_zen.responses_base_url", cfg.OpenCodeZen.ResponsesBaseURL)
	add("opencode_zen.gemini_base_url", cfg.OpenCodeZen.GeminiBaseURL)
	add("aws_bedrock.base_url", cfg.AWSBedrock.BaseURL)
	add("aws_bedrock.anthropic_base_url", cfg.AWSBedrock.AnthropicBaseURL)
	add("openrouter.base_url", cfg.OpenRouter.BaseURL)
	add("commandcode.base_url", cfg.CommandCode.BaseURL)
	add("commandcode.anthropic_base_url", cfg.CommandCode.AnthropicBaseURL)
	add("commandcode.responses_base_url", cfg.CommandCode.ResponsesBaseURL)
	add("cline_pass.base_url", cfg.ClinePass.BaseURL)
	add("workbuddy.base_url", cfg.WorkBuddy.BaseURL)
	return out
}

// LoadJSON validates a config using the same environment and defaults as a
// disk load. The input remains unchanged so callers can persist placeholders.
func LoadJSON(data []byte) (*Config, error) {
	var cfg Config
	if err := json.Unmarshal([]byte(interpolateEnvVars(string(data))), &cfg); err != nil {
		return nil, fmt.Errorf("parsing JSON: %w", err)
	}

	if err := applyEnvOverrides(&cfg); err != nil {
		return nil, err
	}
	// Note where each pool entry came from, now that both sources have merged.
	if err := recordKeySources(&cfg, data); err != nil {
		return nil, err
	}
	applyDefaults(&cfg)

	if err := validate(&cfg); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	return &cfg, nil
}

// ResolveConfigPath determines which config file to load.
func ResolveConfigPath() string {
	if path := envValue("ROUTATIC_PROXY_CONFIG"); path != "" {
		return ExpandHome(path)
	}
	path := ExpandHome(defaultConfigPath)
	if _, err := os.Stat(path); err == nil {
		return path
	}
	legacyPath := ExpandHome(legacyConfigPath)
	if _, err := os.Stat(legacyPath); err == nil {
		return legacyPath
	}
	return path
}

// ExpandHome resolves a leading "~/" against the user's home directory. It
// returns the path unchanged if the home directory cannot be determined.
func ExpandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

// interpolateEnvVars replaces ${ENV_VAR} patterns with their actual values.
func interpolateEnvVars(s string) string {
	return envVarPattern.ReplaceAllStringFunc(s, func(match string) string {
		// Extract variable name from ${VAR}
		varName := match[2 : len(match)-1]
		if val := envValue(varName); val != "" {
			return val
		}
		// Leave unchanged if env var is not set
		return match
	})
}

// InterpolateEnvVars exposes the loader's ${VAR} substitution so callers can
// compare a value read from disk against one the process is actually using.
//
// The settings API needs this: keys leave the process as labels derived from
// the resolved credential, while the file may still hold a ${VAR} placeholder.
// Comparing a label against the unresolved text would report every
// environment-backed key as changed. Callers must not use the result for
// anything but comparison - writing it back would put the resolved secret on
// disk, which is exactly what the redaction round trip exists to prevent.
func InterpolateEnvVars(value string) string {
	return interpolateEnvVars(value)
}

// applyEnvOverrides applies environment variable overrides to the config.
func applyEnvOverrides(cfg *Config) error {
	// Overrides target each field independently; migration must preserve that layout.
	for _, row := range keyPoolFields {
		if v := envValue(row.envPrefix + "_API_KEY"); v != "" {
			*row.single(cfg) = v
		}
		if v := envValue(row.envPrefix + "_API_KEYS"); v != "" {
			*row.many(cfg) = parseCommaSeparatedKeys(v)
		}
	}
	if v := envValue("ROUTATIC_PROXY_AWS_BILLING_ENABLED"); v != "" {
		enabled, err := strconv.ParseBool(v)
		if err != nil {
			return invalid("", "invalid_environment", "ROUTATIC_PROXY_AWS_BILLING_ENABLED must be a boolean")
		}
		cfg.AWSBedrock.Billing.Enabled = enabled
	}
	if v := envValue("ROUTATIC_PROXY_AWS_BILLING_PROFILE"); v != "" {
		cfg.AWSBedrock.Billing.Profile = v
	}
	if v := envValue("ROUTATIC_PROXY_AWS_BILLING_LINKED_ACCOUNT_ID"); v != "" {
		cfg.AWSBedrock.Billing.LinkedAccountID = v
	}

	if v := envValue("ROUTATIC_PROXY_OPENROUTER_MANAGEMENT_API_KEY"); v != "" {
		cfg.OpenRouter.ManagementAPIKey = v
	}
	if v := envValue("ROUTATIC_PROXY_COMMANDCODE_URL"); v != "" {
		cfg.CommandCode.BaseURL = v
	}
	if v := envValue("ROUTATIC_PROXY_COMMANDCODE_ANTHROPIC_URL"); v != "" {
		cfg.CommandCode.AnthropicBaseURL = v
	}
	if v := envValue("ROUTATIC_PROXY_COMMANDCODE_RESPONSES_URL"); v != "" {
		cfg.CommandCode.ResponsesBaseURL = v
	}

	if v := envValue("ROUTATIC_PROXY_CLINE_PASS_URL"); v != "" {
		cfg.ClinePass.BaseURL = v
	}
	// CLINE_API_KEY is the name Cline's own docs and every third-party client
	// use for this credential, so it is accepted as the last resort rather than
	// making an operator rename a key they already exported.
	if cfg.ClinePass.APIKey == "" && len(cfg.ClinePass.APIKeys) == 0 {
		if v := os.Getenv("CLINE_API_KEY"); v != "" {
			cfg.ClinePass.APIKey = v
		}
	}

	if v := envValue("ROUTATIC_PROXY_HOST"); v != "" {
		cfg.Host = v
	}
	if v := envValue("ROUTATIC_PROXY_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.Port = port
		}
	}
	if v := envValue("ROUTATIC_PROXY_OPENCODE_URL"); v != "" {
		cfg.OpenCodeGo.BaseURL = v
	}
	if v := envValue("ROUTATIC_PROXY_OPENCODE_ZEN_URL"); v != "" {
		cfg.OpenCodeZen.BaseURL = v
	}
	if v := envValue("ROUTATIC_PROXY_LOG_LEVEL"); v != "" {
		cfg.Logging.Level = v
	}
	return nil
}

func envValue(name string) string {
	if val := os.Getenv(name); val != "" {
		return val
	}
	if legacyName, ok := legacyEnvNames[name]; ok {
		return os.Getenv(legacyName)
	}
	for canonicalName, legacyName := range legacyEnvNames {
		if name == legacyName {
			return os.Getenv(canonicalName)
		}
	}
	return ""
}

// applyDefaults fills in missing configuration values with sensible defaults.
func applyDefaults(cfg *Config) {
	if cfg.Host == "" {
		cfg.Host = defaultHost
	}
	if cfg.Port == 0 {
		cfg.Port = defaultPort
	}
	if cfg.AnthropicFirst.BaseURL == "" {
		cfg.AnthropicFirst.BaseURL = defaultAnthropicAPIURL
	}
	// Endpoint and timeout defaults, one row per field. Keeping this as a table
	// rather than a chain of per-platform blocks is what makes "add a platform"
	// a matter of adding rows instead of finding every branch that mentions one.
	applySiteDefaults(cfg)
	if strings.TrimSpace(cfg.ClinePass.ChannelPin) == "" {
		cfg.ClinePass.ChannelPin = DefaultClinePassChannel
	}
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = defaultLogLevel
	}
	if cfg.Fallbacks == nil {
		cfg.Fallbacks = make(map[string][]ModelConfig)
	}
	if cfg.ModelOverrides == nil {
		cfg.ModelOverrides = make(map[string]ModelConfig)
	}
	if cfg.ModelFamilyOverrides == nil {
		cfg.ModelFamilyOverrides = make(map[string]ModelConfig)
	}
	if cfg.Catalog.MaxAgeHours == 0 {
		cfg.Catalog.MaxAgeHours = defaultCatalogMaxAge
	}
	if cfg.Catalog.SourceURL == "" {
		cfg.Catalog.SourceURL = defaultCatalogSourceURL
	}
	// Capture is off unless enabled, but `{"enabled": true}` on its own used to
	// mean "rotate on every entry and never delete", and left prompts and
	// responses unredacted. The example config's values are the defaults now.
	if capture := cfg.Logging.DebugCapture; capture != nil {
		if capture.MaxFiles <= 0 {
			capture.MaxFiles = 100
		}
		if capture.MaxFileSize <= 0 {
			capture.MaxFileSize = 100 << 20
		}
		if !capture.RedactAPIKeys {
			capture.RedactAPIKeys = true
		}
	}
}

// validate checks that all required configuration fields are present.
func validate(cfg *Config) error {
	if cfg.Port < 0 || cfg.Port > 65535 {
		return invalid("port", "port_range", "port must be between 1 and 65535 (0 uses the default)")
	}
	if len(cfg.EffectiveAPIKeys()) == 0 && len(cfg.OpenCodeGo.EffectiveAPIKeys()) == 0 &&
		len(cfg.OpenCodeZen.EffectiveAPIKeys()) == 0 && len(cfg.AWSBedrock.EffectiveAPIKeys()) == 0 &&
		len(cfg.OpenRouter.EffectiveAPIKeys()) == 0 && len(cfg.CommandCode.EffectiveAPIKeys()) == 0 &&
		len(cfg.ClinePass.EffectiveAPIKeys()) == 0 && len(cfg.WorkBuddy.EffectiveAPIKeys()) == 0 {
		return invalid("", "missing_api_key", "api_key or api_keys is required (set via config file or ROUTATIC_PROXY_API_KEY env var; OC_GO_CC_API_KEY is still supported)")
	}
	// An unresolved placeholder or a malformed endpoint fails at request time,
	// where the error names neither the file nor the field: `base_url` used to
	// escape both checks entirely, so a misspelled variable name produced
	// "unsupported protocol scheme" on every call. `host` is exempt from the URL
	// check (it is a host or address, not a URL) but not from the placeholder one.
	for _, setting := range endpointSettings(cfg) {
		if setting.Value == "" {
			continue
		}
		if envVarPattern.MatchString(setting.Value) {
			return invalid(setting.Field, "unresolved_env",
				"%s contains an unresolved env var — set the corresponding environment variable or remove the placeholder", setting.Field)
		}
		if setting.Field == "host" {
			continue
		}
		u, err := url.Parse(setting.Value)
		if err != nil || u.Host == "" || u.User != nil || u.Fragment != "" ||
			(u.Scheme != "http" && u.Scheme != "https") {
			return invalid(setting.Field, "plain_url",
				"%s must be an absolute http or https URL without credentials or fragment", setting.Field)
		}
	}

	for _, raw := range cfg.TrustedProxies {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			return invalid("trusted_proxies", "empty_entry", "trusted_proxies entries must not be empty")
		}
		if net.ParseIP(entry) != nil {
			continue
		}
		if _, _, err := net.ParseCIDR(entry); err != nil {
			return invalid("trusted_proxies", "invalid_cidr", "trusted_proxies entry %q is not an IP address or CIDR", entry)
		}
	}

	if cfg.AnthropicFirst.Enabled {
		u, err := url.Parse(cfg.AnthropicFirst.BaseURL)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return invalid("anthropic_first.base_url", "absolute_url", "anthropic_first.base_url must be an absolute http or https URL")
		}
		// Two routing authorities with opposite intents: anthropic_first sends
		// every /v1/messages request straight to Anthropic, while active_site
		// promises the dashboard that every request goes through the selected
		// platform. Both cannot hold, and silently letting one win made the
		// selector lie about where traffic went.
		if cfg.ActiveSite != "" {
			return invalid("active_site", "routing_conflict", "active_site cannot be combined with anthropic_first.enabled: the first routes every request to one platform, the second sends it to Anthropic before any platform is considered (remove one of the two)")
		}
	}

	if err := validateAPIKeys(cfg.APIKeys); err != nil {
		return err
	}

	if err := validateSingleAPIKey(cfg.APIKey); err != nil {
		return err
	}

	// Validate provider-specific API keys
	if err := validateSingleAPIKey(cfg.OpenCodeGo.APIKey); err != nil {
		return AtField("opencode_go", err)
	}
	if err := validateAPIKeys(cfg.OpenCodeGo.APIKeys); err != nil {
		return AtField("opencode_go", err)
	}
	if err := validateKeyStrategy("opencode_go.key_strategy", cfg.OpenCodeGo.KeyStrategy); err != nil {
		return AtField("opencode_go", err)
	}

	if err := validateSingleAPIKey(cfg.OpenCodeZen.APIKey); err != nil {
		return AtField("opencode_zen", err)
	}
	if err := validateAPIKeys(cfg.OpenCodeZen.APIKeys); err != nil {
		return AtField("opencode_zen", err)
	}
	if err := validateKeyStrategy("opencode_zen.key_strategy", cfg.OpenCodeZen.KeyStrategy); err != nil {
		return AtField("opencode_zen", err)
	}

	if err := validateSingleAPIKey(cfg.AWSBedrock.APIKey); err != nil {
		return AtField("aws_bedrock", err)
	}
	if err := validateAPIKeys(cfg.AWSBedrock.APIKeys); err != nil {
		return AtField("aws_bedrock", err)
	}
	if err := validateKeyStrategy("aws_bedrock.key_strategy", cfg.AWSBedrock.KeyStrategy); err != nil {
		return AtField("aws_bedrock", err)
	}
	if err := cfg.AWSBedrock.Billing.Validate(); err != nil {
		return err
	}

	if err := validateSingleAPIKey(cfg.OpenRouter.APIKey); err != nil {
		return AtField("openrouter", err)
	}
	if err := validateAPIKeys(cfg.OpenRouter.APIKeys); err != nil {
		return AtField("openrouter", err)
	}
	if err := validateKeyStrategy("openrouter.key_strategy", cfg.OpenRouter.KeyStrategy); err != nil {
		return AtField("openrouter", err)
	}
	if err := validateSingleAPIKey(cfg.OpenRouter.ManagementAPIKey); err != nil {
		return invalid("openrouter.management_api_key", "unresolved_env", "openrouter.management_api_key: %v", err)
	}
	if err := validateSingleAPIKey(cfg.CommandCode.APIKey); err != nil {
		return AtField("commandcode", err)
	}
	if err := validateAPIKeys(cfg.CommandCode.APIKeys); err != nil {
		return AtField("commandcode", err)
	}
	if err := validateKeyStrategy("commandcode.key_strategy", cfg.CommandCode.KeyStrategy); err != nil {
		return AtField("commandcode", err)
	}
	for field, endpoint := range map[string]string{
		"base_url":           cfg.CommandCode.BaseURL,
		"anthropic_base_url": cfg.CommandCode.AnthropicBaseURL,
		"responses_base_url": cfg.CommandCode.ResponsesBaseURL,
	} {
		if endpoint == "" { // validate is also used on configs before defaults.
			continue
		}
		u, err := url.Parse(endpoint)
		if err != nil || u.Host == "" || u.User != nil || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") {
			return invalid("commandcode."+field, "plain_url", "commandcode.%s must be an absolute http or https URL without credentials or fragment", field)
		}
	}
	if err := validateTimeouts("commandcode", cfg.CommandCode.TimeoutMs, cfg.CommandCode.StreamTimeoutMs, cfg.CommandCode.StreamingTimeoutMs); err != nil {
		return err
	}

	if err := validateSingleAPIKey(cfg.ClinePass.APIKey); err != nil {
		return AtField("cline_pass", err)
	}
	if err := validateAPIKeys(cfg.ClinePass.APIKeys); err != nil {
		return AtField("cline_pass", err)
	}
	if err := validateKeyStrategy("cline_pass.key_strategy", cfg.ClinePass.KeyStrategy); err != nil {
		return AtField("cline_pass", err)
	}
	if endpoint := cfg.ClinePass.BaseURL; endpoint != "" { // validate is also used on configs before defaults.
		u, err := url.Parse(endpoint)
		if err != nil || u.Host == "" || u.User != nil || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") {
			return invalid("cline_pass.base_url", "plain_url", "cline_pass.base_url must be an absolute http or https URL without credentials or fragment")
		}
	}
	if err := validateTimeouts("cline_pass", cfg.ClinePass.TimeoutMs, cfg.ClinePass.StreamTimeoutMs, cfg.ClinePass.StreamingTimeoutMs); err != nil {
		return err
	}
	if cfg.ClinePass.ChannelPinEnabled && strings.TrimSpace(cfg.ClinePass.ChannelPin) == "" {
		return invalid("cline_pass.channel_pin", "channel_required", "cline_pass.channel_pin is required when channel_pin_enabled is true")
	}
	if pin := cfg.ClinePass.ChannelPin; pin != "" && !ValidChannelSlug(pin) {
		return invalid("cline_pass.channel_pin", "channel_slug", "cline_pass.channel_pin must be a lowercase provider slug (letters, digits and hyphens)")
	}
	// A capability the gateway refuses fails every request for that model, so
	// an unknown value must be caught here rather than on the live path.
	if err := ValidateChannelCapabilities(cfg.ClinePass.ChannelPinCapabilities); err != nil {
		return invalid("cline_pass.channel_pin_capabilities", "unknown_capability", "%s", err.Error())
	}

	if err := validateSingleAPIKey(cfg.WorkBuddy.APIKey); err != nil {
		return AtField("workbuddy", err)
	}
	if err := validateAPIKeys(cfg.WorkBuddy.APIKeys); err != nil {
		return AtField("workbuddy", err)
	}
	if err := validateKeyStrategy("workbuddy.key_strategy", cfg.WorkBuddy.KeyStrategy); err != nil {
		return AtField("workbuddy", err)
	}
	// The gateway is operator-run, so there is no public default to fall back
	// to: a key without an endpoint would only fail on the first request.
	if endpoint := cfg.WorkBuddy.BaseURL; endpoint != "" || len(cfg.WorkBuddy.EffectiveAPIKeys()) > 0 {
		if endpoint == "" {
			return invalid("workbuddy.base_url", "required", "workbuddy.base_url is required when a WorkBuddy key is configured")
		}
		u, err := url.Parse(endpoint)
		if err != nil || u.Host == "" || u.User != nil || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") {
			return invalid("workbuddy.base_url", "plain_url", "workbuddy.base_url must be an absolute http or https URL without credentials or fragment")
		}
	}
	if err := validateTimeouts("workbuddy", cfg.WorkBuddy.TimeoutMs, cfg.WorkBuddy.StreamTimeoutMs, cfg.WorkBuddy.StreamingTimeoutMs); err != nil {
		return err
	}

	if err := validateOverrideMap("models", cfg.Models); err != nil {
		return err
	}
	for scenario, fallbacks := range cfg.Fallbacks {
		for i, model := range fallbacks {
			if err := validateModelConfig(fmt.Sprintf("fallbacks[%q][%d]", scenario, i), model); err != nil {
				return err
			}
		}
	}
	if err := validateModelOverrides(cfg.ModelOverrides); err != nil {
		return err
	}

	if err := validateModelFamilyOverrides(cfg.ModelFamilyOverrides); err != nil {
		return err
	}

	if err := validateAnthropicToolsDisabled(cfg); err != nil {
		return err
	}

	if err := validateVisionModels(cfg); err != nil {
		return err
	}

	if err := validateCostScenarios(cfg.CostRouting); err != nil {
		return err
	}

	// An unknown site here would filter every routing target away and read as a
	// routing failure later, so it is caught where the typo was made.
	if cfg.ActiveSite != "" {
		if !SupportedProvider(cfg.ActiveSite) {
			return invalid("active_site", "unknown_provider", "active_site %q is not a known platform", cfg.ActiveSite)
		}
		cfg.ActiveSite = NormalizeProvider(cfg.ActiveSite)
	}

	return nil
}

var channelSlugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// ValidChannelSlug reports whether a value is a single gateway channel name.
func ValidChannelSlug(value string) bool {
	return channelSlugPattern.MatchString(value)
}

// knownChannelCapabilities is the set of `has` values the gateway accepted when
// measured on 2026-10-23, per model:
//
//	deepseek-v4.1-flash-1017 : vision, implicit-caching, reasoning, structured-output
//	deepseek-v4-pro          : implicit-caching, reasoning, structured-output (NOT vision)
//
// The set is hard-coded rather than discovered at load time because a request
// carrying an unknown value is refused with stream_initialization_failed - the
// failure happens on the live path, so validating here is the only way to keep
// a typo from turning into an outage. Values confirmed to be rejected by both
// models (tools, function-calling, streaming, json-mode, image-input,
// prompt-caching) are deliberately absent: naming one would fail every request.
var knownChannelCapabilities = map[string]bool{
	"vision":            true,
	"implicit-caching":  true,
	"reasoning":         true,
	"structured-output": true,
}

// ValidateChannelCapabilities rejects a capability filter the gateway would
// refuse, before the value can reach a live request.
func ValidateChannelCapabilities(byModel map[string][]string) error {
	for model, capabilities := range byModel {
		for _, capability := range capabilities {
			if !knownChannelCapabilities[capability] {
				return fmt.Errorf(
					"cline_pass.channel_pin_capabilities[%q]: unknown capability %q "+
						"(accepted: vision, implicit-caching, reasoning, structured-output)",
					model, capability)
			}
		}
	}
	return nil
}

func validateTimeouts(provider string, total, idle, streaming int) error {
	for _, value := range []struct {
		field string
		ms    int
	}{{"timeout_ms", total}, {"stream_timeout_ms", idle}, {"streaming_timeout_ms", streaming}} {
		if value.ms < 0 {
			return invalid(provider+"."+value.field, "non_negative", "%s timeouts must not be negative", provider)
		}
	}
	return nil
}

// validateCostScenarios rejects cost_routing.scenarios keys that are not real
// routing scenarios. A typo would otherwise sit in the config doing nothing,
// which is the failure mode this validation exists to prevent.
func validateCostScenarios(cr *CostRoutingConfig) error {
	if cr == nil {
		return nil
	}
	for name := range cr.Scenarios {
		if !slices.Contains(CostScenarioNames, name) {
			return invalid("cost_routing.scenarios", "unknown_scenario", "cost_routing.scenarios has unknown scenario %q (valid: %s)",
				name, strings.Join(CostScenarioNames, ", "))
		}
	}
	return nil
}

// validateVisionModels checks that when a vision scenario is configured,
// the primary model supports vision. Vision scenarios are optional —
// only validate them when they appear in the models map.
func validateVisionModels(cfg *Config) error {
	for _, scenario := range []string{"vision", "vision_complex", "vision_long_context"} {
		model, ok := cfg.Models[scenario]
		if !ok {
			continue
		}
		// Only a model known to lack vision is a configuration error here. A
		// model whose capability is unstated may well accept images; refusing to
		// start would punish the operator for the catalog's silence.
		if vision, known := ModelHasVision(ResolveModelConfig(model)); known && !vision {
			return invalid(fmt.Sprintf("models[%q]", scenario), "vision_required", "models[%q] does not support vision but is configured for vision scenario", scenario)
		}
	}
	return nil
}

// validateAnthropicToolsDisabled checks that models with anthropic_tools_disabled
// set are configured correctly. This field only applies to models that route to
// the Anthropic endpoint; enabling it on an OpenAI Chat Completions model has no
// effect and likely indicates a misconfiguration.
func validateAnthropicToolsDisabled(cfg *Config) error {
	for key, mc := range cfg.Models {
		if mc.AnthropicToolsDisabled {
			// Models in cfg.Models are selectable by scenario routing. The flag
			// is only meaningful on models that go through the Anthropic endpoint.
			// Log a warning since the config system can't resolve the endpoint
			// without the client package.
			fmt.Fprintf(os.Stderr, "WARNING: config: models[%q] has anthropic_tools_disabled=true — this is only effective on models routing to the Anthropic endpoint\n", key)
		}
	}
	for key, mc := range cfg.ModelOverrides {
		if mc.AnthropicToolsDisabled {
			fmt.Fprintf(os.Stderr, "WARNING: config: model_overrides[%q] has anthropic_tools_disabled=true — this is only effective on models routing to the Anthropic endpoint\n", key)
		}
	}
	for key, mc := range cfg.ModelFamilyOverrides {
		if mc.AnthropicToolsDisabled {
			fmt.Fprintf(os.Stderr, "WARNING: config: model_family_overrides[%q] has anthropic_tools_disabled=true — this is only effective on models routing to the Anthropic endpoint\n", key)
		}
	}
	return nil
}

// validateAPIKeys ensures no api_keys entries contain unresolved ${VAR} placeholders.
// Unresolved placeholders indicate the user did not set the corresponding env vars,
// and the literal placeholder string would be sent as a bearer token.
func validateSingleAPIKey(key string) error {
	if key == "" {
		return nil
	}
	if envVarPattern.MatchString(key) {
		return invalid("api_key", "unresolved_env", "api_key contains unresolved env var — set the corresponding environment variable, or remove the api_key field if this platform uses its own api_keys")
	}
	return nil
}

// validateKeyStrategy rejects anything the scheduler does not implement, so a
// typo cannot silently fall back to a different rule than the operator asked
// for.
func validateKeyStrategy(field, value string) error {
	switch v := strings.TrimSpace(value); v {
	case "", KeyStrategyFillFirst:
		return nil
	default:
		return fmt.Errorf("%s must be %q (the only implemented strategy), got %q", field, KeyStrategyFillFirst, v)
	}
}

func validateAPIKeys(keys []string) error {
	for i, key := range keys {
		if key == "" {
			return invalid(fmt.Sprintf("api_keys[%d]", i), "empty_key", "api_keys[%d] is empty — each key must be a non-empty string", i)
		}
		if envVarPattern.MatchString(key) {
			return invalid(fmt.Sprintf("api_keys[%d]", i), "unresolved_env", "api_keys[%d] contains unresolved env var — set the corresponding environment variable or remove this entry", i)
		}
	}
	return nil
}

// validateModelOverrides ensures each override entry has a non-empty model_id
// and a recognized provider. Empty model_id would produce broken upstream URLs
// (surfacing far from the config error); an unknown provider would silently
// fall through to defaults at request time.
func validateModelOverrides(overrides map[string]ModelConfig) error {
	return validateOverrideMap("model_overrides", overrides)
}

// validateModelFamilyOverrides ensures each family override entry has a
// non-empty family key, a non-empty model_id, and a recognized provider.
func validateModelFamilyOverrides(overrides map[string]ModelConfig) error {
	for key := range overrides {
		if strings.TrimSpace(key) == "" {
			return invalid("model_family_overrides", "empty_family", "model_family_overrides has an empty family key")
		}
	}
	return validateOverrideMap("model_family_overrides", overrides)
}

// validateOverrideMap validates the shared shape of override maps: each entry
// must have a non-empty model_id and a recognized provider. label names the
// config section for error messages.
func validateOverrideMap(label string, overrides map[string]ModelConfig) error {
	for key, mc := range overrides {
		if err := validateModelConfig(fmt.Sprintf("%s[%q]", label, key), mc); err != nil {
			return err
		}
	}
	return nil
}

func validateModelConfig(label string, mc ModelConfig) error {
	if strings.TrimSpace(mc.ModelID) == "" {
		return invalid(label+".model_id", "model_required", "%s is missing required field model_id", label)
	}
	if !SupportedProvider(mc.Provider) {
		return invalid(label+".provider", "unknown_provider", "%s has invalid provider %q", label, mc.Provider)
	}
	switch mc.WireFormat {
	case "", "auto", "openai", "anthropic", "responses", "gemini":
	default:
		return invalid(label+".wire_format", "wire_format", "%s has invalid wire_format %q", label, mc.WireFormat)
	}
	if mc.WireFormat == "gemini" && NormalizeProvider(mc.Provider) != "opencode-zen" {
		return invalid(label+".wire_format", "gemini_unsupported", "%s: only opencode-zen implements the gemini upstream wire format", label)
	}
	if NormalizeProvider(mc.Provider) == "commandcode" && mc.WireFormat != "" && mc.WireFormat != "auto" && mc.WireFormat != "openai" && mc.WireFormat != "anthropic" && mc.WireFormat != "responses" {
		return invalid(label+".wire_format", "commandcode_wire_format", "%s: commandcode supports only openai, anthropic or responses upstream wire formats", label)
	}
	// ClinePass publishes no Messages endpoint, so anthropic is not a choice
	// here: accepting it would send a model to an endpoint that does not exist
	// and report the gateway's 401 as an auth problem.
	if NormalizeProvider(mc.Provider) == "cline-pass" && mc.WireFormat != "" && mc.WireFormat != "auto" && mc.WireFormat != "openai" {
		return invalid(label+".wire_format", "cline_wire_format", "%s: cline-pass supports only the openai upstream wire format", label)
	}
	// WorkBuddy's provider always speaks Chat Completions. An explicit
	// anthropic/responses/gemini value is not a request the gateway can honour;
	// it only desynchronizes the handler's response path from the bytes the
	// gateway actually sends.
	if NormalizeProvider(mc.Provider) == "workbuddy" && mc.WireFormat != "" && mc.WireFormat != "auto" && mc.WireFormat != "openai" {
		return invalid(label+".wire_format", "workbuddy_wire_format", "%s: workbuddy supports only the openai upstream wire format", label)
	}
	// ClinePass takes its billing pool from the model id's first segment and
	// answers a bare name with 400 "invalid model format. Expected format:
	// modelType/model". The field is sent verbatim, so a config entry without
	// the prefix is only discovered on the failure path that uses it - a
	// fallback, i.e. after the primary already failed - where the 400 hides the
	// real cause. Catch it at load time instead.
	if NormalizeProvider(mc.Provider) == "cline-pass" && !hasModelTypePrefix(mc.ModelID) {
		return invalid(label+".model_id", "cline_model_id", "%s: cline-pass model_id %q must be written as \"modelType/model\" (for example \"cline-pass/deepseek-v4.1-flash\"); a bare name is rejected upstream with HTTP 400", label, mc.ModelID)
	}
	return nil
}

// hasModelTypePrefix reports whether id has the "modelType/model" shape the
// ClinePass gateway requires. Both segments must be non-empty: the gateway
// rejects "/model" and "model/" for the same reason it rejects a bare name.
func hasModelTypePrefix(id string) bool {
	modelType, model, ok := strings.Cut(id, "/")
	return ok && modelType != "" && model != ""
}

// applySiteDefaults fills every unset endpoint and timeout from the value its
// API publishes.
//
// A row per field, so adding a platform means adding rows rather than hunting
// for each branch that mentions one. The order inside a row is the fallback
// order: an unset idle or total timeout takes the platform's overall timeout,
// and the overall timeout takes the shared default.
func applySiteDefaults(cfg *Config) {
	for _, row := range []struct {
		target *string
		value  string
	}{
		{&cfg.OpenCodeGo.BaseURL, defaultBaseURL},
		{&cfg.OpenCodeGo.AnthropicBaseURL, defaultAnthropicBaseURL},
		{&cfg.OpenCodeGo.ResponsesBaseURL, defaultResponsesBaseURL},
		{&cfg.OpenCodeZen.BaseURL, defaultZenBaseURL},
		{&cfg.OpenCodeZen.AnthropicBaseURL, defaultZenAnthropicBaseURL},
		{&cfg.OpenCodeZen.ResponsesBaseURL, defaultZenResponsesBaseURL},
		{&cfg.OpenCodeZen.GeminiBaseURL, defaultZenGeminiBaseURL},
		{&cfg.OpenRouter.BaseURL, defaultOpenRouterBaseURL},
		{&cfg.CommandCode.BaseURL, defaultCommandCodeBaseURL},
		{&cfg.CommandCode.AnthropicBaseURL, defaultCommandCodeAnthropicBaseURL},
		{&cfg.CommandCode.ResponsesBaseURL, defaultCommandCodeResponsesBaseURL},
		{&cfg.ClinePass.BaseURL, defaultClinePassBaseURL},
	} {
		if *row.target == "" {
			*row.target = row.value
		}
	}

	// Timeouts share one default, so each row only names the duration it falls
	// back to within its own platform.
	for _, row := range []struct {
		timeout, idle, streaming *int
	}{
		{&cfg.OpenCodeGo.TimeoutMs, &cfg.OpenCodeGo.StreamTimeoutMs, &cfg.OpenCodeGo.StreamingTimeoutMs},
		{&cfg.OpenCodeZen.TimeoutMs, &cfg.OpenCodeZen.StreamTimeoutMs, &cfg.OpenCodeZen.StreamingTimeoutMs},
		{&cfg.AWSBedrock.TimeoutMs, &cfg.AWSBedrock.StreamTimeoutMs, &cfg.AWSBedrock.StreamingTimeoutMs},
		{&cfg.OpenRouter.TimeoutMs, &cfg.OpenRouter.StreamTimeoutMs, &cfg.OpenRouter.StreamingTimeoutMs},
		{&cfg.CommandCode.TimeoutMs, &cfg.CommandCode.StreamTimeoutMs, &cfg.CommandCode.StreamingTimeoutMs},
		{&cfg.ClinePass.TimeoutMs, &cfg.ClinePass.StreamTimeoutMs, &cfg.ClinePass.StreamingTimeoutMs},
		{&cfg.WorkBuddy.TimeoutMs, &cfg.WorkBuddy.StreamTimeoutMs, &cfg.WorkBuddy.StreamingTimeoutMs},
	} {
		if *row.timeout == 0 {
			*row.timeout = defaultTimeoutMs
		}
		if *row.idle == 0 {
			if *row.streaming > 0 {
				*row.idle = *row.streaming
			} else {
				*row.idle = *row.timeout
			}
		}
		if *row.streaming == 0 {
			*row.streaming = *row.timeout
		}
	}
}

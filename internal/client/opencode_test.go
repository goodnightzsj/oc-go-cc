package client

import (
	"testing"
	"time"

	"github.com/routatic/proxy/internal/config"
)

func TestProvider(t *testing.T) {
	tests := []struct {
		name     string
		model    config.ModelConfig
		expected string
	}{
		{
			name:     "empty provider defaults to opencode-go",
			model:    config.ModelConfig{ModelID: "test-model"},
			expected: ProviderOpenCodeGo,
		},
		{
			name:     "explicit opencode-go provider",
			model:    config.ModelConfig{Provider: ProviderOpenCodeGo, ModelID: "test-model"},
			expected: ProviderOpenCodeGo,
		},
		{
			name:     "explicit opencode-zen provider",
			model:    config.ModelConfig{Provider: ProviderOpenCodeZen, ModelID: "test-model"},
			expected: ProviderOpenCodeZen,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Provider(tt.model); got != tt.expected {
				t.Fatalf("Provider() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestIsZen(t *testing.T) {
	tests := []struct {
		name     string
		model    config.ModelConfig
		expected bool
	}{
		{
			name:     "opencode-go is not zen",
			model:    config.ModelConfig{Provider: ProviderOpenCodeGo},
			expected: false,
		},
		{
			name:     "opencode-zen is zen",
			model:    config.ModelConfig{Provider: ProviderOpenCodeZen},
			expected: true,
		},
		{
			name:     "empty provider is not zen",
			model:    config.ModelConfig{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsZen(tt.model); got != tt.expected {
				t.Fatalf("IsZen() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestStreamIdleTimeout(t *testing.T) {
	tests := []struct {
		name     string
		goMs     int
		zenMs    int
		provider string
		wantDur  time.Duration
	}{
		{
			name:     "Go provider uses OpenCodeGo.StreamTimeoutMs",
			goMs:     120000, // 2 min
			provider: "opencode-go",
			wantDur:  120 * time.Second,
		},
		{
			name:     "Zen provider uses OpenCodeZen.StreamTimeoutMs",
			goMs:     100000,
			zenMs:    600000, // 10 min
			provider: "opencode-zen",
			wantDur:  10 * time.Minute,
		},
		{
			name:     "falls back to OpenCodeGo.TimeoutMs when StreamTimeoutMs is zero",
			goMs:     300000, // 5 min
			provider: "opencode-go",
			wantDur:  5 * time.Minute,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				OpenCodeGo:  config.OpenCodeGoConfig{TimeoutMs: tt.goMs, StreamTimeoutMs: tt.goMs},
				OpenCodeZen: config.OpenCodeZenConfig{TimeoutMs: tt.zenMs, StreamTimeoutMs: tt.zenMs},
			}
			// Fallback test: zero out StreamTimeoutMs for that provider.
			if tt.name == "falls back to OpenCodeGo.TimeoutMs when StreamTimeoutMs is zero" {
				cfg.OpenCodeGo.StreamTimeoutMs = 0
			}
			atomic := config.NewAtomicConfig(cfg, "/tmp/test-config.json")
			c := &OpenCodeClient{atomic: atomic}
			mc := config.ModelConfig{Provider: tt.provider, ModelID: "test-model"}
			got := c.StreamIdleTimeout(mc)
			if got != tt.wantDur {
				t.Errorf("StreamIdleTimeout() = %v, want %v", got, tt.wantDur)
			}
		})
	}
}

func TestRequestTimeout_UsesConfiguredTimeout(t *testing.T) {
	cfg := &config.Config{
		OpenCodeGo: config.OpenCodeGoConfig{
			TimeoutMs: 120000,
		},
	}
	atomicCfg := config.NewAtomicConfig(cfg, "")
	c := NewOpenCodeClient(atomicCfg)

	model := config.ModelConfig{Provider: ProviderOpenCodeGo, ModelID: "kimi-k2.6"}
	timeout := c.RequestTimeout(model)
	if timeout != 120*time.Second {
		t.Errorf("RequestTimeout = %v, want 120s", timeout)
	}
}

func TestRequestTimeout_FallsBackToDefault(t *testing.T) {
	cfg := &config.Config{
		OpenCodeGo: config.OpenCodeGoConfig{
			TimeoutMs: 0,
		},
	}
	atomicCfg := config.NewAtomicConfig(cfg, "")
	c := NewOpenCodeClient(atomicCfg)

	model := config.ModelConfig{Provider: ProviderOpenCodeGo, ModelID: "kimi-k2.6"}
	timeout := c.RequestTimeout(model)
	if timeout != 5*time.Minute {
		t.Errorf("RequestTimeout = %v, want 5m", timeout)
	}
}

func TestRequestTimeout_ZenProvider(t *testing.T) {
	cfg := &config.Config{
		OpenCodeZen: config.OpenCodeZenConfig{
			TimeoutMs: 60000,
		},
	}
	atomicCfg := config.NewAtomicConfig(cfg, "")
	c := NewOpenCodeClient(atomicCfg)

	model := config.ModelConfig{Provider: ProviderOpenCodeZen, ModelID: "claude-sonnet-4.5"}
	timeout := c.RequestTimeout(model)
	if timeout != 60*time.Second {
		t.Errorf("RequestTimeout = %v, want 60s", timeout)
	}
}

func TestStreamingTimeout_UsesStreamingTimeoutMs(t *testing.T) {
	cfg := &config.Config{
		OpenCodeGo: config.OpenCodeGoConfig{
			TimeoutMs:          300000,
			StreamingTimeoutMs: 600000,
		},
	}
	atomicCfg := config.NewAtomicConfig(cfg, "")
	c := NewOpenCodeClient(atomicCfg)

	model := config.ModelConfig{Provider: ProviderOpenCodeGo, ModelID: "kimi-k2.6"}
	timeout := c.StreamingTimeout(model)
	if timeout != 600*time.Second {
		t.Errorf("StreamingTimeout = %v, want 600s", timeout)
	}
}

func TestStreamingTimeout_FallsBackToTimeoutMs(t *testing.T) {
	cfg := &config.Config{
		OpenCodeGo: config.OpenCodeGoConfig{
			TimeoutMs:          300000,
			StreamingTimeoutMs: 0,
		},
	}
	atomicCfg := config.NewAtomicConfig(cfg, "")
	c := NewOpenCodeClient(atomicCfg)

	model := config.ModelConfig{Provider: ProviderOpenCodeGo, ModelID: "kimi-k2.6"}
	timeout := c.StreamingTimeout(model)
	if timeout != 300*time.Second {
		t.Errorf("StreamingTimeout = %v, want 300s (fallback to timeout_ms)", timeout)
	}
}

func TestStreamingTimeout_FallsBackToDefault(t *testing.T) {
	cfg := &config.Config{
		OpenCodeGo: config.OpenCodeGoConfig{
			TimeoutMs:          0,
			StreamingTimeoutMs: 0,
		},
	}
	atomicCfg := config.NewAtomicConfig(cfg, "")
	c := NewOpenCodeClient(atomicCfg)

	model := config.ModelConfig{Provider: ProviderOpenCodeGo, ModelID: "kimi-k2.6"}
	timeout := c.StreamingTimeout(model)
	if timeout != 5*time.Minute {
		t.Errorf("StreamingTimeout = %v, want 5m", timeout)
	}
}

func TestStreamingTimeout_ZenProvider(t *testing.T) {
	cfg := &config.Config{
		OpenCodeZen: config.OpenCodeZenConfig{
			TimeoutMs:          300000,
			StreamingTimeoutMs: 600000,
		},
	}
	atomicCfg := config.NewAtomicConfig(cfg, "")
	c := NewOpenCodeClient(atomicCfg)

	model := config.ModelConfig{Provider: ProviderOpenCodeZen, ModelID: "claude-sonnet-4.5"}
	timeout := c.StreamingTimeout(model)
	if timeout != 600*time.Second {
		t.Errorf("StreamingTimeout = %v, want 600s", timeout)
	}
}

func TestStreamingTimeout_SmallConfiguredValue(t *testing.T) {
	cfg := &config.Config{
		OpenCodeGo: config.OpenCodeGoConfig{
			TimeoutMs:          300000,
			StreamingTimeoutMs: 100,
		},
	}
	atomicCfg := config.NewAtomicConfig(cfg, "")
	c := NewOpenCodeClient(atomicCfg)

	model := config.ModelConfig{Provider: ProviderOpenCodeGo, ModelID: "kimi-k2.6"}
	timeout := c.StreamingTimeout(model)
	if timeout != 100*time.Millisecond {
		t.Errorf("StreamingTimeout = %v, want 100ms", timeout)
	}
}

func TestGetProviderAPIKeys_ProviderSpecificKeys(t *testing.T) {
	cfg := &config.Config{
		OpenCodeGo: config.OpenCodeGoConfig{
			APIKeys: []string{"go-key-1", "go-key-2"},
		},
		OpenCodeZen: config.OpenCodeZenConfig{
			APIKey: "zen-specific-key",
		},
		AWSBedrock: config.AWSBedrockConfig{
			APIKeys: []string{"bedrock-key-1", "bedrock-key-2"},
		},
	}

	tests := []struct {
		name     string
		provider string
		want     []string
	}{
		{
			name:     "OpenCode Go uses its own keys",
			provider: ProviderOpenCodeGo,
			want:     []string{"go-key-1", "go-key-2"},
		},
		{
			name:     "OpenCode Zen uses its own key",
			provider: ProviderOpenCodeZen,
			want:     []string{"zen-specific-key"},
		},
		{
			name:     "AWS Bedrock uses its own keys",
			provider: ProviderAWSBedrock,
			want:     []string{"bedrock-key-1", "bedrock-key-2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := config.ModelConfig{Provider: tt.provider, ModelID: "test-model"}
			got := cfg.ProviderAPIKeys(Provider(model))
			if len(got) != len(tt.want) {
				t.Errorf("getProviderAPIKeys() = %v, want %v", got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("getProviderAPIKeys()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestGetProviderAPIKeys_FallbackToGlobal(t *testing.T) {
	cfg := &config.Config{
		APIKeys:    []string{"global-key-1", "global-key-2"},
		OpenCodeGo: config.OpenCodeGoConfig{
			// No provider-specific keys
		},
	}

	model := config.ModelConfig{Provider: ProviderOpenCodeGo, ModelID: "test-model"}
	got := cfg.ProviderAPIKeys(Provider(model))

	want := []string{"global-key-1", "global-key-2"}
	if len(got) != len(want) {
		t.Errorf("getProviderAPIKeys() = %v, want %v (fallback to global)", got, want)
		return
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("getProviderAPIKeys()[%d] = %q, want %q (fallback to global)", i, got[i], want[i])
		}
	}
}

func TestGetProviderAPIKeys_ProviderKeysPrecedence(t *testing.T) {
	cfg := &config.Config{
		APIKeys: []string{"global-key"},
		OpenCodeGo: config.OpenCodeGoConfig{
			APIKeys: []string{"go-specific-key"},
		},
	}

	model := config.ModelConfig{Provider: ProviderOpenCodeGo, ModelID: "test-model"}
	got := cfg.ProviderAPIKeys(Provider(model))

	// Should use provider-specific keys, not global
	want := []string{"go-specific-key"}
	if len(got) != len(want) || got[0] != want[0] {
		t.Errorf("getProviderAPIKeys() = %v, want %v (provider keys should take precedence)", got, want)
	}
}

func TestOpenRouterKeys(t *testing.T) {
	cfg := &config.Config{
		OpenRouter: config.OpenRouterConfig{
			APIKey: "openrouter-specific-key",
		},
	}

	model := config.ModelConfig{Provider: ProviderOpenRouter, ModelID: "openrouter-model"}
	got := cfg.ProviderAPIKeys(Provider(model))

	want := []string{"openrouter-specific-key"}
	if len(got) != len(want) || got[0] != want[0] {
		t.Errorf("getProviderAPIKeys() = %v, want %v", got, want)
	}
}

func TestOpenRouterTimeout(t *testing.T) {
	cfg := &config.Config{
		OpenRouter: config.OpenRouterConfig{
			TimeoutMs:          120000,
			StreamTimeoutMs:    180000,
			StreamingTimeoutMs: 240000,
		},
	}
	atomicCfg := config.NewAtomicConfig(cfg, "")
	c := NewOpenCodeClient(atomicCfg)

	model := config.ModelConfig{Provider: ProviderOpenRouter, ModelID: "openrouter-model"}

	if got := c.RequestTimeout(model); got != 120*time.Second {
		t.Errorf("RequestTimeout = %v, want 120s", got)
	}
	if got := c.StreamIdleTimeout(model); got != 180*time.Second {
		t.Errorf("StreamIdleTimeout = %v, want 180s", got)
	}
	if got := c.StreamingTimeout(model); got != 240*time.Second {
		t.Errorf("StreamingTimeout = %v, want 240s", got)
	}
}

func TestOpenRouterTimeout_FallsBackToTimeoutMs(t *testing.T) {
	cfg := &config.Config{
		OpenRouter: config.OpenRouterConfig{
			TimeoutMs:          120000,
			StreamTimeoutMs:    0,
			StreamingTimeoutMs: 0,
		},
	}
	atomicCfg := config.NewAtomicConfig(cfg, "")
	c := NewOpenCodeClient(atomicCfg)

	model := config.ModelConfig{Provider: ProviderOpenRouter, ModelID: "openrouter-model"}

	if got := c.StreamIdleTimeout(model); got != 120*time.Second {
		t.Errorf("StreamIdleTimeout = %v, want 120s", got)
	}
	if got := c.StreamingTimeout(model); got != 120*time.Second {
		t.Errorf("StreamingTimeout = %v, want 120s", got)
	}
}

func TestGetProviderAPIKeys_EmptyReturnsGlobal(t *testing.T) {
	cfg := &config.Config{
		APIKey:     "global-single-key",
		OpenCodeGo: config.OpenCodeGoConfig{
			// No keys configured
		},
	}

	model := config.ModelConfig{Provider: ProviderOpenCodeGo, ModelID: "test-model"}
	got := cfg.ProviderAPIKeys(Provider(model))

	want := []string{"global-single-key"}
	if len(got) != len(want) || got[0] != want[0] {
		t.Errorf("getProviderAPIKeys() = %v, want %v (should fallback to global)", got, want)
	}
}

package config

import "testing"

func TestResolveModelConfig_KnownModelGetsCapacity(t *testing.T) {
	m := ResolveModelConfig(ModelConfig{Provider: "opencode-go", ModelID: "kimi-k2.6"})
	if m.ContextWindow != 256000 {
		t.Errorf("kimi-k2.6 ContextWindow = %d, want 256000", m.ContextWindow)
	}
	if m.MaxOutputTokens != 8192 {
		t.Errorf("kimi-k2.6 MaxOutputTokens = %d, want 8192", m.MaxOutputTokens)
	}
	if !m.Vision {
		t.Error("kimi-k2.6 should resolve to Vision=true")
	}
	if m.ContextMargin != DefaultContextMargin {
		t.Errorf("ContextMargin = %d, want default %d", m.ContextMargin, DefaultContextMargin)
	}
}

func TestResolveModelConfig_PreservesExplicitValues(t *testing.T) {
	m := ResolveModelConfig(ModelConfig{
		Provider:     "opencode-go",
		ModelID:      "kimi-k2.6",
		ContextWindow: 500000,
		MaxTokens:     4096,
	})
	// Explicit config value must win over the registry default.
	if m.ContextWindow != 500000 {
		t.Errorf("explicit ContextWindow = %d, want 500000 (must not be overwritten)", m.ContextWindow)
	}
	// MaxOutputTokens was zero, so it still gets filled from the registry.
	if m.MaxOutputTokens != 8192 {
		t.Errorf("MaxOutputTokens = %d, want 8192 (filled from registry)", m.MaxOutputTokens)
	}
}

func TestResolveModelConfig_NewModelsRegistered(t *testing.T) {
	// Models introduced by the upstream model-list increments must resolve to
	// realistic capacities so capacity filtering can compose with them.
	cases := map[string]struct {
		contextWindow   int
		maxOutputTokens int
		vision          bool
	}{
		"glm-5.2":        {200000, 8192, false},
		"kimi-k2.7-code": {256000, 32768, true},
		"kimi-k3":        {1000000, 131072, true},
		"mimo-v2-omni":   {1000000, 8192, true},
	}
	for id, want := range cases {
		m := ResolveModelConfig(ModelConfig{Provider: "opencode-go", ModelID: id})
		if m.ContextWindow != want.contextWindow {
			t.Errorf("%s ContextWindow = %d, want %d", id, m.ContextWindow, want.contextWindow)
		}
		if m.MaxOutputTokens != want.maxOutputTokens {
			t.Errorf("%s MaxOutputTokens = %d, want %d", id, m.MaxOutputTokens, want.maxOutputTokens)
		}
		if m.Vision != want.vision {
			t.Errorf("%s Vision = %v, want %v", id, m.Vision, want.vision)
		}
	}
}

func TestResolveModelConfig_UnknownModelUnconstrained(t *testing.T) {
	m := ResolveModelConfig(ModelConfig{Provider: "opencode-go", ModelID: "some-future-model"})
	if m.ContextWindow != 0 {
		t.Errorf("unknown model ContextWindow = %d, want 0 (unconstrained)", m.ContextWindow)
	}
	// ContextMargin is still defaulted.
	if m.ContextMargin != DefaultContextMargin {
		t.Errorf("unknown model ContextMargin = %d, want default", m.ContextMargin)
	}
}

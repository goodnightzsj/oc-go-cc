package config

import (
	"testing"
)

func boolPtr(b bool) *bool {
	return &b
}

func TestResolveModelConfig(t *testing.T) {
	tests := []struct {
		name     string
		input    ModelConfig
		expected ModelConfig
	}{
		{
			name: "legacy model with empty ModelRef gets hardcoded metadata",
			input: ModelConfig{
				ModelID: "kimi-k2.6",
			},
			expected: ModelConfig{
				ModelID:         "kimi-k2.6",
				ContextWindow:   256000,
				MaxOutputTokens: 8192,
				Vision:          BoolPtr(true),
				ContextMargin:   DefaultContextMargin,
				SupportsTools:   boolPtr(true),
			},
		},
		{
			name:  "known mixed-case model uses canonical ID and metadata",
			input: ModelConfig{ModelID: "DeepSeek-V4-Pro"},
			expected: ModelConfig{
				ModelID:         "deepseek-v4-pro",
				ContextWindow:   1000000,
				MaxOutputTokens: 8192,
				ContextMargin:   DefaultContextMargin,
				SupportsTools:   boolPtr(true),
			},
		},
		{
			name:  "vendor-prefixed model uses family metadata without changing wire ID",
			input: ModelConfig{Provider: "commandcode", ModelID: "moonshotai/Kimi-K2.7-Code"},
			expected: ModelConfig{
				ModelID:         "moonshotai/Kimi-K2.7-Code",
				ContextWindow:   256000,
				MaxOutputTokens: 32768,
				Vision:          BoolPtr(true),
				ContextMargin:   DefaultContextMargin,
				SupportsTools:   boolPtr(true),
			},
		},
		{
			name:  "unknown custom model preserves case",
			input: ModelConfig{ModelID: "Vendor-Custom-Pro"},
			expected: ModelConfig{
				ModelID:       "Vendor-Custom-Pro",
				ContextMargin: DefaultContextMargin,
				SupportsTools: boolPtr(true),
			},
		},
		{
			name: "kimi-k3 gets hardcoded metadata (1M context, 131K output, vision)",
			input: ModelConfig{
				ModelID: "kimi-k3",
			},
			expected: ModelConfig{
				ModelID:         "kimi-k3",
				ContextWindow:   1000000,
				MaxOutputTokens: 131072,
				Vision:          BoolPtr(true),
				ContextMargin:   DefaultContextMargin,
				SupportsTools:   boolPtr(true),
			},
		},
		{
			name: "ModelRef present preserves explicit catalog capabilities",
			input: ModelConfig{
				ModelID:       "deepseek-v4-flash",
				ModelRef:      "deepseek/deepseek-v4-flash@opencode-go",
				ContextWindow: 12345,
				Vision:        BoolPtr(true),
				SupportsTools: boolPtr(true),
			},
			expected: ModelConfig{
				ModelID:       "deepseek-v4-flash",
				ModelRef:      "deepseek/deepseek-v4-flash@opencode-go",
				ContextWindow: 12345,
				Vision:        BoolPtr(true),
				ContextMargin: DefaultContextMargin,
				SupportsTools: boolPtr(true),
			},
		},
		{
			name: "ModelRef present with zero values still gets defaults",
			input: ModelConfig{
				ModelID:  "deepseek-v4-flash",
				ModelRef: "deepseek/deepseek-v4-flash@opencode-go",
			},
			expected: ModelConfig{
				ModelID:       "deepseek-v4-flash",
				ModelRef:      "deepseek/deepseek-v4-flash@opencode-go",
				ContextMargin: DefaultContextMargin,
				SupportsTools: boolPtr(true),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveModelConfig(tt.input)

			if got.ModelID != tt.expected.ModelID {
				t.Errorf("ModelID = %q, want %q", got.ModelID, tt.expected.ModelID)
			}
			if got.ModelRef != tt.expected.ModelRef {
				t.Errorf("ModelRef = %q, want %q", got.ModelRef, tt.expected.ModelRef)
			}
			if got.ContextWindow != tt.expected.ContextWindow {
				t.Errorf("ContextWindow = %d, want %d", got.ContextWindow, tt.expected.ContextWindow)
			}
			if got.MaxOutputTokens != tt.expected.MaxOutputTokens {
				t.Errorf("MaxOutputTokens = %d, want %d", got.MaxOutputTokens, tt.expected.MaxOutputTokens)
			}
			// Vision is tri-state: nil is "no source said", which is a different
			// answer from false. Compare the two that way, not by pointer.
			gotVision, gotKnown := ModelHasVision(got)
			wantVision, wantKnown := ModelHasVision(tt.expected)
			if gotKnown != wantKnown || (gotKnown && gotVision != wantVision) {
				t.Errorf("Vision = (%v, known=%v), want (%v, known=%v)", gotVision, gotKnown, wantVision, wantKnown)
			}
			if got.ContextMargin != tt.expected.ContextMargin {
				t.Errorf("ContextMargin = %d, want %d", got.ContextMargin, tt.expected.ContextMargin)
			}
			if (got.SupportsTools == nil) != (tt.expected.SupportsTools == nil) {
				t.Fatalf("SupportsTools nil mismatch: got %v, want %v", got.SupportsTools, tt.expected.SupportsTools)
			}
			if got.SupportsTools != nil && *got.SupportsTools != *tt.expected.SupportsTools {
				t.Errorf("SupportsTools = %v, want %v", *got.SupportsTools, *tt.expected.SupportsTools)
			}
		})
	}
}

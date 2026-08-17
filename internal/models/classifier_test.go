package models

import "testing"

func TestClassifyEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		modelID  string
		expected EndpointType
	}{
		{
			name:     "minimax m2.5 uses chat completions on Zen",
			modelID:  "minimax-m2.5",
			expected: EndpointChatCompletions,
		},
		{
			name:     "minimax m2.7 uses chat completions on Zen",
			modelID:  "minimax-m2.7",
			expected: EndpointChatCompletions,
		},
		{
			name:     "minimax m3 uses chat completions on Zen",
			modelID:  "minimax-m3",
			expected: EndpointChatCompletions,
		},
		{
			name:     "qwen3.5-plus uses anthropic endpoint",
			modelID:  "qwen3.5-plus",
			expected: EndpointAnthropic,
		},
		{
			name:     "qwen3.6-plus uses anthropic endpoint",
			modelID:  "qwen3.6-plus",
			expected: EndpointAnthropic,
		},
		{
			name:     "qwen3.7-plus uses anthropic endpoint",
			modelID:  "qwen3.7-plus",
			expected: EndpointAnthropic,
		},
		{
			name:     "qwen3.7-max uses anthropic endpoint",
			modelID:  "qwen3.7-max",
			expected: EndpointAnthropic,
		},
		{
			name:     "gemini-3.5-flash uses gemini endpoint",
			modelID:  "gemini-3.5-flash",
			expected: EndpointGemini,
		},
		{
			name:     "gemini-3.1-pro uses gemini endpoint",
			modelID:  "gemini-3.1-pro",
			expected: EndpointGemini,
		},
		{
			name:     "gemini-3-flash uses gemini endpoint",
			modelID:  "gemini-3-flash",
			expected: EndpointGemini,
		},
		{
			name:     "gpt-5.5 uses responses endpoint",
			modelID:  "gpt-5.5",
			expected: EndpointResponses,
		},
		{
			name:     "gpt-5.4 uses responses endpoint",
			modelID:  "gpt-5.4",
			expected: EndpointResponses,
		},
		{
			name:     "gpt-5 uses responses endpoint",
			modelID:  "gpt-5",
			expected: EndpointResponses,
		},
		{
			name:     "kimi-k2.6 uses chat completions endpoint",
			modelID:  "kimi-k2.6",
			expected: EndpointChatCompletions,
		},
		{
			name:     "kimi-k2.7-code uses chat completions endpoint",
			modelID:  "kimi-k2.7-code",
			expected: EndpointChatCompletions,
		},
		{
			name:     "kimi-k2.5 uses chat completions endpoint",
			modelID:  "kimi-k2.5",
			expected: EndpointChatCompletions,
		},
		{
			name:     "mimo-v2.5 uses chat completions endpoint",
			modelID:  "mimo-v2.5",
			expected: EndpointChatCompletions,
		},
		{
			name:     "mimo-v2.5-pro uses chat completions endpoint",
			modelID:  "mimo-v2.5-pro",
			expected: EndpointChatCompletions,
		},
		{
			name:     "glm-5.1 uses chat completions endpoint",
			modelID:  "glm-5.1",
			expected: EndpointChatCompletions,
		},
		{
			name:     "glm-5.2 uses chat completions endpoint",
			modelID:  "glm-5.2",
			expected: EndpointChatCompletions,
		},
		{
			name:     "glm-5 uses chat completions endpoint",
			modelID:  "glm-5",
			expected: EndpointChatCompletions,
		},
		{
			name:     "deepseek-v4-flash uses chat completions endpoint",
			modelID:  "deepseek-v4-flash",
			expected: EndpointChatCompletions,
		},
		{
			name:     "grok-build-0.1 uses chat completions endpoint",
			modelID:  "grok-build-0.1",
			expected: EndpointChatCompletions,
		},
		{
			name:     "big-pickle uses chat completions endpoint",
			modelID:  "big-pickle",
			expected: EndpointChatCompletions,
		},
		{
			name:     "north-mini-code-free uses chat completions endpoint",
			modelID:  "north-mini-code-free",
			expected: EndpointChatCompletions,
		},
		{
			name:     "deepseek-v4-flash-free uses chat completions endpoint",
			modelID:  "deepseek-v4-flash-free",
			expected: EndpointChatCompletions,
		},
		{
			name:     "claude-sonnet-4-5 uses anthropic endpoint",
			modelID:  "claude-sonnet-4-5",
			expected: EndpointAnthropic,
		},
		{
			name:     "claude-opus-4-7 uses anthropic endpoint",
			modelID:  "claude-opus-4-7",
			expected: EndpointAnthropic,
		},
		{
			name:     "claude-haiku-4-5 uses anthropic endpoint",
			modelID:  "claude-haiku-4-5",
			expected: EndpointAnthropic,
		},
		{
			name:     "unknown model uses chat completions endpoint",
			modelID:  "unknown-model",
			expected: EndpointChatCompletions,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClassifyEndpoint(tt.modelID); got != tt.expected {
				t.Fatalf("ClassifyEndpoint(%q) = %v, want %v", tt.modelID, got, tt.expected)
			}
		})
	}
}

func TestIsAnthropicModelOnlyRoutesNativeAnthropicModels(t *testing.T) {
	tests := []struct {
		name    string
		modelID string
		want    bool
	}{
		{
			name:    "minimax m2.5 uses anthropic endpoint on Go provider",
			modelID: "minimax-m2.5",
			want:    true,
		},
		{
			name:    "minimax m2.7 uses anthropic endpoint on Go provider",
			modelID: "minimax-m2.7",
			want:    true,
		},
		{
			name:    "minimax m3 uses anthropic endpoint on Go provider",
			modelID: "minimax-m3",
			want:    true,
		},
		{
			name:    "deepseek pro uses openai endpoint",
			modelID: "deepseek-v4-pro",
			want:    false,
		},
		{
			name:    "deepseek flash uses openai endpoint",
			modelID: "deepseek-v4-flash",
			want:    false,
		},
		{
			name:    "kimi k2.6 uses openai endpoint",
			modelID: "kimi-k2.6",
			want:    false,
		},
		{
			name:    "kimi k2.7-code uses openai endpoint",
			modelID: "kimi-k2.7-code",
			want:    false,
		},
		{
			name:    "kimi k3 uses openai endpoint",
			modelID: "kimi-k3",
			want:    false,
		},
		{
			name:    "glm-5.1 uses openai endpoint",
			modelID: "glm-5.1",
			want:    false,
		},
		{
			name:    "glm-5.2 uses openai endpoint",
			modelID: "glm-5.2",
			want:    false,
		},
		{
			name:    "glm-5 uses openai endpoint",
			modelID: "glm-5",
			want:    false,
		},
		{
			name:    "kimi-k2.5 uses openai endpoint",
			modelID: "kimi-k2.5",
			want:    false,
		},
		{
			name:    "mimo-v2.5 uses openai endpoint",
			modelID: "mimo-v2.5",
			want:    false,
		},
		{
			name:    "mimo-v2.5-pro uses openai endpoint",
			modelID: "mimo-v2.5-pro",
			want:    false,
		},
		{
			name:    "qwen3.5-plus uses anthropic endpoint on Go provider",
			modelID: "qwen3.5-plus",
			want:    true,
		},
		{
			name:    "qwen3.6-plus uses anthropic endpoint on Go provider",
			modelID: "qwen3.6-plus",
			want:    true,
		},
		{
			name:    "qwen3.7-plus uses anthropic endpoint on Go provider",
			modelID: "qwen3.7-plus",
			want:    true,
		},
		{
			name:    "qwen3.7-max uses anthropic endpoint (no oa-compat support)",
			modelID: "qwen3.7-max",
			want:    true,
		},
		{
			name:    "claude models use openai endpoint on Go provider",
			modelID: "claude-sonnet-4-5",
			want:    false,
		},
		{
			name:    "claude-opus-4-7 uses openai endpoint on Go provider",
			modelID: "claude-opus-4-7",
			want:    false,
		},
		{
			name:    "claude-haiku-4-5 uses openai endpoint on Go provider",
			modelID: "claude-haiku-4-5",
			want:    false,
		},
		{
			name:    "claude-3-5-haiku uses openai endpoint on Go provider",
			modelID: "claude-3-5-haiku",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsAnthropicModel(tt.modelID); got != tt.want {
				t.Fatalf("IsAnthropicModel(%q) = %v, want %v", tt.modelID, got, tt.want)
			}
		})
	}
}

func TestIsZenAnthropicModel(t *testing.T) {
	tests := []struct {
		modelID string
		want    bool
	}{
		// Claude models on Zen use Anthropic endpoint
		{"claude-sonnet-4-5", true},
		{"claude-opus-4-7", true},
		{"claude-haiku-4-5", true},
		{"claude-3-5-haiku", true},
		{"claude-3-5-sonnet", true},
		{"claude-3-opus", true},
		// Qwen models on Zen use Anthropic endpoint
		{"qwen3.7-max", true},
		{"qwen3.7-plus", true},
		{"qwen3.6-plus", true},
		{"qwen3.5-plus", true},
		{"qwen3.5", true},
		// Non-Anthropic models
		{"kimi-k2.6", false},
		{"kimi-k2.7-code", false},
		{"glm-5.1", false},
		{"glm-5.2", false},
		{"glm-5", false},
		{"gemini-3.5-flash", false},
		{"gemini-3.1-pro", false},
		{"gpt-5.5", false},
		{"gpt-5", false},
		{"minimax-m2.5", false},
		{"minimax-m2.7", false},
		{"minimax-m3", false},
		{"deepseek-v4-pro", false},
		{"mimo-v2.5", false},
	}

	for _, tt := range tests {
		t.Run(tt.modelID, func(t *testing.T) {
			if got := IsZenAnthropicModel(tt.modelID); got != tt.want {
				t.Fatalf("IsZenAnthropicModel(%q) = %v, want %v", tt.modelID, got, tt.want)
			}
		})
	}
}

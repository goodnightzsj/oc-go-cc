package config

import (
	"slices"
	"testing"
)

func TestProviderDisplayOrder(t *testing.T) {
	providers := []string{"z-custom", "openrouter", "aws_bedrock", "opencode-zen", "commandcode", "opencode_go", "a-custom"}
	slices.SortFunc(providers, CompareProviderDisplay)
	want := []string{"opencode_go", "commandcode", "opencode-zen", "aws_bedrock", "openrouter", "a-custom", "z-custom"}
	if !slices.Equal(providers, want) {
		t.Fatalf("provider display order = %v, want %v", providers, want)
	}
	if CompareProviderDisplay("", "commandcode") >= 0 {
		t.Fatal("legacy empty provider must retain its OpenCode Go display position")
	}
}

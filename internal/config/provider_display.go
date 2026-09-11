package config

import (
	"cmp"
	"strings"
)

// CompareProviderDisplay orders labels only; routing, fallback and key order
// remain owned by their configured chains.
func CompareProviderDisplay(a, b string) int {
	return cmp.Or(cmp.Compare(providerDisplayRank(a), providerDisplayRank(b)), strings.Compare(a, b))
}

func providerDisplayRank(provider string) int {
	switch NormalizeProvider(provider) {
	case "opencode-go":
		return 0
	case "commandcode":
		return 1
	case "opencode-zen":
		return 2
	case "aws-bedrock":
		return 3
	case "openrouter":
		return 4
	default:
		return 5
	}
}

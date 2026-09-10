package handlers

import (
	"testing"

	"github.com/routatic/proxy/internal/config"
)

func TestModelChainKeepsSameIDOnDifferentProviders(t *testing.T) {
	base := []config.ModelConfig{{Provider: "opencode-go", ModelID: "same-model"}}
	extra := []config.ModelConfig{
		{Provider: "opencode_go", ModelID: "same-model"},
		{Provider: "opencode-zen", ModelID: "same-model"},
	}
	chain := appendUniqueModels(base, extra)
	if len(chain) != 2 || chain[1].Provider != "opencode-zen" {
		t.Fatalf("provider/model chain = %+v", chain)
	}
}

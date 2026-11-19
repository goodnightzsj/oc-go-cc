package config

import (
	"fmt"
	"strings"
	"testing"
)

func TestLoadJSONClineFallbackRequiresExplicitBillingPool(t *testing.T) {
	for _, id := range []string{"deepseek-v4.1-flash", "cline-pass/deepseek-v4.1-flash", "deepseek/deepseek-v4.1-flash", "vmc/deepseek-v4.1-flash"} {
		payload := fmt.Sprintf(`{"cline_pass":{"api_key":"synthetic-key"},"fallbacks":{"default":[{"provider":"cline-pass","model_id":%q}]}}`, id)
		cfg, err := LoadJSON([]byte(payload))
		if !strings.Contains(id, "/") {
			if err == nil || !strings.Contains(err.Error(), "fallbacks") || !strings.Contains(err.Error(), "modelType/model") {
				t.Fatalf("bare fallback not rejected at load boundary: %v", err)
			}
			continue
		}
		if err != nil || cfg.Fallbacks["default"][0].ModelID != id {
			t.Fatalf("explicit billing pool rewritten: cfg=%v err=%v", cfg != nil, err)
		}
	}
}

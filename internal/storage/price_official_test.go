package storage

import "testing"

func TestPriceForProviderModel_OfficialOpenCode(t *testing.T) {
	// input / output / cache_read / cache_write (per 1M tokens, USD), base
	// context band, re-verified 2026-09-14 against opencode.ai/docs/zh-cn/go.
	// The peak multiplier and the context tiers are applied at pricing time
	// from the same table, so they are asserted separately.
	//
	// cacheWrite is 0 for models the docs list with no separate cache-write
	// price; those bill cache creation at the input rate.
	cases := []struct {
		model                          string
		in, out, cacheRead, cacheWrite float64
	}{
		{"deepseek-v4-flash", 0.15, 0.60, 0.003, 0},
		{"deepseek-v4-pro", 0.66, 1.98, 0.022, 0},
		{"kimi-k3", 3.00, 15.00, 0.30, 0},
		{"kimi-k2.7-code", 0.95, 4.00, 0.19, 0},
		{"kimi-k2.6", 0.95, 4.00, 0.16, 0},
		{"glm-5.3-flash", 0.15, 0.50, 0.03, 0},
		{"glm-5.3", 1.40, 4.40, 0.26, 0},
		{"glm-5.2", 1.40, 4.40, 0.26, 0},
		{"glm-5.1", 1.40, 4.40, 0.26, 0},
		{"mimo-v2.5", 0.14, 0.28, 0.0028, 0},
		{"mimo-v2.5-pro", 0.435, 0.87, 0.003625, 0},
		{"minimax-m3", 0.30, 1.20, 0.06, 0},
		// The docs carry a Cache Write column that models.dev omits entirely.
		{"minimax-m2.7", 0.30, 1.20, 0.06, 0.375},
		{"minimax-m2.5", 0.30, 1.20, 0.06, 0.375},
		{"grok-4.6", 2.00, 6.00, 0.50, 0},
		{"grok-4.5", 2.00, 6.00, 0.30, 0},
		{"qwen3.8-max", 2.00, 6.00, 0.25, 2.50},
		{"qwen3.8-flash", 0.15, 0.47, 0.016, 0.20},
		{"qwen3.7-max", 2.50, 7.50, 0.50, 3.125},
		{"qwen3.7-plus", 0.40, 1.60, 0.04, 0.50},
		{"qwen3.6-plus", 0.50, 3.00, 0.05, 0.625},
		// OpenCode Go publishes only Luna of the GPT-5.6 family. The previous
		// table's loose "gpt-5.6" rule would have priced every variant at one
		// rate; naming Luna exactly leaves Sol and Terra to the generic
		// fallback, which is a known approximation rather than a wrong exact.
		{"gpt-5.6-luna", 0.20, 1.20, 0.02, 0.25},
	}
	for _, c := range cases {
		in, out, cacheRead, cacheWrite, ok := PriceForProviderModel("opencode-go", c.model, 0)
		if !ok || in != c.in || out != c.out || cacheRead != c.cacheRead || cacheWrite != c.cacheWrite {
			t.Errorf("%s: got %v/%v/%v/%v ok=%v, want %v/%v/%v/%v",
				c.model, in, out, cacheRead, cacheWrite, ok, c.in, c.out, c.cacheRead, c.cacheWrite)
		}
	}
}

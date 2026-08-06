package storage

import "testing"

func TestPriceForModel_OfficialOpenCode(t *testing.T) {
	// input / output / cache_read (per 1M tokens, USD), per opencode.ai/docs/go
	cases := []struct{ model string; in, out, cache float64 }{
		{"deepseek-v4-flash", 0.14, 0.28, 0.0028},
		{"deepseek-v4-pro", 0.435, 0.87, 0.003625},
		{"kimi-k2.6", 0.95, 4.00, 0.16},
		{"glm-5.2", 1.40, 4.40, 0.26},
		{"minimax-m3", 0.30, 1.20, 0.06},
		{"qwen3.6-plus", 0.50, 3.00, 0.05},
		{"qwen3.7-plus", 0.40, 1.60, 0.04},
	}
	for _, c := range cases {
		in, out, cache, ok := PriceForModel(c.model)
		if !ok || in != c.in || out != c.out || cache != c.cache {
			t.Errorf("%s: got %v/%v/%v ok=%v, want %v/%v/%v", c.model, in, out, cache, ok, c.in, c.out, c.cache)
		}
	}
}

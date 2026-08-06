package storage

import "testing"

func TestPriceForModel_OfficialOpenCode(t *testing.T) {
	cases := []struct{ model string; in, out float64 }{
		{"deepseek-v4-flash", 0.14, 0.28},
		{"deepseek-v4-pro", 0.435, 0.87},
		{"kimi-k2.6", 0.95, 4.00},
		{"glm-5.2", 1.40, 4.40},
		{"minimax-m3", 0.30, 1.20},
		{"qwen3.6-plus", 0.50, 3.00},
		{"qwen3.7-plus", 0.40, 1.60},
	}
	for _, c := range cases {
		in, out, ok := PriceForModel(c.model)
		if !ok || in != c.in || out != c.out {
			t.Errorf("%s: got %v/%v ok=%v, want %v/%v", c.model, in, out, ok, c.in, c.out)
		}
	}
}

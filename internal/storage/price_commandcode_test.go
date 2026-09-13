package storage

import (
	"database/sql"
	"math"
	"testing"
	"time"
)

// CommandCode publishes per-model rates on its Pricing & Limits page, and the
// account's own usage records bill at exactly those rates. This pins the values
// the proxy prices with; a drift here is a silent cost error, not a crash.
func TestPriceForProviderModel_OfficialCommandCode(t *testing.T) {
	// input / output / cache_read per 1M tokens, USD, off-peak.
	cases := []struct {
		model              string
		in, out, cacheRead float64
	}{
		{"deepseek/deepseek-v4-flash", 0.15, 0.60, 0.003},
		{"deepseek/deepseek-v4-pro", 0.66, 1.98, 0.022},
		{"moonshotai/Kimi-K2.6", 0.95, 4.00, 0.16},
		{"zai-org/GLM-5.2", 1.40, 4.40, 0.26},
		{"Qwen/Qwen3.7-Plus", 0.40, 1.60, 0.08},
		{"MiniMaxAI/MiniMax-M3", 0.30, 1.20, 0.06}, // -50% deal already applied
		{"longcat-2.0:free", 0, 0, 0},              // free models are priced at 0, not unknown
	}
	for _, c := range cases {
		in, out, cacheRead, _, ok := PriceForProviderModel("commandcode", c.model, 0)
		if !ok || in != c.in || out != c.out || cacheRead != c.cacheRead {
			t.Errorf("%s: got %v/%v/%v ok=%v, want %v/%v/%v",
				c.model, in, out, cacheRead, ok, c.in, c.out, c.cacheRead)
		}
	}
}

// Nineteen real CommandCode runs, replayed through the proxy's own pricing.
// `input` is the cache-miss part and `cacheRead` the hit part, which is the
// shape this proxy stores (splitPromptTokens strips the cached prefix), while
// CommandCode reports the two combined. `want` is the platform's own figure for
// that run, taken from api.commandcode.ai/internal/usage, so this asserts the
// local estimate reproduces the bill rather than merely being self-consistent.
func TestCommandCodeRunsMatchThePlatformBill(t *testing.T) {
	// Every captured run falls outside 01-04 and 06-10 UTC, so the DeepSeek
	// peak multiplier is 1 and the comparison is against the off-peak rates.
	offPeak := time.Date(2026, 9, 13, 12, 30, 0, 0, time.UTC)

	cases := []struct {
		model                    string
		input, output, cacheRead int64
		want                     float64
	}{
		{"deepseek/deepseek-v4-flash", 32, 16, 0, 1.44e-05},
		{"moonshotai/Kimi-K2.6", 10, 16, 0, 7.35e-05},
		{"zai-org/GLM-5.2", 14, 16, 0, 9e-05},
		{"Qwen/Qwen3.7-Plus", 12, 118, 0, 0.0001936},
		{"zai-org/GLM-5.2", 14, 24, 0, 0.0001252},
		{"deepseek/deepseek-v4-flash", 153, 29, 0, 4.035e-05},
		{"deepseek/deepseek-v4-flash", 816, 186, 0, 0.000234},
		{"moonshotai/Kimi-K2.6", 127, 47, 0, 0.00030865},
		{"deepseek/deepseek-v4-flash", 176, 160, 640, 0.00012432},
		{"deepseek/deepseek-v4-flash", 5615, 9, 4608, 0.000861474},
		{"deepseek/deepseek-v4-flash", 247, 48, 9984, 9.5802e-05},
		{"deepseek/deepseek-v4-flash", 217, 5, 10112, 6.5886e-05},
		{"moonshotai/Kimi-K2.6", 9091, 60, 0, 0.00887645},
		{"moonshotai/Kimi-K2.6", 85, 43, 9090, 0.00170715},
		{"deepseek/deepseek-v4-flash", 1168, 66, 0, 0.0002148},
		{"deepseek/deepseek-v4-flash", 312, 112, 512, 0.000115536},
		{"deepseek/deepseek-v4-flash", 217, 65, 1024, 7.4622e-05},
		{"deepseek/deepseek-v4-flash", 32, 26, 0, 2.04e-05},
		{"deepseek/deepseek-v4-flash", 31, 16, 0, 1.425e-05},
	}
	for _, c := range cases {
		got, ok := costForProviderTokensAt("commandcode", c.model, c.input, c.output, c.cacheRead, 0,
			sql.NullFloat64{}, sql.NullFloat64{}, offPeak)
		if !ok {
			t.Errorf("%s: platform publishes rates, so the cost must be known", c.model)
			continue
		}
		if math.Abs(got-c.want) > 1e-12 {
			t.Errorf("%s (in=%d out=%d cr=%d): got %.10f, want platform %.10f",
				c.model, c.input, c.output, c.cacheRead, got, c.want)
		}
	}
}

package storage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// clinePricePageFixture is the shape of the real page: a Model ID table
// followed by the Reference pricing table, with the footnote and peak
// qualifiers the live page carries.
const clinePricePageFixture = `
| Model             | Model ID                       |
| ----------------- | ------------------------------ |
| GLM-5.3           | ` + "`cline-pass/glm-5.3`" + `           |
| Kimi K2.7 Code    | ` + "`cline-pass/kimi-k2.7-code`" + `    |
| DeepSeek V4 Pro   | ` + "`cline-pass/deepseek-v4-pro`" + `   |
| Qwen3.7 Plus      | ` + "`cline-pass/qwen3.7-plus`" + `      |

## Reference pricing

| Model                                    | Input  | Output  | Cached Read | Cached Write |
| ---------------------------------------- | ------ | ------- | ----------- | ------------ |
| GLM-5.3                                  | \$1.40 | \$4.40  | \$0.26      | -            |
| Kimi K2.7 Code                           | \$0.95 | \$4.00  | \$0.19      | -            |
| DeepSeek V4 Pro (Peak)<sup>1</sup>       | \$1.32 | \$3.96  | \$0.044     | -            |
| DeepSeek V4 Pro (Off-peak)<sup>1</sup>   | \$0.66 | \$1.98  | \$0.022     | -            |
| Qwen3.7 Plus (≤ 256K tokens)             | \$0.40 | \$1.60  | \$0.04      | \$0.50       |
| Qwen3.7 Plus (> 256K tokens)             | \$1.20 | \$4.80  | \$0.12      | \$1.50       |
`

// The page's two tables are joined on the display label, so the id comes from
// the platform rather than from folding the label into a slug. A guessed slug
// that folded one character differently would price a different model.
func TestClinePassPricesJoinTheIDTable(t *testing.T) {
	entries, err := parseClinePassPricePage([]byte(clinePricePageFixture))
	if err != nil {
		t.Fatal(err)
	}
	byMatch := map[string]priceEntry{}
	for _, e := range entries {
		byMatch[e.Match] = e
	}
	for _, want := range []string{"cline-pass/glm-5.3", "cline-pass/kimi-k2.7-code", "cline-pass/deepseek-v4-pro", "cline-pass/qwen3.7-plus"} {
		if _, ok := byMatch[want]; !ok {
			t.Errorf("no rule for %q; parsed %d rules", want, len(entries))
		}
	}
	if e := byMatch["cline-pass/kimi-k2.7-code"]; e.Input != 0.95 || e.Output != 4.0 || e.CacheRead != 0.19 {
		t.Errorf("kimi-k2.7-code = %+v, want 0.95/4.00/0.19", e)
	}
}

// Peak and Off-peak are two rows for one model. With no verified peak window
// registered for this platform, the estimate uses Off-peak; taking the Peak row
// would price real traffic at 2x while looking well-formed.
func TestClinePassPricesUseOffPeakNotPeak(t *testing.T) {
	entries, err := parseClinePassPricePage([]byte(clinePricePageFixture))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Match != "cline-pass/deepseek-v4-pro" {
			continue
		}
		if e.Input != 0.66 || e.Output != 1.98 {
			t.Errorf("deepseek-v4-pro = %v/%v, want the Off-peak 0.66/1.98", e.Input, e.Output)
		}
		return
	}
	t.Fatal("deepseek-v4-pro has no rule")
}

// A context band is a tier, and the "≤" row is the base both bands sit around:
// the ">" row is the band above the named threshold.
func TestClinePassPricesMapBandsToTiers(t *testing.T) {
	entries, err := parseClinePassPricePage([]byte(clinePricePageFixture))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Match != "cline-pass/qwen3.7-plus" {
			continue
		}
		if e.Input != 0.40 || e.Output != 1.60 || e.CacheWrite != 0.50 {
			t.Errorf("base band = %v/%v/%v, want the ≤ 256K row 0.4/1.6/0.5", e.Input, e.Output, e.CacheWrite)
		}
		if len(e.Tiers) != 1 {
			t.Fatalf("got %d tiers, want exactly the > 256K band", len(e.Tiers))
		}
		tier := e.Tiers[0]
		if tier.Size != 256*1024 || tier.Input != 1.20 || tier.Output != 4.80 || tier.CacheWrite != 1.50 {
			t.Errorf("tier = %+v, want size 262144 at 1.2/4.8/1.5", tier)
		}
		// A request above the threshold must bill the band, not the base.
		in, out, _, cw := e.ratesFor(300000)
		if in != 1.20 || out != 4.80 || cw != 1.50 {
			t.Errorf("ratesFor(300000) = %v/%v/%v, want the band", in, out, cw)
		}
		in, out, _, _ = e.ratesFor(1000)
		if in != 0.40 || out != 1.60 {
			t.Errorf("ratesFor(1000) = %v/%v, want the base", in, out)
		}
		return
	}
	t.Fatal("qwen3.7-plus has no rule")
}

// A price row naming a model the id table does not list means the page moved
// under us. Dropping the row would show a missing price; the table is this
// platform's only price source, so it has to be an error.
func TestClinePassPricesRejectUnknownLabel(t *testing.T) {
	page := `
| Model  | Model ID                  |
| ------ | ------------------------- |
| GLM-5.3 | ` + "`cline-pass/glm-5.3`" + ` |

| Model   | Input  | Output | Cached Read | Cached Write |
| ------- | ------ | ------ | ----------- | ------------ |
| Mystery | \$1.00 | \$2.00 | -           | -            |
`
	if _, err := parseClinePassPricePage([]byte(page)); err == nil {
		t.Fatal("a price row with no matching id was accepted")
	}
}

// The embedded seed and the live page must agree: a drift between them means
// one of the two is stale, and the seed is what a failed refresh falls back to.
func TestClinePassSeedMatchesPublishedPage(t *testing.T) {
	entries, err := parseClinePassPricePage([]byte(clinePricePageFixture))
	if err != nil {
		t.Fatal(err)
	}
	seed, err := rateTables()
	if err != nil {
		t.Fatal(err)
	}
	seedByMatch := map[string]priceEntry{}
	for _, e := range seed["cline-pass"] {
		seedByMatch[e.Match] = e
	}
	if len(seedByMatch) == 0 {
		t.Fatal("the embedded cline-pass table is empty, so a failed refresh would leave no prices")
	}
	for _, want := range entries {
		got, ok := seedByMatch[want.Match]
		if !ok {
			t.Errorf("seed has no rule for %q", want.Match)
			continue
		}
		if got.Input != want.Input || got.Output != want.Output || got.CacheRead != want.CacheRead || got.CacheWrite != want.CacheWrite || len(got.Tiers) != len(want.Tiers) {
			t.Errorf("seed %q = %+v, page says %+v", want.Match, got, want)
		}
	}
}

func TestClinePassPriceFetchReportsHTTPFailure(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer upstream.Close()
	if _, err := FetchClinePassPrices(context.Background(), upstream.Client(), upstream.URL); err == nil {
		t.Fatal("a 404 price page was accepted")
	}
}

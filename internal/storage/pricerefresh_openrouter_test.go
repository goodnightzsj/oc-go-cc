package storage

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/routatic/proxy/internal/site"
)

// The listing states a per-token price as a string; this package prices per
// million tokens. Getting the scale wrong by 1e6 would make every OpenRouter
// row look free.
func TestOpenRouterPricesAreScaledToPerMillion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "anthropic/claude-x", "pricing": map[string]string{"prompt": "0.000003", "completion": "0.000015"}},
				// A genuinely free model: zero is a price, not an absence.
				{"id": "meta/llama-free", "pricing": map[string]string{"prompt": "0", "completion": "0"}},
				// No usable rate at all: must not be recorded as free.
				{"id": "broken/model", "pricing": map[string]string{"prompt": "", "completion": "n/a"}},
			},
		})
	}))
	defer srv.Close()

	entries, _, err := FetchOpenRouterPrices(context.Background(), srv.Client(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]priceEntry{}
	for _, e := range entries {
		byID[e.Match] = e
	}
	if _, ok := byID["broken/model"]; ok {
		t.Error("a model with no usable rate was recorded; it would read as priced")
	}
	paid := byID["anthropic/claude-x"]
	if paid.Input != 3 || paid.Output != 15 {
		t.Errorf("claude-x = %v/%v per million, want 3/15", paid.Input, paid.Output)
	}
	free, ok := byID["meta/llama-free"]
	if !ok {
		t.Fatal("a free model was dropped; zero is a real price here")
	}
	if free.Input != 0 || free.Output != 0 {
		t.Errorf("free model = %v/%v, want 0/0", free.Input, free.Output)
	}
}

// An empty answer must be an error, not an installed empty table: clearing the
// prices would silently zero every OpenRouter cost.
func TestOpenRouterPricesRejectAnEmptyListing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()
	if _, _, err := FetchOpenRouterPrices(context.Background(), srv.Client(), srv.URL); err == nil {
		t.Fatal("an empty listing was accepted as a price table")
	}
}

// The platform has to be bound to a table, or the lookup refuses before it ever
// consults one and every OpenRouter model reads as unpriced.
func TestOpenRouterHasARateTable(t *testing.T) {
	desc, found := site.Lookup("openrouter")
	if !found {
		t.Fatal("openrouter is not a registered platform")
	}
	if desc.RateTable == "" {
		t.Fatal("openrouter has no rate table bound; PriceForProviderModel returns unknown before reading any prices")
	}
}

// The listing leads with the highest band: a model with a cheaper override
// bills at the cheaper rate outside that window. Storing the headline as the
// base and then multiplying it by a peak factor charged tencent/hy3 1.6x
// everywhere.
func TestOpenRouterUsesTheCheapestDeclaredBandAsBase(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"tencent/hy3","pricing":{
			"prompt":"0.000000132","completion":"0.000000528","input_cache_read":"0.000000033",
			"overrides":[
				{"utc_start":0,"utc_end":1600,"prompt":"0.000000132","completion":"0.000000528"},
				{"utc_start":1600,"utc_end":0,"prompt":"0.0000000825","completion":"0.00000033"}
			]}}]}`))
	}))
	defer srv.Close()

	entries, rules, err := FetchOpenRouterPrices(context.Background(), srv.Client(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	got := entries[0]
	if got.Input != 0.0825 || got.Output != 0.33 {
		t.Fatalf("base = %v/%v per million, want the off-peak band 0.0825/0.33", got.Input, got.Output)
	}
	if got.CacheRead == 0 {
		t.Fatal("cache reads were dropped; a cached prompt would bill at zero")
	}
	if len(rules) != 1 || len(rules[0].Windows) != 1 {
		t.Fatalf("rules = %+v, want one rule covering 16:00 to midnight", rules)
	}
	// The 00:00-16:00 band is the expensive one; the 16:00-00:00 band is the
	// base, so the rule covers the morning window.
	if window := rules[0].Windows[0]; window.Start != 0 || window.End != 16 {
		t.Fatalf("window = %+v, want 0-16", window)
	}
	if diff := rules[0].Multiplier - 1.6; diff > 0.001 || diff < -0.001 {
		t.Fatalf("multiplier = %v, want 1.6", rules[0].Multiplier)
	}
	if !rules[0].AllDays {
		t.Fatal("the payload states no day condition, so the rule applies every day")
	}
}

// A context band and cache prices are separate facts from the base rate.
func TestOpenRouterKeepsTiersAndCachePrices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"openai/gpt-x","pricing":{
			"prompt":"0.000008","completion":"0.00004","input_cache_read":"0.0000004",
			"overrides":[{"min_prompt_tokens":272000,"prompt":"0.000016","completion":"0.00006","input_cache_read":"0.0000008"}]}}]}`))
	}))
	defer srv.Close()

	entries, _, err := FetchOpenRouterPrices(context.Background(), srv.Client(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d", len(entries))
	}
	entry := entries[0]
	if entry.Input != 8 || entry.Output != 40 || math.Abs(entry.CacheRead-0.4) > 1e-9 {
		t.Fatalf("base = %v/%v cache=%v, want 8/40 cache=0.4", entry.Input, entry.Output, entry.CacheRead)
	}
	if len(entry.Tiers) != 1 || entry.Tiers[0].Size != 272000 || entry.Tiers[0].Input != 16 {
		t.Fatalf("tiers = %+v, want one band at 272000 tokens / 16 per million", entry.Tiers)
	}
	in, out, cacheRead, _ := entry.ratesFor(300000)
	if in != 16 || out != 60 || math.Abs(cacheRead-0.8) > 1e-9 {
		t.Fatalf("ratesFor(300k) = %v/%v cache=%v, want the long-context band", in, out, cacheRead)
	}
}

// One unparsable side is an absence, not a free half.
func TestOpenRouterSkipsAHalfPricedModel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[
			{"id":"half/priced","pricing":{"prompt":"0.000001","completion":""}},
			{"id":"ok/model","pricing":{"prompt":"0.000001","completion":"0.000002"}}]}`))
	}))
	defer srv.Close()
	entries, _, err := FetchOpenRouterPrices(context.Background(), srv.Client(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Match == "half/priced" {
			t.Fatalf("a half priced model was recorded as %v/%v", entry.Input, entry.Output)
		}
	}
}

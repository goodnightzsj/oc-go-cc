package storage

import (
	"context"
	"encoding/json"
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

	entries, err := FetchOpenRouterPrices(context.Background(), srv.Client(), srv.URL)
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
	if _, err := FetchOpenRouterPrices(context.Background(), srv.Client(), srv.URL); err == nil {
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

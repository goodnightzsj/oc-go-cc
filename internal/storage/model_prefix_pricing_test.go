package storage

import (
	"context"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/history"
)

func TestPricingIdentityPreservesOtherProviders(t *testing.T) {
	db := newCostTestDB(t)
	if _, err := db.DB().Exec(`INSERT INTO models (id, provider, name, cost_input_per_m, cost_output_per_m) VALUES
		('zen-custom', 'opencode-zen', 'custom:v2', 1, 2),
		('zen-other', 'opencode-zen', 'v2', 9, 9)`); err != nil {
		t.Fatal(err)
	}
	insertCostRecord(t, db, history.RequestRecord{
		ID: "colon-id", Provider: "opencode-zen", Model: "custom:v2",
		InputTokens: 1_000_000, Success: true,
	})
	for _, backfill := range []bool{false, true} {
		if backfill {
			if _, err := db.DB().Exec(`UPDATE requests SET cost_usd = NULL`); err != nil {
				t.Fatal(err)
			}
			if _, err := db.BackfillRequestCosts(context.Background()); err != nil {
				t.Fatal(err)
			}
		}
		rows, _, err := NewRequests(db).Query(t.Context(), RequestQuery{Page: 1, PageSize: 10})
		if err != nil || len(rows) != 1 {
			t.Fatalf("rows=%v err=%v", rows, err)
		}
		if !rows[0].CostKnown || rows[0].CostUSD != 1 || rows[0].Model != "custom:v2" {
			t.Errorf("backfill=%v: custom:v2 must retain its identity and cost 1, got %+v", backfill, rows[0])
		}
	}
}

// Realm parsing belongs to WorkBuddy, not to arbitrary model IDs.
func TestModelNameForPricing(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"global:deepseek-v4.1-flash", "deepseek-v4.1-flash"},
		{"cn:deepseek-v4.1-flash", "deepseek-v4.1-flash"},
		{"global:glm-5.3", "glm-5.3"},
		// No prefix: unchanged.
		{"deepseek-v4.1-flash", "deepseek-v4.1-flash"},
		{"deepseek/deepseek-v4-pro", "deepseek/deepseek-v4-pro"},
		// A colon that is part of the name, not a realm, is left alone.
		{"vendor:model:variant", "vendor:model:variant"},
		{"global:model:variant", "model:variant"},
		{"model-with.dots:tag", "model-with.dots:tag"},
		// Degenerate shapes must not lose the whole name.
		{":leading", ":leading"},
		{"trailing:", "trailing:"},
		{"global:", "global:"},
		{"", ""},
	} {
		if got := modelNameForPricing("workbuddy", tc.in); got != tc.want {
			t.Errorf("modelNameForPricing(%q) = %q, want %q", tc.in, got, tc.want)
		}
		for _, provider := range []string{"opencode-zen", "openrouter", "commandcode", "aws-bedrock", "", "custom"} {
			if got := modelNameForPricing(provider, tc.in); got != tc.in {
				t.Errorf("%s model identity changed: %q => %q", provider, tc.in, got)
			}
		}
	}
}

// Realm-prefixed records retain their wire ID while using WorkBuddy's rate table.
func TestRealmPrefixedModelIsPriced(t *testing.T) {
	db := newCostTestDB(t)
	insertCostRecord(t, db, history.RequestRecord{
		ID: "realm-1", Model: "global:deepseek-v4.1-flash", Provider: "workbuddy",
		StartTime: time.Now().Add(-time.Minute), InputTokens: 1_000_000, Success: true,
	})

	rows, _, err := NewRequests(db).Query(t.Context(), RequestQuery{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d", len(rows))
	}
	if !rows[0].CostKnown || rows[0].Model != "global:deepseek-v4.1-flash" {
		t.Fatalf("realm-prefixed model lost its identity or price: %+v", rows[0])
	}
	// 1M input at 0.15/M = 0.15.
	if rows[0].CostUSD < 0.149 || rows[0].CostUSD > 0.151 {
		t.Errorf("cost = %v, want ~0.15", rows[0].CostUSD)
	}
}

package storage

import (
	"testing"
	"time"

	"github.com/routatic/proxy/internal/history"
)

// A gateway that fronts several realms names its models `global:<model>`, and
// the catalog stores the bare name once. Without stripping the prefix the
// lookup found nothing and every request through that platform reported an
// unknown cost - a silent failure, since the row still looked well-formed.
func TestModelNameForPricing(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"global:deepseek-v4.1-flash", "deepseek-v4.1-flash"},
		{"cn:deepseek-v4.1-flash", "deepseek-v4.1-flash"},
		{"global:glm-5.3", "glm-5.3"},
		// No prefix: unchanged.
		{"deepseek-v4.1-flash", "deepseek-v4.1-flash"},
		{"deepseek/deepseek-v4-pro", "deepseek/deepseek-v4-pro"},
		// A colon that is part of the name, not a realm, is left alone.
		{"vendor:model:variant", "model:variant"}, // first colon is a plain word, so it is a realm
		{"model-with.dots:tag", "model-with.dots:tag"},
		// Degenerate shapes must not lose the whole name.
		{":leading", ":leading"},
		{"trailing:", "trailing:"},
		{"", ""},
	} {
		if got := modelNameForPricing(tc.in); got != tc.want {
			t.Errorf("modelNameForPricing(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// The end-to-end consequence: a realm-prefixed model stored through Insert must
// carry a cost, because the catalog row for the bare name exists.
func TestRealmPrefixedModelIsPriced(t *testing.T) {
	db := newCostTestDB(t)
	if _, err := db.DB().Exec(`INSERT INTO models (id, provider, name, cost_input_per_m, cost_output_per_m)
		VALUES ('deepseek-v4.1-flash', 'workbuddy', 'deepseek-v4.1-flash', 0.15, 0.6)`); err != nil {
		t.Fatal(err)
	}
	insertCostRecord(t, db, history.RequestRecord{
		ID: "realm-1", Model: "global:deepseek-v4.1-flash", Provider: "workbuddy",
		StartTime: time.Now().Add(-time.Minute), InputTokens: 1_000_000, Success: true,
	})

	rows, _, err := NewRequests(db).Query(RequestQuery{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d", len(rows))
	}
	if !rows[0].CostKnown {
		t.Fatal("a realm-prefixed model was not priced; the catalog row for the bare name exists")
	}
	// 1M input at 0.15/M = 0.15.
	if rows[0].CostUSD < 0.149 || rows[0].CostUSD > 0.151 {
		t.Errorf("cost = %v, want ~0.15", rows[0].CostUSD)
	}
}

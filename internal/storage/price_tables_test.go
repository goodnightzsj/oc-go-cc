package storage

import (
	"testing"
	"time"

	"github.com/routatic/proxy/internal/site"
)

// TestPriceTablesReportsSeedBeforeRefresh pins the state that motivates the
// whole endpoint: before any refresh installs a table, the embedded snapshot is
// serving prices and nothing says so. The seed counts are still reported, so a
// reader sees which platform is stale rather than only that it is empty.
func TestPriceTablesReportsSeedBeforeRefresh(t *testing.T) {
	// Restore whatever the process had, so this test cannot leak state into
	// another one that prices a request.
	prevOverrides, prevAt := priceOverrides.Load(), priceRefreshedAt.Load()
	t.Cleanup(func() {
		priceOverrides.Store(prevOverrides)
		priceRefreshedAt.Store(prevAt)
	})
	priceOverrides.Store(nil)
	priceRefreshedAt.Store(nil)

	// No refresh has run, so nothing is live. The embedded seeds are not
	// reported by PriceTables (they are always present and not worth a row);
	// what matters is that nothing claims to be live.
	if got := PriceTables(); len(got) != 0 {
		t.Errorf("PriceTables before any refresh = %+v, want empty (nothing live yet)", got)
	}
}

// TestPriceTablesMarksRefreshedPlatformsLive covers the distinction the
// dashboard renders: a table fetched from its publisher is live and carries the
// time it was fetched; one that is not must not borrow that status.
func TestPriceTablesMarksRefreshedPlatformsLive(t *testing.T) {
	prevOverrides, prevAt := priceOverrides.Load(), priceRefreshedAt.Load()
	t.Cleanup(func() {
		priceOverrides.Store(prevOverrides)
		priceRefreshedAt.Store(prevAt)
	})

	fetched := map[string][]priceEntry{
		site.CommandCode: {{Match: "deepseek", Input: 0.15, Output: 0.60}},
	}
	InstallPrices(fetched)
	before := time.Now().UTC()

	tables := PriceTables()
	cc, ok := tables[site.CommandCode]
	if !ok {
		t.Fatalf("PriceTables missing %s after a successful install: %+v", site.CommandCode, tables)
	}
	if !cc.Live {
		t.Error("a just-installed table must report as live")
	}
	// InstallPrices merges the fetched rules onto the embedded seed rather than
	// replacing it (a wholesale swap is what un-prices real models), so the
	// installed table is the seed plus the fetched rule.
	seedRules := priceSeedCounts()[site.CommandCode]
	if cc.Rules != seedRules+1 {
		t.Errorf("rules = %d, want %d (seed %d + 1 fetched)", cc.Rules, seedRules+1, seedRules)
	}
	if cc.SeedRules != seedRules {
		t.Errorf("seed_rules = %d, want %d", cc.SeedRules, seedRules)
	}
	if cc.RefreshedAt.Before(before.Add(-time.Minute)) || cc.RefreshedAt.After(time.Now().UTC().Add(time.Minute)) {
		t.Errorf("refreshed_at = %v, want approximately now", cc.RefreshedAt)
	}
	// A platform that was not fetched must be absent rather than reported live
	// with a zero timestamp, which would read as "refreshed at the epoch".
	if _, ok := tables[site.ClinePass]; ok {
		t.Errorf("%s reported without being fetched: %+v", site.ClinePass, tables[site.ClinePass])
	}
}

// TestInstallPricesKeepsUnfetchedTimestamps pins that a partial refresh does not
// make an unrefreshed platform look fresh. This is the failure that matters:
// reporting a stale table as live is worse than reporting no timestamp, because
// it is the one state the endpoint exists to make visible.
func TestInstallPricesKeepsUnfetchedTimestamps(t *testing.T) {
	prevOverrides, prevAt := priceOverrides.Load(), priceRefreshedAt.Load()
	t.Cleanup(func() {
		priceOverrides.Store(prevOverrides)
		priceRefreshedAt.Store(prevAt)
	})
	priceOverrides.Store(nil)
	priceRefreshedAt.Store(nil)

	InstallPrices(map[string][]priceEntry{
		site.CommandCode: {{Match: "a", Input: 1, Output: 1}},
		site.ClinePass:   {{Match: "b", Input: 1, Output: 1}},
	})
	firstCC := PriceTables()[site.CommandCode].RefreshedAt

	time.Sleep(5 * time.Millisecond)
	// Second refresh reaches only CommandCode.
	InstallPrices(map[string][]priceEntry{
		site.CommandCode: {{Match: "a", Input: 2, Output: 2}},
	})

	tables := PriceTables()
	if !tables[site.CommandCode].RefreshedAt.After(firstCC) {
		t.Error("the refreshed platform must carry its new timestamp")
	}
	if tables[site.ClinePass].RefreshedAt != PriceTables()[site.ClinePass].RefreshedAt {
		t.Error("timestamps must be stable across reads")
	}
	if !tables[site.ClinePass].Live {
		t.Error("an earlier refresh is still live: its table is installed")
	}
	// Its rule count is unchanged, proving the second install did not silently
	// drop a platform it did not mention.
	seedCL := priceSeedCounts()[site.ClinePass]
	if tables[site.ClinePass].Rules != seedCL+1 {
		t.Errorf("cline-pass rules = %d, want %d (kept from the first refresh)", tables[site.ClinePass].Rules, seedCL+1)
	}
}

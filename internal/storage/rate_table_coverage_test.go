package storage

import (
	"context"
	"errors"
	"testing"

	"github.com/routatic/proxy/internal/site"
)

// Closing two silent failures an audit found in the pricing path.
//
// A platform's descriptor names its price table in `RateTable`, and the three
// tables are wired by hand in two places: the embedded seed map, and the
// inline refresh branches in RefreshPrices. Neither was checked against the
// registry. A descriptor naming a table that does not exist, or a platform
// added with a published table that nobody wired into the refresh, produced no
// error at all - its rows fell through to the catalog-rate path or reported as
// unknown, and both render as a well-formed dashboard number.
//
// That is the failure the price-refresh machinery was written in response to.
// pricerefresh.go's own header records that the build-time snapshot "proved to
// be wrong within weeks ... and nothing reported it - the dashboard simply
// showed well-formed numbers computed from retired rates". This is that shape,
// so the two assertions below make it a failing test instead.

// TestRateTableFilesCoversRegistry. Every non-empty RateTable a descriptor names
// must have an embedded table, and every embedded table must belong to a
// registered platform. A stray key is dead weight; a missing one is traffic
// priced from the wrong source.
func TestRateTableFilesCoversRegistry(t *testing.T) {
	known := make(map[string]bool, len(site.All()))
	for _, d := range site.All() {
		known[d.ID] = true
		if d.RateTable == "" {
			// Publishing no per-token prices is a deliberate state; see the
			// field's comment. Not an omission.
			continue
		}
		if _, ok := rateTableFiles[d.RateTable]; !ok {
			t.Errorf("platform %q names rate table %q, which has no embedded seed; its models would price from the catalog path or report as unknown",
				d.ID, d.RateTable)
		}
	}
	for name := range rateTableFiles {
		if !known[name] {
			t.Errorf("embedded rate table %q belongs to no registered platform", name)
		}
	}
}

// TestRefreshPricesReachesEveryRateTable. RefreshPrices wires its sources as
// inline branches rather than driven by the registry, so a platform with a
// published table can be added to the descriptor list and the seed map without
// ever being refreshed. Its prices then age at whatever the binary was built
// with, and PriceTables() does not report it as stale either - that list only
// covers platforms present in priceOverrides, so the platform is simply absent.
//
// The check is about coverage, not fetching, so both sources are made to fail:
// RefreshPrices reports a failure per platform it attempts, which makes the
// returned error map a precise record of what it visited. A rate table missing
// from that map is one the refresh never tries.
func TestRefreshPricesReachesEveryRateTable(t *testing.T) {
	// A closed server: both HTTP sources fail, and each failure names its
	// platform. OpenCode Go's source is the local catalog, failed via its hook.
	_, errs := RefreshPrices(context.Background(), PriceRefreshConfig{
		CatalogPath: t.TempDir(),
		RefreshCatalog: func(context.Context) error {
			return errors.New("catalog unavailable")
		},
		CommandCodeURL: "http://127.0.0.1:1/pro",
		ClinePassURL:   "http://127.0.0.1:1/clinepass",
	}, nil)

	for _, d := range site.All() {
		if d.RateTable == "" {
			continue
		}
		if _, ok := errs[d.RateTable]; !ok {
			t.Errorf("platform %q publishes rate table %q but RefreshPrices never attempts it, so its prices would age silently - add a branch and a PriceRefreshConfig URL for it",
				d.ID, d.RateTable)
		}
	}
}

package storage

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/site"
)

// Prices are refreshed from the platforms' pages, so a page that changes shape
// must fail loudly rather than install an empty or partial table: a wrong table
// produces well-formed numbers that are silently wrong, which is exactly the
// failure this whole path exists to end.
func TestRefreshPricesInstallsBothTables(t *testing.T) {
	catalog := writeTempCatalog(t, `{"providers":{"opencode-go":{"models":{
		"deepseek-v4-flash":{"cost":{"input":0.15,"output":0.60,"cache_read":0.003}},
		"qwen3.7-plus":{"cost":{"input":0.40,"output":1.60,"cache_read":0.04,
			"tiers":[{"input":1.20,"output":4.80,"cache_read":0.12,"tier":{"type":"context","size":256000}}]}}
	}}}}`)

	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/cline") {
			_, _ = w.Write([]byte(clinePricePageFixture))
			return
		}
		_, _ = w.Write([]byte(commandCodePage(t, `[
			{"id":"deepseek/deepseek-v4-flash","inputCost":0.15,"outputCost":0.60,"cacheReadCost":0.003,
			 "tiers":[{"context":"≤ 256K","rates":{"input":0.15,"output":0.60,"cacheRead":0.003}}]},
			{"id":"xai/grok-4.6","inputCost":2,"outputCost":6,"cacheReadCost":0.5,
			 "tiers":[{"context":"≤ 200K","rates":{"input":2,"output":6,"cacheRead":0.5}},
			          {"context":"> 200K","rates":{"input":4,"output":12,"cacheRead":1}}]}
		]`)))
	}))
	defer page.Close()

	resetPrices(t)
	fetched, errs := RefreshPrices(context.Background(), PriceRefreshConfig{
		CatalogPath:    catalog,
		CommandCodeURL: page.URL,
		ClinePassURL:   page.URL + "/cline",
	}, page.Client())

	if len(errs) != 0 {
		t.Fatalf("refresh reported errors: %v", errs)
	}
	if got := len(fetched[site.OpenCodeGo]); got != 2 {
		t.Errorf("opencode-go rules = %d, want 2", got)
	}
	if got := len(fetched[site.CommandCode]); got != 2 {
		t.Errorf("commandcode rules = %d, want 2", got)
	}

	// The refreshed tables must actually be what prices now.
	in, out, _, _, ok := PriceForProviderModel(site.OpenCodeGo, "deepseek-v4-flash", 0)
	if !ok || in != 0.15 || out != 0.60 {
		t.Errorf("opencode-go price after refresh = %v/%v ok=%v, want 0.15/0.60", in, out, ok)
	}
	if _, _, _, _, ok := PriceForProviderModel(site.CommandCode, "grok-4.6", 0); !ok {
		t.Error("commandcode grok-4.6 must be priced after refresh")
	}
	// A tier from the refreshed page must be live too.
	_, _, _, _, _ = PriceForProviderModel(site.CommandCode, "grok-4.6", 300_000)
	if got := CurrentPrices()[site.CommandCode]; got != 2 {
		t.Errorf("CurrentPrices report = %d, want 2", got)
	}
}

// A failure on one platform must not clear the other platform's table, and must
// leave the previously installed one in place.
func TestRefreshKeepsGoodTablesWhenOnePlatformFails(t *testing.T) {
	catalog := writeTempCatalog(t, `{"providers":{"opencode-go":{"models":{
		"deepseek-v4-flash":{"cost":{"input":0.15,"output":0.60}}
	}}}}`)
	resetPrices(t)

	ok1, errs1 := RefreshPrices(context.Background(), PriceRefreshConfig{
		CatalogPath:    catalog,
		CommandCodeURL: "http://127.0.0.1:1/nope",     // nothing listens here
		ClinePassURL:   "http://127.0.0.1:1/nope-too", // nor here
	}, &http.Client{Timeout: 2 * time.Second})
	if len(errs1) != 2 || errs1[site.CommandCode] == nil || errs1[site.ClinePass] == nil {
		t.Fatalf("want commandcode and cline-pass errors, got %v", errs1)
	}
	if len(ok1[site.OpenCodeGo]) == 0 {
		t.Fatal("opencode-go must still refresh when commandcode fails")
	}
	if _, _, _, _, ok := PriceForProviderModel(site.OpenCodeGo, "deepseek-v4-flash", 0); !ok {
		t.Error("opencode-go must remain priced after a half-failed refresh")
	}

	// Second round: the catalog turns bad, commandcode stays down. The good
	// opencode-go table from round one must survive rather than be replaced by
	// an empty result.
	bad := writeTempCatalog(t, `{"providers":{}}`)
	_, errs2 := RefreshPrices(context.Background(), PriceRefreshConfig{
		CatalogPath:    bad,
		CommandCodeURL: "http://127.0.0.1:1/nope",
		ClinePassURL:   "http://127.0.0.1:1/nope-too",
	}, &http.Client{Timeout: 2 * time.Second})
	if errs2[site.OpenCodeGo] == nil {
		t.Error("a catalog without the provider must be an error, not an empty table")
	}
	if _, _, _, _, ok := PriceForProviderModel(site.OpenCodeGo, "deepseek-v4-flash", 0); !ok {
		t.Error("opencode-go must keep its last good table after a failed refresh")
	}
}

// An empty or malformed page is an error, never a silently-installed empty
// table that would turn every cost into "unknown" or, worse, free.
func TestPriceFetchersRejectEmptyResults(t *testing.T) {
	resetPrices(t)

	if _, err := FetchOpenCodeGoPrices(""); err == nil {
		t.Error("an unset catalog path must be an error")
	}
	if _, err := FetchOpenCodeGoPrices(filepath.Join(t.TempDir(), "absent.json")); err == nil {
		t.Error("a missing catalog file must be an error")
	}
	if _, err := FetchCommandCodePrices(context.Background(), nil, "http://127.0.0.1:1/nope"); err == nil {
		t.Error("an unreachable price page must be an error")
	}

	empty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html>no payload here</html>"))
	}))
	defer empty.Close()
	if _, err := FetchCommandCodePrices(context.Background(), empty.Client(), empty.URL); err == nil {
		t.Error("a page with no model payload must be an error, not an empty table")
	}
}

func TestPriceRefreshLoopStopsBeforeInstallingAfterCancellation(t *testing.T) {
	resetPrices(t)
	path := writeTempCatalog(t, `{"providers":{"opencode-go":{"models":{"cancelled":{"cost":{"input":9,"output":9}}}}}}`)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	PriceRefreshLoop(ctx, PriceRefreshConfig{CatalogPath: path}, nil, nil)
	if len(CurrentPrices()) != 0 {
		t.Fatal("cancelled price refresh must not install a new table")
	}
}

func TestRefreshPricesSyncsCatalogAndIsolatesSyncFailure(t *testing.T) {
	resetPrices(t)
	path := writeTempCatalog(t, `{"providers":{"opencode-go":{"models":{"fresh-model":{"cost":{"input":9,"output":9}}}}}}`)
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/cline") {
			_, _ = w.Write([]byte(clinePricePageFixture))
			return
		}
		_, _ = w.Write([]byte(commandCodePage(t, `[{"id":"fresh-model","inputCost":0.2,"outputCost":0.8}]`)))
	}))
	defer page.Close()
	var syncErr error
	cfg := PriceRefreshConfig{
		CatalogPath: path, CommandCodeURL: page.URL, ClinePassURL: page.URL + "/cline",
		RefreshCatalog: func(ctx context.Context) error {
			if syncErr != nil {
				return syncErr
			}
			return os.WriteFile(path, []byte(`{"providers":{"opencode-go":{"models":{"fresh-model":{"cost":{"input":0.15,"output":0.6}}}}}}`), 0600)
		},
	}
	if _, errs := RefreshPrices(context.Background(), cfg, page.Client()); len(errs) != 0 {
		t.Fatal(errs)
	}
	assertGoPrice := func() {
		t.Helper()
		in, out, _, _, ok := PriceForProviderModel(site.OpenCodeGo, "fresh-model", 0)
		if !ok || in != 0.15 || out != 0.6 {
			t.Fatalf("price = %v/%v (known=%v), want synced 0.15/0.6", in, out, ok)
		}
	}
	assertGoPrice()
	// A failed sync must not label an old file as a successful price refresh.
	syncErr = errors.New("synthetic catalog sync failure")
	if err := os.WriteFile(path, []byte(`{"providers":{"opencode-go":{"models":{"fresh-model":{"cost":{"input":9,"output":9}}}}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	fetched, errs := RefreshPrices(context.Background(), cfg, page.Client())
	if !errors.Is(errs[site.OpenCodeGo], syncErr) || len(fetched[site.OpenCodeGo]) != 0 || len(fetched[site.CommandCode]) != 1 {
		t.Fatalf("per-platform refresh outcome = %v / %v", fetched, errs)
	}
	assertGoPrice()
}

func TestPriceRefreshLoopRepeatsAndStops(t *testing.T) {
	resetPrices(t)
	path := writeTempCatalog(t, `{"providers":{"opencode-go":{"models":{"loop-model":{"cost":{"input":0.15,"output":0.6}}}}}}`)
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/cline") {
			_, _ = w.Write([]byte(clinePricePageFixture))
			return
		}
		_, _ = w.Write([]byte(commandCodePage(t, `[{"id":"loop-model","inputCost":0.2,"outputCost":0.8}]`)))
	}))
	defer page.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	preparations, rounds := 0, 0
	PriceRefreshLoop(ctx, PriceRefreshConfig{
		CatalogPath: path, CommandCodeURL: page.URL, ClinePassURL: page.URL + "/cline", Interval: time.Millisecond,
		RefreshCatalog: func(context.Context) error { preparations++; return nil },
	}, page.Client(), func(counts map[string]int, errs map[string]error) {
		if len(errs) != 0 || counts[site.OpenCodeGo] != 1 || counts[site.CommandCode] != 1 || counts[site.ClinePass] == 0 {
			t.Errorf("refresh result: %v / %v", counts, errs)
		}
		rounds++
		if rounds == 2 {
			cancel()
		}
	})
	if preparations != 2 || rounds != 2 {
		t.Fatalf("loop prepared %d catalogs and reported %d rounds, want 2 before cancellation", preparations, rounds)
	}
}

// The band label wording is the page's, and getting it backwards would price
// every request at the long-context rate. "≤ 256K" is a ceiling; the band that
// starts above it is the *next* row, whose own "> 256K" label is not a bound.
func TestTierBoundUsesTheCeilingLabel(t *testing.T) {
	for _, c := range []struct {
		label string
		want  int64
		ok    bool
	}{
		{"≤ 256K", 256 * 1024, true},
		{"≤ 512K", 512 * 1024, true},
		{"≤ 1M", 1024 * 1024, true},
		{"> 256K", 0, false},
		{"> 200K", 0, false},
		{"", 0, false},
	} {
		got, ok := tierBoundFromLabel(c.label)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("tierBoundFromLabel(%q) = %d,%v; want %d,%v", c.label, got, ok, c.want, c.ok)
		}
	}
}

// — helpers —

// resetPrices clears installed overrides so tests do not leak into each other.
func resetPrices(t *testing.T) {
	t.Helper()
	prev := priceOverrides.Load()
	empty := map[string][]priceEntry{}
	priceOverrides.Store(&empty)
	t.Cleanup(func() {
		if prev != nil {
			priceOverrides.Store(prev)
		} else {
			priceOverrides.Store(nil)
		}
	})
}

func writeTempCatalog(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "catalog.json")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// commandCodePage wraps a models array in the flight-chunk envelope the real
// page uses, so the test exercises the same reassembly path: each chunk is a
// JSON string literal whose decoded value is a slice of flight protocol text.
func commandCodePage(t *testing.T, modelsArray string) string {
	t.Helper()
	payload := `b:["$","$L35",null,{"models":` + modelsArray + `}]`
	literals := []string{`10:"$Sreact.fragment"`, `11:I[61189,[],""]`, payload}
	encoded, err := json.Marshal(strings.Join(literals, "\n"))
	if err != nil {
		t.Fatal(err)
	}
	return `<script>self.__next_f.push([1,` + string(encoded) + `])</script>`
}

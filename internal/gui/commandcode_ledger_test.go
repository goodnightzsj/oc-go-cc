package gui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/history"
	"github.com/routatic/proxy/internal/storage"
)

// The account's official figures cover every request on the account, including
// traffic that never reached this proxy, so they cannot be compared against the
// local ledger unless both sides are scoped to the same period. This pins the
// window (the subscription period the account reports, not a calendar month)
// and the platform filter.
func TestCommandCodeLedgerScopesToTheSubscriptionPeriod(t *testing.T) {
	srv, db := commandCodeLedgerServer(t, `["synthetic-command-key"]`)
	period := func(offset time.Duration) time.Time {
		return time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC).Add(offset)
	}
	for _, rec := range []history.RequestRecord{
		{ID: "in-period", Provider: "commandcode", Model: "deepseek/deepseek-v4-flash", StartTime: period(0), CostUSD: 1.25, CostKnown: true, CostSource: "estimated"},
		{ID: "before-period", Provider: "commandcode", Model: "deepseek/deepseek-v4-flash", StartTime: period(-30 * 24 * time.Hour), CostUSD: 99, CostKnown: true, CostSource: "estimated"},
		{ID: "other-platform", Provider: "opencode-go", Model: "deepseek-v4-flash", StartTime: period(0), CostUSD: 77, CostKnown: true, CostSource: "estimated"},
	} {
		if err := storage.NewRequests(db).Insert(rec); err != nil {
			t.Fatalf("insert %s: %v", rec.ID, err)
		}
	}

	ledger := commandCodeLedger(t, srv)
	if ledger["requests"] != float64(1) {
		t.Errorf("ledger requests = %v, want 1 (only in-period commandcode rows)", ledger["requests"])
	}
	if got := ledger["cost_usd"].(float64); got != 1.25 {
		t.Errorf("ledger cost = %v, want 1.25", got)
	}
	if ledger["known_requests"] != float64(1) {
		t.Errorf("ledger known_requests = %v, want 1", ledger["known_requests"])
	}
	if ledger["unknown_cost_requests"] != float64(0) {
		t.Errorf("ledger unknown_cost_requests = %v, want 0", ledger["unknown_cost_requests"])
	}
}

func TestCommandCodeLedgerPreservesUnknownCost(t *testing.T) {
	srv, db := commandCodeLedgerServer(t, `["synthetic-command-key"]`)
	err := storage.NewRequests(db).Insert(history.RequestRecord{
		ID: "unknown-cost", Provider: "commandcode", Model: "synthetic-unpriced-model",
		StartTime:   time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC),
		InputTokens: 1000, OutputTokens: 100, Success: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	ledger := commandCodeLedger(t, srv)
	if ledger["requests"] != float64(1) || ledger["known_requests"] != float64(1) ||
		ledger["unknown_cost_requests"] != float64(1) || ledger["cost_usd"] != float64(0) {
		t.Fatalf("unknown cost lost its coverage while preserving known request status: %+v", ledger)
	}
}

// Two keys share one local ledger because stored rows carry no key identity, so
// attributing a single account's window to it would be a guess. The block stays
// absent rather than showing a number that looks like a reconciliation result.
func TestCommandCodeLedgerRefusesToAttributeAMultiKeyPool(t *testing.T) {
	srv, _ := commandCodeLedgerServer(t, `["synthetic-command-a","synthetic-command-b"]`)
	if _, ok := quotaAccounts(t, srv)[0]["ledger"]; ok {
		t.Fatal("a multi-key pool must not present per-account local totals")
	}
}

// commandCodeLedgerServer builds a single-account CommandCode server whose
// subscription runs 2026-09-10 to 2026-10-10, backed by a temp database.
func commandCodeLedgerServer(t *testing.T, keysJSON string) (*Server, *storage.Database) {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/gateway/alpha/billing/credits":
			_, _ = fmt.Fprint(w, `{"credits":{"freeCredits":0,"monthlyCredits":60,"purchasedCredits":0}}`)
		case "/gateway/alpha/billing/subscriptions":
			_, _ = fmt.Fprint(w, `{"success":true,"data":{"planId":"individual-goat","status":"active","currentPeriodStart":"2026-09-10T07:35:57.000Z","currentPeriodEnd":"2026-10-10T07:35:57.000Z"}}`)
		case "/gateway/alpha/usage/summary":
			_, _ = fmt.Fprint(w, `{"totalCount":9,"totalCost":8.5,"completedCount":9,"failedCount":0,"totalTokensIn":10,"totalTokensOut":2,"totalTokens":12,"totalCredits":8.5,"periodBasis":"billing-period"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(upstream.Close)

	cfg := fmt.Sprintf(`{"api_key":"synthetic-global","commandcode":{"base_url":%q,"api_keys":%s}}`,
		upstream.URL+"/gateway/provider/v1/chat/completions", keysJSON)
	srv, _ := configTestServer(t, cfg)
	db, err := storage.Open(storage.Config{DatabasePath: filepath.Join(t.TempDir(), "ledger.db"), WALEnabled: false})
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	srv.storage = db
	return srv, db
}

func quotaAccounts(t *testing.T, srv *Server) []map[string]any {
	t.Helper()
	rec := httptest.NewRecorder()
	srv.handleQuota(rec, httptest.NewRequest(http.MethodGet, "/api/quota?provider=commandcode", nil))
	var result struct {
		Accounts []map[string]any `json:"accounts"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &result) != nil {
		t.Fatalf("quota response: HTTP %d", rec.Code)
	}
	if len(result.Accounts) == 0 {
		t.Fatal("no accounts in the quota response")
	}
	return result.Accounts
}

func commandCodeLedger(t *testing.T, srv *Server) map[string]any {
	t.Helper()
	ledger, ok := quotaAccounts(t, srv)[0]["ledger"].(map[string]any)
	if !ok {
		t.Fatal("account carries no ledger block")
	}
	return ledger
}

package gui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/history"
	"github.com/routatic/proxy/internal/storage"
)

func TestQuotaLocalUsageKeepsUnknownCostsAndAccountScope(t *testing.T) {
	for _, mode := range []string{"unknown-cost", "multiple-keys", "storage-error"} {
		t.Run(mode, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/docs" {
					_, _ = w.Write([]byte(docsFixture))
					return
				}
				_, _ = w.Write([]byte(`{"monthly":{"usedPercent":50,"usedDollars":30,"resetsAt":"2026-09-15T00:00:00Z"}}`))
			}))
			defer upstream.Close()
			db, err := storage.Open(storage.Config{DatabasePath: filepath.Join(t.TempDir(), "quota.db")})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			for _, id := range []string{"known", "unknown"} {
				err := storage.NewRequests(db).Insert(history.RequestRecord{
					ID: id, Provider: "opencode-go", Model: "glm-5.2", StartTime: time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC),
					CostKnown: true, CostUSD: 2, CostSource: storage.CostSourceEstimated,
				})
				if err != nil {
					t.Fatal(err)
				}
			}
			if _, err := db.DB().Exec(`UPDATE requests SET cost_usd = NULL WHERE id = ?`, "unknown"); err != nil {
				t.Fatal(err)
			}
			keys := []string{"synthetic-key-a"}
			if mode == "multiple-keys" {
				keys = append(keys, "synthetic-key-b")
			}
			if mode == "storage-error" {
				if err := db.Close(); err != nil {
					t.Fatal(err)
				}
			}
			srv := quotaTestServer(t, upstream.URL, keys...)
			srv.modelLimitsURL = []string{upstream.URL + "/docs"}
			srv.storage = db
			rec := httptest.NewRecorder()
			srv.handleQuota(rec, httptest.NewRequest(http.MethodGet, "/api/quota", nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("quota status = %d: %s", rec.Code, rec.Body)
			}
			var response struct {
				ModelUsage      []map[string]any `json:"model_usage"`
				ModelUsageError string           `json:"model_usage_error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "unknown-cost":
				if len(response.ModelUsage) != 1 {
					t.Fatalf("local model rows = %d, want 1", len(response.ModelUsage))
				}
				row := response.ModelUsage[0]
				if row["unknown_cost_requests"] != float64(1) || row["requests"] != float64(2) || row["used_usd"] != float64(2) || row["percent"] != nil {
					t.Errorf("unknown cost was calibrated or treated as free: %v", row)
				}
			case "multiple-keys":
				if len(response.ModelUsage) != 0 {
					t.Error("unattributed multi-key history must not be presented as per-account usage")
				}
			case "storage-error":
				if response.ModelUsageError == "" {
					t.Error("a failed ledger query must be reported, not silently replaced by an empty model table")
				}
			}
		})
	}
}

func TestQuotaUnsupportedAccountAPIWithoutSendingGoKeys(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("unsupported platform quota must not send a key to OpenCode Go")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer upstream.Close()
	srv := quotaTestServer(t, upstream.URL, "synthetic-go-key")
	rec := httptest.NewRecorder()
	srv.handleQuota(rec, httptest.NewRequest(http.MethodGet, "/api/quota?provider=commandcode", nil))
	var response struct {
		Provider string `json:"provider"`
		Source   string `json:"source"`
		Reason   string `json:"reason"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &response) != nil {
		t.Fatalf("platform capability status = %d: %s", rec.Code, rec.Body.String())
	}
	if response.Provider != "commandcode" || response.Source != "none" || response.Reason != "no_public_account_api" {
		t.Fatal("CommandCode was misrepresented as a Go quota response")
	}
}

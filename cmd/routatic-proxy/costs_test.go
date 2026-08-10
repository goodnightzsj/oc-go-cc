package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/history"
	"github.com/routatic/proxy/internal/storage"
)

func TestRunCostsReconcileDryRunAndApply(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "data.db")
	configPath := writeTestConfigWithDB(t, tmp, dbPath)
	completedAt := time.Date(2026, 8, 6, 7, 0, 0, 0, time.UTC)
	startedAt := completedAt.Add(-1662 * time.Millisecond)

	db, err := storage.Open(storage.Config{DatabasePath: dbPath})
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	if err := storage.NewRequests(db).Insert(history.RequestRecord{
		ID: "request-1", Model: "deepseek-v4-flash", StartTime: startedAt, Duration: 2705 * time.Millisecond,
		CacheCreationTokens: 10, OutputTokens: 2, CacheReadTokens: 30, Success: true,
	}); err != nil {
		t.Fatalf("insert request: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close storage: %v", err)
	}

	inputPath := filepath.Join(tmp, "usage.json")
	capture := providerCostCapture{Rows: []storage.ProviderCostRecord{{
		Time: completedAt, Model: "deepseek-v4-flash",
		InputTokens: 10, OutputTokens: 2, CacheReadTokens: 30,
		ProviderCostUnits: 1234,
	}}}
	data, err := json.Marshal(capture)
	if err != nil {
		t.Fatalf("marshal capture: %v", err)
	}
	if err := os.WriteFile(inputPath, data, 0600); err != nil {
		t.Fatalf("write capture: %v", err)
	}

	cmd, output := newCaptureCommand(t)
	if err := runCostsReconcile(cmd, configPath, inputPath, false); err != nil {
		t.Fatalf("dry run: %v", err)
	}
	var report storage.ProviderCostReport
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatalf("decode dry-run report: %v", err)
	}
	if report.Exact != 1 || report.WouldUpdate != 1 || report.Updated != 0 {
		t.Fatalf("unexpected dry-run report: %+v", report)
	}

	cmd, output = newCaptureCommand(t)
	if err := runCostsReconcile(cmd, configPath, inputPath, true); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatalf("decode apply report: %v", err)
	}
	if report.Updated != 1 {
		t.Fatalf("updated = %d, want 1", report.Updated)
	}

	db, err = storage.Open(storage.Config{DatabasePath: dbPath})
	if err != nil {
		t.Fatalf("reopen storage: %v", err)
	}
	defer func() { _ = db.Close() }()
	var source string
	if err := db.DB().QueryRowContext(context.Background(), `SELECT cost_source FROM requests WHERE id = 'request-1'`).Scan(&source); err != nil {
		t.Fatalf("read source: %v", err)
	}
	if source != storage.CostSourceProvider {
		t.Fatalf("cost source = %q, want %q", source, storage.CostSourceProvider)
	}
}

func TestRunCostsImportReplacesProviderUsage(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "data.db")
	configPath := writeTestConfigWithDB(t, tmp, dbPath)
	capturedAt := time.Now().UTC().Truncate(time.Second)
	inputPath := filepath.Join(tmp, "usage.json")
	capture := providerCostCapture{
		CapturedAt: capturedAt,
		Rows: []storage.ProviderCostRecord{{
			Time: capturedAt.Add(-time.Minute), Model: "deepseek-v4-flash",
			Provider: "inf-go.oa-compat", Plan: "lite", InputTokens: 10,
			OutputTokens: 2, CacheReadTokens: 30, ProviderCostUnits: 1234,
		}},
	}
	data, err := json.Marshal(capture)
	if err != nil {
		t.Fatalf("marshal capture: %v", err)
	}
	if err := os.WriteFile(inputPath, data, 0600); err != nil {
		t.Fatalf("write capture: %v", err)
	}
	cmd, output := newCaptureCommand(t)
	if err := runCostsImport(cmd, configPath, inputPath); err != nil {
		t.Fatalf("import: %v", err)
	}
	var summary storage.ProviderUsageSummary
	if err := json.Unmarshal(output.Bytes(), &summary); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	if summary.TotalRequests != 1 || summary.CostUSD != 0.00001234 || summary.Provider != "inf-go.oa-compat" {
		t.Fatalf("unexpected imported summary: %+v", summary)
	}
}

func TestRunCostsSyncRequestsUsesPersistedSnapshot(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "data.db")
	configPath := writeTestConfigWithDB(t, tmp, dbPath)
	db, err := storage.Open(storage.Config{DatabasePath: dbPath})
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	if err := db.ReplaceProviderUsage(context.Background(), now.Add(time.Minute), []storage.ProviderCostRecord{{
		Time: now, Model: "model-a", Provider: "platform-a", InputTokens: 10, ProviderCostUnits: 1234,
	}}); err != nil {
		t.Fatalf("replace provider usage: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close storage: %v", err)
	}

	cmd, output := newCaptureCommand(t)
	if err := runCostsSyncRequests(cmd, configPath, false); err != nil {
		t.Fatalf("dry-run sync: %v", err)
	}
	var report storage.ProviderRequestSyncReport
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatalf("decode sync report: %v", err)
	}
	if report.SnapshotRows != 1 || report.WouldInsert != 1 || report.Inserted != 0 {
		t.Fatalf("unexpected sync report: %+v", report)
	}
}

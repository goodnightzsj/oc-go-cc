package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/storage"
)

func TestCostsReconcileRequiresExplicitSupportedProvider(t *testing.T) {
	for _, args := range [][]string{
		{"--input", "missing.json"},
		{"--input", "missing.json", "--provider", ""},
		{"--input", "missing.json", "--provider", "unsupported"},
	} {
		cmd := costsReconcileCmd()
		cmd.SetArgs(args)
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "provider") {
			t.Errorf("args=%v: error=%v, want provider validation before opening config or input", args, err)
		}
	}
}

func TestRunCostsSyncRequestsReportsAmbiguity(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "data.db")
	configPath := writeTestConfigWithDB(t, dir, dbPath)
	db, err := storage.Open(storage.Config{DatabasePath: dbPath})
	if err != nil {
		t.Fatal(err)
	}
	when := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	row := storage.ProviderCostRecord{Time: when, Model: "review-model", Provider: "inf-go.oa-compat", InputTokens: 1, ProviderCostUSD: 1}
	if err := db.ReplaceProviderUsage(context.Background(), time.Now().UTC(), []storage.ProviderCostRecord{row, row}); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	cmd, output := newCaptureCommand(t)
	var warnings bytes.Buffer
	cmd.SetErr(&warnings)
	if err := runCostsSyncRequests(cmd, configPath, true); !errors.Is(err, storage.ErrAmbiguousProviderCosts) {
		t.Fatalf("sync error=%v, want ambiguous snapshot", err)
	}
	var report storage.ProviderRequestSyncReport
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatalf("failed sync omitted its JSON diagnosis: %v", err)
	}
	if report.TargetProvider != "opencode-go" || report.Ambiguous != 2 || report.Inserted != 0 || report.Removed != 0 {
		t.Fatalf("unexpected failed sync report: %+v", report)
	}
	if !strings.Contains(warnings.String(), "OpenCode Go") || !strings.Contains(warnings.String(), "cross-account") {
		t.Fatalf("missing snapshot-scope warning: %s", warnings.String())
	}
}

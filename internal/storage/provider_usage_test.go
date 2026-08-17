package storage

import (
	"context"
	"math"
	"path/filepath"
	"testing"
	"time"
)

func TestProviderUsageSnapshotAnalyticsAndReplacement(t *testing.T) {
	db, err := Open(Config{DatabasePath: filepath.Join(t.TempDir(), "provider-usage.db")})
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	defer func() { _ = db.Close() }()
	now := time.Now().UTC().Truncate(time.Second)
	rows := []ProviderCostRecord{
		{Time: now.Add(-2 * time.Hour), Model: "model-a", Provider: "platform-a", Plan: "lite", InputTokens: 10, OutputTokens: 2, ReasoningTokens: 1, CacheReadTokens: 100, ProviderCostUnits: 12_345_678},
		{Time: now.Add(-time.Hour), Model: "model-a", Provider: "platform-a", Plan: "lite", InputTokens: 20, OutputTokens: 3, CacheWrite5mTokens: 7, ProviderCostUnits: 10_000_000},
		{Time: now, Model: "model-b", Provider: "platform-b", Plan: "pro", InputTokens: 30, OutputTokens: 4, CacheWrite1hTokens: 8, ProviderCostUnits: 20_000_000},
	}
	if err := db.ReplaceProviderUsage(context.Background(), now, rows); err != nil {
		t.Fatalf("replace provider usage: %v", err)
	}
	got, err := db.GetProviderUsageAnalytics(context.Background(), 30)
	if err != nil {
		t.Fatalf("get analytics: %v", err)
	}
	if got.Summary.TotalRequests != 3 || got.Summary.InputTokens != 60 || got.Summary.CacheCreationTokens != 15 {
		t.Fatalf("unexpected summary: %+v", got.Summary)
	}
	if math.Abs(got.Summary.CostUSD-0.42345678) > 1e-12 {
		t.Fatalf("cost = %.8f, want 0.42345678", got.Summary.CostUSD)
	}
	if got.Summary.ReasoningTokens != 1 {
		t.Fatalf("reasoning tokens = %d, want 1", got.Summary.ReasoningTokens)
	}
	if err := db.ReplaceProviderUsage(context.Background(), now.Add(time.Minute), rows[:1]); err != nil {
		t.Fatalf("replace second snapshot: %v", err)
	}
	got, err = db.GetProviderUsageAnalytics(context.Background(), 0)
	if err != nil {
		t.Fatalf("get replacement: %v", err)
	}
	if got.Summary.TotalRequests != 1 || got.Summary.CostUSD != 0.12345678 {
		t.Fatalf("replacement retained stale rows: %+v", got.Summary)
	}
}

func TestProviderUsageSnapshotRejectsEmptyReplacement(t *testing.T) {
	db, err := Open(Config{DatabasePath: filepath.Join(t.TempDir(), "provider-usage.db")})
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	defer func() { _ = db.Close() }()
	now := time.Now().UTC().Truncate(time.Second)
	row := ProviderCostRecord{Time: now, Model: "model-a", ProviderCostUnits: 1}
	if err := db.ReplaceProviderUsage(context.Background(), now, []ProviderCostRecord{row}); err != nil {
		t.Fatalf("seed provider usage: %v", err)
	}
	if err := db.ReplaceProviderUsage(context.Background(), now.Add(time.Minute), nil); err == nil {
		t.Fatal("empty replacement succeeded")
	}
	got, err := db.GetProviderUsageAnalytics(context.Background(), 0)
	if err != nil {
		t.Fatalf("get provider usage: %v", err)
	}
	if got.Summary.TotalRequests != 1 {
		t.Fatalf("empty replacement removed existing snapshot: %+v", got.Summary)
	}
}

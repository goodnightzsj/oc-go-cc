package storage

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"
)

// ProviderUsageSummary describes the account-wide usage population imported
// from OpenCode. It is deliberately separate from request analytics: account
// rows may include other clients and cannot be joined to proxy request IDs.
type ProviderUsageSummary struct {
	TotalRequests       int64     `json:"total_requests"`
	InputTokens         int64     `json:"input_tokens"`
	OutputTokens        int64     `json:"output_tokens"`
	ReasoningTokens     int64     `json:"reasoning_tokens"`
	CacheReadTokens     int64     `json:"cache_read_tokens"`
	CacheCreationTokens int64     `json:"cache_creation_tokens"`
	CostUSD             float64   `json:"cost_usd"`
	SnapshotAt          time.Time `json:"snapshot_at"`
	ObservedAt          time.Time `json:"observed_at"`
	Provider            string    `json:"provider,omitempty"`
	Plan                string    `json:"plan,omitempty"`
}

type ProviderUsageBreakdown struct {
	Name                string  `json:"name"`
	Model               string  `json:"model,omitempty"`
	Provider            string  `json:"provider,omitempty"`
	Plan                string  `json:"plan,omitempty"`
	Requests            int64   `json:"requests"`
	InputTokens         int64   `json:"input_tokens"`
	OutputTokens        int64   `json:"output_tokens"`
	ReasoningTokens     int64   `json:"reasoning_tokens"`
	CacheReadTokens     int64   `json:"cache_read_tokens"`
	CacheCreationTokens int64   `json:"cache_creation_tokens"`
	CostUSD             float64 `json:"cost_usd"`
}

func (b ProviderUsageBreakdown) TotalTokens() int64 {
	return b.InputTokens + b.OutputTokens + b.CacheReadTokens + b.CacheCreationTokens
}

type ProviderUsageDailyPoint struct {
	Date                string  `json:"date"`
	Requests            int64   `json:"requests"`
	InputTokens         int64   `json:"input_tokens"`
	OutputTokens        int64   `json:"output_tokens"`
	ReasoningTokens     int64   `json:"reasoning_tokens"`
	CacheReadTokens     int64   `json:"cache_read_tokens"`
	CacheCreationTokens int64   `json:"cache_creation_tokens"`
	CostUSD             float64 `json:"cost_usd"`
}

type ProviderUsageAnalytics struct {
	Summary   ProviderUsageSummary      `json:"summary"`
	Models    []ProviderUsageBreakdown  `json:"models"`
	Providers []ProviderUsageBreakdown  `json:"providers"`
	Plans     []ProviderUsageBreakdown  `json:"plans"`
	Trend     []ProviderUsageDailyPoint `json:"trend"`
	Recent    []ProviderCostRecord      `json:"recent"`
}

// ReplaceProviderUsage atomically replaces the last sanitized account
// snapshot. A snapshot is the only supported write mode so stale rows cannot
// be mistaken for current account billing.
func (d *Database) ReplaceProviderUsage(ctx context.Context, capturedAt time.Time, rows []ProviderCostRecord) error {
	if capturedAt.IsZero() {
		return fmt.Errorf("captured_at is required")
	}
	if len(rows) == 0 {
		return fmt.Errorf("provider usage snapshot is empty")
	}
	for i, row := range rows {
		if err := validateProviderCostRecord(row); err != nil {
			return fmt.Errorf("provider usage row %d: %w", i+1, err)
		}
	}
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM provider_usage`); err != nil {
		_ = tx.Rollback()
		return err
	}
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO provider_usage (
			snapshot_at, observed_at, provider, plan, model,
			input_tokens, output_tokens, reasoning_tokens, cache_read_tokens,
			cache_write_5m_tokens, cache_write_1h_tokens, cost_units
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	defer func() { _ = stmt.Close() }()
	for _, row := range rows {
		costUnits := row.ProviderCostUnits
		if costUnits == 0 && row.ProviderCostUSD > 0 {
			costUnits = int64(math.Round(row.ProviderCostUSD * 1e8))
		}
		if _, err := stmt.ExecContext(ctx,
			capturedAt.UTC().Format(time.RFC3339Nano), row.Time.UTC().Format(time.RFC3339Nano),
			strings.TrimSpace(row.Provider), strings.TrimSpace(row.Plan), row.Model,
			row.InputTokens, row.OutputTokens, row.ReasoningTokens, row.CacheReadTokens,
			row.CacheWrite5mTokens, row.CacheWrite1hTokens, costUnits,
		); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func (d *Database) GetProviderUsageAnalytics(ctx context.Context, days int) (*ProviderUsageAnalytics, error) {
	if days < 0 {
		days = 0
	}
	args := []any{}
	where := ""
	if days > 0 {
		where = " WHERE observed_at >= ?"
		args = append(args, time.Now().AddDate(0, 0, -days).UTC().Format(time.RFC3339Nano))
	}
	out := &ProviderUsageAnalytics{}
	row := d.db.QueryRowContext(ctx, `
		SELECT COUNT(*), COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0),
		       COALESCE(SUM(reasoning_tokens), 0), COALESCE(SUM(cache_read_tokens), 0),
		       COALESCE(SUM(cache_write_5m_tokens + cache_write_1h_tokens), 0),
		       COALESCE(SUM(cost_units), 0), COALESCE(MAX(snapshot_at), ''), COALESCE(MAX(observed_at), ''),
		       COALESCE(MAX(provider), ''), COALESCE(MAX(plan), '')
		FROM provider_usage`+where, args...)
	var costUnits int64
	var snapshotAt, observedAt, provider, plan string
	if err := row.Scan(&out.Summary.TotalRequests, &out.Summary.InputTokens, &out.Summary.OutputTokens,
		&out.Summary.ReasoningTokens, &out.Summary.CacheReadTokens, &out.Summary.CacheCreationTokens,
		&costUnits, &snapshotAt, &observedAt, &provider, &plan); err != nil {
		return nil, err
	}
	out.Summary.CostUSD = float64(costUnits) / 1e8
	out.Summary.Provider = provider
	out.Summary.Plan = plan
	if snapshotAt != "" {
		out.Summary.SnapshotAt, _ = time.Parse(time.RFC3339Nano, snapshotAt)
	}
	if observedAt != "" {
		out.Summary.ObservedAt, _ = time.Parse(time.RFC3339Nano, observedAt)
	}
	for _, spec := range []struct {
		name  string
		group string
		into  *[]ProviderUsageBreakdown
	}{
		{"models", "model", &out.Models},
		{"providers", "provider", &out.Providers},
		{"plans", "plan", &out.Plans},
	} {
		rows, err := d.db.QueryContext(ctx, `
			SELECT COALESCE(`+spec.group+`, 'unknown'),
			       COUNT(*), COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0),
			       COALESCE(SUM(reasoning_tokens), 0), COALESCE(SUM(cache_read_tokens), 0),
			       COALESCE(SUM(cache_write_5m_tokens + cache_write_1h_tokens), 0),
			       COALESCE(SUM(cost_units), 0), COALESCE(MAX(model), ''),
			       COALESCE(MAX(provider), ''), COALESCE(MAX(plan), '')
			FROM provider_usage`+where+` GROUP BY `+spec.group+` ORDER BY COUNT(*) DESC, `+spec.group, args...)
		if err != nil {
			return nil, err
		}
		var list []ProviderUsageBreakdown
		for rows.Next() {
			var item ProviderUsageBreakdown
			var units int64
			if err := rows.Scan(&item.Name, &item.Requests, &item.InputTokens, &item.OutputTokens,
				&item.ReasoningTokens, &item.CacheReadTokens, &item.CacheCreationTokens, &units,
				&item.Model, &item.Provider, &item.Plan); err != nil {
				_ = rows.Close()
				return nil, err
			}
			item.CostUSD = float64(units) / 1e8
			list = append(list, item)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
		*spec.into = list
	}
	trendRows, err := d.db.QueryContext(ctx, `
		SELECT DATE(observed_at), COUNT(*), COALESCE(SUM(input_tokens), 0),
		       COALESCE(SUM(output_tokens), 0), COALESCE(SUM(reasoning_tokens), 0),
		       COALESCE(SUM(cache_read_tokens), 0),
		       COALESCE(SUM(cache_write_5m_tokens + cache_write_1h_tokens), 0),
		       COALESCE(SUM(cost_units), 0)
		FROM provider_usage`+where+` GROUP BY DATE(observed_at) ORDER BY DATE(observed_at)`, args...)
	if err != nil {
		return nil, err
	}
	for trendRows.Next() {
		var point ProviderUsageDailyPoint
		var units int64
		if err := trendRows.Scan(&point.Date, &point.Requests, &point.InputTokens, &point.OutputTokens,
			&point.ReasoningTokens, &point.CacheReadTokens, &point.CacheCreationTokens, &units); err != nil {
			_ = trendRows.Close()
			return nil, err
		}
		point.CostUSD = float64(units) / 1e8
		out.Trend = append(out.Trend, point)
	}
	if err := trendRows.Err(); err != nil {
		_ = trendRows.Close()
		return nil, err
	}
	if err := trendRows.Close(); err != nil {
		return nil, err
	}
	recentRows, err := d.db.QueryContext(ctx, `
		SELECT observed_at, model, provider, plan, input_tokens, output_tokens,
		       reasoning_tokens, cache_read_tokens, cache_write_5m_tokens,
		       cache_write_1h_tokens, cost_units
		FROM provider_usage`+where+` ORDER BY observed_at DESC LIMIT 8`, args...)
	if err != nil {
		return nil, err
	}
	for recentRows.Next() {
		var item ProviderCostRecord
		var observed string
		if err := recentRows.Scan(&observed, &item.Model, &item.Provider, &item.Plan,
			&item.InputTokens, &item.OutputTokens, &item.ReasoningTokens, &item.CacheReadTokens,
			&item.CacheWrite5mTokens, &item.CacheWrite1hTokens, &item.ProviderCostUnits); err != nil {
			_ = recentRows.Close()
			return nil, err
		}
		item.Time, _ = time.Parse(time.RFC3339Nano, observed)
		out.Recent = append(out.Recent, item)
	}
	if err := recentRows.Err(); err != nil {
		_ = recentRows.Close()
		return nil, err
	}
	return out, recentRows.Close()
}

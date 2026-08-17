package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"
)

const maxProviderCostIssueExamples = 50

var ErrAmbiguousProviderCosts = errors.New("provider cost reconciliation contains ambiguous matches")

// ProviderCostRecord contains only the provider fields used to identify a
// completed request and replace its estimated cost.
type ProviderCostRecord struct {
	Time               time.Time `json:"time"`
	Model              string    `json:"model"`
	Provider           string    `json:"provider,omitempty"`
	Plan               string    `json:"plan,omitempty"`
	InputTokens        int64     `json:"input_tokens"`
	OutputTokens       int64     `json:"output_tokens"`
	ReasoningTokens    int64     `json:"reasoning_tokens,omitempty"`
	CacheReadTokens    int64     `json:"cache_read_tokens"`
	CacheWrite5mTokens int64     `json:"cache_write_5m_tokens"`
	CacheWrite1hTokens int64     `json:"cache_write_1h_tokens"`
	ProviderCostUnits  int64     `json:"cost_units"`
	ProviderCostUSD    float64   `json:"cost_usd"`
}

func (r ProviderCostRecord) costUSD() float64 {
	if r.ProviderCostUnits != 0 {
		return float64(r.ProviderCostUnits) / 1e8
	}
	return r.ProviderCostUSD
}

// ProviderCostIssue is a sanitized example of a row that was not uniquely
// matched. CandidateCount is the number of proxy rows sharing the relevant
// identity or timestamp/model pair.
type ProviderCostIssue struct {
	Kind           string    `json:"kind"`
	Time           time.Time `json:"time"`
	Model          string    `json:"model"`
	CandidateCount int       `json:"candidate_count"`
}

// ProviderCostReport describes a dry run or apply operation. Missing provider
// rows are expected when the provider account is shared with other clients.
type ProviderCostReport struct {
	ProviderRows    int                 `json:"provider_rows"`
	Exact           int                 `json:"exact"`
	Ambiguous       int                 `json:"ambiguous"`
	Missing         int                 `json:"missing"`
	Conflicting     int                 `json:"conflicting"`
	WouldUpdate     int                 `json:"would_update"`
	Updated         int                 `json:"updated"`
	ExactCostUSD    float64             `json:"exact_cost_usd"`
	IssueExamples   []ProviderCostIssue `json:"issue_examples,omitempty"`
	IssuesTruncated bool                `json:"issues_truncated,omitempty"`
}

type providerCostCandidate struct {
	id                  string
	completedAt         time.Time
	model               string
	input, output       int64
	cacheRead, cacheNew int64
	cost                sql.NullFloat64
	costSource          sql.NullString
}

type providerCostMatch struct {
	requestID string
	costUSD   float64
}

// ReconcileProviderCosts requires a unique full token fingerprint on both
// sides. Provider completion timestamps are second-granular; the proxy may
// finish local response handling in that second or the following second.
// When apply is true, all updates are rejected if any identity is ambiguous.
func (d *Database) ReconcileProviderCosts(ctx context.Context, providerRows []ProviderCostRecord, apply bool) (ProviderCostReport, error) {
	report := ProviderCostReport{ProviderRows: len(providerRows)}
	if len(providerRows) == 0 {
		return report, nil
	}

	minTime, maxTime := providerRows[0].Time, providerRows[0].Time
	for i, row := range providerRows {
		if err := validateProviderCostRecord(row); err != nil {
			return report, fmt.Errorf("provider row %d: %w", i+1, err)
		}
		if row.Time.Before(minTime) {
			minTime = row.Time
		}
		if row.Time.After(maxTime) {
			maxTime = row.Time
		}
	}

	candidates, err := d.providerCostCandidates(ctx, minTime, maxTime)
	if err != nil {
		return report, err
	}

	providerByIdentity := make(map[string][]ProviderCostRecord)
	for _, row := range providerRows {
		freshTokens := row.InputTokens + row.CacheWrite5mTokens + row.CacheWrite1hTokens
		key := providerCostFingerprint(row.Model, freshTokens, row.OutputTokens, row.CacheReadTokens)
		providerByIdentity[key] = append(providerByIdentity[key], row)
	}
	candidatesByIdentity := make(map[string][]providerCostCandidate)
	candidatesByTimeModel := make(map[string][]providerCostCandidate)
	for _, row := range candidates {
		key := providerCostFingerprint(row.model, row.input+row.cacheNew, row.output, row.cacheRead)
		candidatesByIdentity[key] = append(candidatesByIdentity[key], row)
		for _, observedAt := range []time.Time{row.completedAt, row.completedAt.Add(-time.Second)} {
			coarse := providerCostTimeModel(observedAt, row.model)
			candidatesByTimeModel[coarse] = append(candidatesByTimeModel[coarse], row)
		}
	}

	keys := make([]string, 0, len(providerByIdentity))
	for key := range providerByIdentity {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	var matches []providerCostMatch
	for _, key := range keys {
		providers := providerByIdentity[key]
		proxy := candidatesByIdentity[key]
		if len(proxy) > 0 && (len(providers) != 1 || len(proxy) != 1) {
			report.Ambiguous += len(providers)
			for _, row := range providers {
				report.addIssue("ambiguous", row, len(proxy))
			}
			continue
		}
		if len(providers) == 1 && len(proxy) == 1 {
			providerRow, candidate := providers[0], proxy[0]
			completionSkew := candidate.completedAt.UTC().Unix() - providerRow.Time.UTC().Unix()
			if completionSkew < 0 || completionSkew > 1 {
				report.Conflicting++
				report.addIssue("completion_time_conflict", providerRow, 1)
				continue
			}
			cost := providerRow.costUSD()
			if candidate.costSource.Valid && candidate.costSource.String == CostSourceProvider &&
				candidate.cost.Valid && math.Abs(candidate.cost.Float64-cost) > 1e-12 {
				report.Conflicting++
				report.addIssue("provider_cost_conflict", providerRow, 1)
				continue
			}
			report.Exact++
			report.ExactCostUSD += cost
			if !candidate.costSource.Valid || candidate.costSource.String != CostSourceProvider {
				report.WouldUpdate++
				matches = append(matches, providerCostMatch{requestID: candidate.id, costUSD: cost})
			}
			continue
		}

		for _, row := range providers {
			coarseCount := len(candidatesByTimeModel[providerCostTimeModel(row.Time, row.Model)])
			if coarseCount > 0 {
				report.Conflicting++
				report.addIssue("token_conflict", row, coarseCount)
			} else {
				report.Missing++
				report.addIssue("missing", row, 0)
			}
		}
	}

	if !apply {
		return report, nil
	}
	if report.Ambiguous > 0 {
		return report, ErrAmbiguousProviderCosts
	}
	if len(matches) == 0 {
		return report, nil
	}

	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return report, err
	}
	stmt, err := tx.PrepareContext(ctx, `UPDATE requests SET cost_usd = ?, cost_source = ? WHERE id = ?`)
	if err != nil {
		_ = tx.Rollback()
		return report, err
	}
	defer func() { _ = stmt.Close() }()
	for _, match := range matches {
		result, err := stmt.ExecContext(ctx, match.costUSD, CostSourceProvider, match.requestID)
		if err != nil {
			_ = tx.Rollback()
			return report, err
		}
		updated, err := result.RowsAffected()
		if err != nil {
			_ = tx.Rollback()
			return report, err
		}
		report.Updated += int(updated)
	}
	if err := tx.Commit(); err != nil {
		return report, err
	}
	return report, nil
}

func validateProviderCostRecord(row ProviderCostRecord) error {
	if row.Time.IsZero() {
		return errors.New("time is required")
	}
	if strings.TrimSpace(row.Model) == "" {
		return errors.New("model is required")
	}
	for name, value := range map[string]int64{
		"input_tokens": row.InputTokens, "output_tokens": row.OutputTokens,
		"reasoning_tokens":  row.ReasoningTokens,
		"cache_read_tokens": row.CacheReadTokens, "cache_write_5m_tokens": row.CacheWrite5mTokens,
		"cache_write_1h_tokens": row.CacheWrite1hTokens, "cost_units": row.ProviderCostUnits,
	} {
		if value < 0 {
			return fmt.Errorf("%s must not be negative", name)
		}
	}
	if row.ProviderCostUSD < 0 {
		return errors.New("cost_usd must not be negative")
	}
	return nil
}

func (d *Database) providerCostCandidates(ctx context.Context, minTime, maxTime time.Time) ([]providerCostCandidate, error) {
	rows, err := d.db.QueryContext(ctx, `
		SELECT id, start_time, duration_ms, model,
		       COALESCE(input_tokens, 0), COALESCE(output_tokens, 0),
		       COALESCE(cache_read_tokens, 0), COALESCE(cache_creation_tokens, 0),
		       cost_usd, cost_source
		FROM requests
		WHERE julianday(start_time) + (COALESCE(duration_ms, 0) / 86400000.0) >= julianday(?)
		  AND julianday(start_time) + (COALESCE(duration_ms, 0) / 86400000.0) < julianday(?)
	`, minTime.UTC().Truncate(time.Second).Format(time.RFC3339Nano),
		maxTime.UTC().Truncate(time.Second).Add(2*time.Second).Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var candidates []providerCostCandidate
	for rows.Next() {
		var row providerCostCandidate
		var rawTime string
		var durationMS int64
		if err := rows.Scan(&row.id, &rawTime, &durationMS, &row.model, &row.input, &row.output,
			&row.cacheRead, &row.cacheNew, &row.cost, &row.costSource); err != nil {
			return nil, err
		}
		parsed, err := time.Parse(time.RFC3339Nano, rawTime)
		if err != nil {
			return nil, fmt.Errorf("parse request %q start time: %w", row.id, err)
		}
		row.completedAt = parsed.Add(time.Duration(durationMS) * time.Millisecond)
		candidates = append(candidates, row)
	}
	return candidates, rows.Err()
}

func providerCostFingerprint(model string, fresh, output, cacheRead int64) string {
	return fmt.Sprintf("%s|%d|%d|%d", normalizeProviderCostModel(model), fresh, output, cacheRead)
}

func providerCostTimeModel(at time.Time, model string) string {
	return fmt.Sprintf("%d|%s", at.UTC().Unix(), normalizeProviderCostModel(model))
}

func normalizeProviderCostModel(model string) string {
	model = strings.TrimSpace(model)
	if i := strings.LastIndexByte(model, '/'); i >= 0 {
		model = model[i+1:]
	}
	return strings.ToLower(model)
}

func (r *ProviderCostReport) addIssue(kind string, row ProviderCostRecord, candidates int) {
	if len(r.IssueExamples) >= maxProviderCostIssueExamples {
		r.IssuesTruncated = true
		return
	}
	r.IssueExamples = append(r.IssueExamples, ProviderCostIssue{
		Kind: kind, Time: row.Time, Model: row.Model, CandidateCount: candidates,
	})
}

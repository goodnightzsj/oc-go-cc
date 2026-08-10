package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"time"
)

// ProviderRequestSyncReport describes the one-time correction that makes the
// primary request history agree with the persisted provider usage snapshot.
type ProviderRequestSyncReport struct {
	SnapshotRows      int       `json:"snapshot_rows"`
	SnapshotAt        time.Time `json:"snapshot_at"`
	ObservedStart     time.Time `json:"observed_start"`
	ObservedEnd       time.Time `json:"observed_end"`
	SnapshotCostUSD   float64   `json:"snapshot_cost_usd"`
	CandidateRows     int       `json:"candidate_rows"`
	MatchedDetails    int       `json:"matched_details"`
	ExistingImported  int       `json:"existing_imported"`
	WouldInsert       int       `json:"would_insert"`
	WouldRemove       int       `json:"would_remove"`
	WouldUpdate       int       `json:"would_update"`
	Inserted          int       `json:"inserted"`
	Removed           int       `json:"removed"`
	Updated           int       `json:"updated"`
	ProjectedRequests int64     `json:"projected_requests"`
	ProjectedCostUSD  float64   `json:"projected_cost_usd"`
}

type providerRequestSyncRow struct {
	sourceID   int64
	snapshotAt time.Time
	ProviderCostRecord
}

type providerRequestCandidate struct {
	id           string
	completedAt  time.Time
	model        string
	provider     string
	input        int64
	output       int64
	cacheRead    int64
	cacheNew     int64
	cost         sql.NullFloat64
	costSource   sql.NullString
	detailsKnown bool
	usageTrusted bool
}

// SyncProviderUsageRequests preserves uniquely matched local requests, drops
// untrustworthy rows inside the snapshot boundary, and fills every missing
// provider row. Rows created after the snapshot are outside the correction.
// Dry runs compute the exact projected totals without writing.
func (d *Database) SyncProviderUsageRequests(ctx context.Context, apply bool) (ProviderRequestSyncReport, error) {
	var report ProviderRequestSyncReport
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return report, err
	}
	defer func() { _ = tx.Rollback() }()

	providerRows, err := providerUsageRowsForSync(ctx, tx)
	if err != nil {
		return report, err
	}
	if len(providerRows) == 0 {
		return report, errors.New("provider usage snapshot is empty")
	}
	report.SnapshotRows = len(providerRows)
	report.SnapshotAt = providerRows[0].snapshotAt
	report.ObservedStart = providerRows[0].Time
	report.ObservedEnd = providerRows[0].Time
	for i, row := range providerRows {
		if !row.snapshotAt.Equal(report.SnapshotAt) {
			return report, errors.New("provider usage contains multiple snapshots")
		}
		if err := validateProviderCostRecord(row.ProviderCostRecord); err != nil {
			return report, fmt.Errorf("provider usage row %d: %w", i+1, err)
		}
		if row.Time.Before(report.ObservedStart) {
			report.ObservedStart = row.Time
		}
		if row.Time.After(report.ObservedEnd) {
			report.ObservedEnd = row.Time
		}
		report.SnapshotCostUSD += row.costUSD()
	}

	candidates, err := providerRequestCandidatesForSync(ctx, tx, report.ObservedStart, report.ObservedEnd, report.SnapshotAt)
	if err != nil {
		return report, err
	}
	report.CandidateRows = len(candidates)

	requestIDs := make([]string, len(providerRows))
	occurrences := map[string]int{}
	desiredByID := make(map[string]int, len(providerRows))
	for i, row := range providerRows {
		canonical := providerRequestCanonical(row.ProviderCostRecord)
		occurrences[canonical]++
		requestIDs[i] = providerRequestID(canonical, occurrences[canonical])
		desiredByID[requestIDs[i]] = i
	}

	mapped := make(map[int]int, len(providerRows))
	usedCandidates := make(map[int]bool, len(candidates))
	for candidateIndex, candidate := range candidates {
		providerIndex, ok := desiredByID[candidate.id]
		if !ok {
			continue
		}
		if candidate.detailsKnown {
			return report, fmt.Errorf("generated request id %q collides with a local request", candidate.id)
		}
		mapped[providerIndex] = candidateIndex
		usedCandidates[candidateIndex] = true
		report.ExistingImported++
	}

	providerByIdentity := map[string][]int{}
	for i, row := range providerRows {
		if _, ok := mapped[i]; ok {
			continue
		}
		fresh := row.InputTokens + row.CacheWrite5mTokens + row.CacheWrite1hTokens
		identity := providerCostFingerprint(row.Model, fresh, row.OutputTokens, row.CacheReadTokens)
		providerByIdentity[identity] = append(providerByIdentity[identity], i)
	}
	candidatesByIdentity := map[string][]int{}
	for i, candidate := range candidates {
		if usedCandidates[i] || !candidate.detailsKnown {
			continue
		}
		identity := providerCostFingerprint(candidate.model, candidate.input+candidate.cacheNew, candidate.output, candidate.cacheRead)
		candidatesByIdentity[identity] = append(candidatesByIdentity[identity], i)
	}
	for identity, providerIndexes := range providerByIdentity {
		candidateIndexes := candidatesByIdentity[identity]
		if len(providerIndexes) != 1 || len(candidateIndexes) != 1 {
			continue
		}
		providerIndex, candidateIndex := providerIndexes[0], candidateIndexes[0]
		skew := candidates[candidateIndex].completedAt.UTC().Unix() - providerRows[providerIndex].Time.UTC().Unix()
		if skew < 0 || skew > 1 {
			continue
		}
		mapped[providerIndex] = candidateIndex
		usedCandidates[candidateIndex] = true
		report.MatchedDetails++
	}
	canonicalProviders := providerRequestProviderMappings(providerRows, candidates, mapped)

	var currentRequests int64
	var currentCost float64
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(cost_usd), 0) FROM requests`).Scan(&currentRequests, &currentCost); err != nil {
		return report, err
	}
	report.ProjectedRequests = currentRequests
	report.ProjectedCostUSD = currentCost

	for i, candidate := range candidates {
		if usedCandidates[i] {
			continue
		}
		report.WouldRemove++
		report.ProjectedRequests--
		if candidate.cost.Valid {
			report.ProjectedCostUSD -= candidate.cost.Float64
		}
	}
	for providerIndex, row := range providerRows {
		candidateIndex, ok := mapped[providerIndex]
		if !ok {
			report.WouldInsert++
			report.ProjectedRequests++
			report.ProjectedCostUSD += row.costUSD()
			continue
		}
		candidate := candidates[candidateIndex]
		cost := row.costUSD()
		provider := canonicalProvider(row.Provider, canonicalProviders)
		if providerRequestNeedsUpdate(candidate, cost, provider) {
			report.WouldUpdate++
			if candidate.cost.Valid {
				report.ProjectedCostUSD -= candidate.cost.Float64
			}
			report.ProjectedCostUSD += cost
		}
	}

	if !apply {
		return report, nil
	}
	if err := applyProviderRequestSync(ctx, tx, providerRows, candidates, requestIDs, mapped, usedCandidates, canonicalProviders, &report); err != nil {
		return report, err
	}
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(cost_usd), 0) FROM requests`).Scan(&report.ProjectedRequests, &report.ProjectedCostUSD); err != nil {
		return report, err
	}
	if err := tx.Commit(); err != nil {
		return report, err
	}
	return report, nil
}

func providerUsageRowsForSync(ctx context.Context, tx *sql.Tx) ([]providerRequestSyncRow, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, snapshot_at, observed_at, provider, plan, model,
		       input_tokens, output_tokens, reasoning_tokens, cache_read_tokens,
		       cache_write_5m_tokens, cache_write_1h_tokens, cost_units
		FROM provider_usage ORDER BY observed_at, id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []providerRequestSyncRow
	for rows.Next() {
		var row providerRequestSyncRow
		var snapshotAt, observedAt string
		if err := rows.Scan(&row.sourceID, &snapshotAt, &observedAt, &row.Provider, &row.Plan, &row.Model,
			&row.InputTokens, &row.OutputTokens, &row.ReasoningTokens, &row.CacheReadTokens,
			&row.CacheWrite5mTokens, &row.CacheWrite1hTokens, &row.ProviderCostUnits); err != nil {
			return nil, err
		}
		var err error
		row.snapshotAt, err = time.Parse(time.RFC3339Nano, snapshotAt)
		if err != nil {
			return nil, fmt.Errorf("parse snapshot time: %w", err)
		}
		row.Time, err = time.Parse(time.RFC3339Nano, observedAt)
		if err != nil {
			return nil, fmt.Errorf("parse observed time: %w", err)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func providerRequestCandidatesForSync(ctx context.Context, tx *sql.Tx, minTime, maxTime, snapshotAt time.Time) ([]providerRequestCandidate, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, start_time, COALESCE(duration_ms, 0), model, COALESCE(provider, ''),
		       COALESCE(input_tokens, 0), COALESCE(output_tokens, 0),
		       COALESCE(cache_read_tokens, 0), COALESCE(cache_creation_tokens, 0),
		       cost_usd, cost_source, details_known, usage_trusted
		FROM requests
		WHERE julianday(start_time) + (COALESCE(duration_ms, 0) / 86400000.0) >= julianday(?)
		  AND julianday(start_time) + (COALESCE(duration_ms, 0) / 86400000.0) < julianday(?)
		  AND julianday(created_at) <= julianday(?)`,
		minTime.UTC().Truncate(time.Second).Format(time.RFC3339Nano),
		maxTime.UTC().Truncate(time.Second).Add(2*time.Second).Format(time.RFC3339Nano),
		snapshotAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []providerRequestCandidate
	for rows.Next() {
		var row providerRequestCandidate
		var startTime string
		var durationMS int64
		var detailsKnown, usageTrusted int
		if err := rows.Scan(&row.id, &startTime, &durationMS, &row.model, &row.provider, &row.input, &row.output,
			&row.cacheRead, &row.cacheNew, &row.cost, &row.costSource, &detailsKnown, &usageTrusted); err != nil {
			return nil, err
		}
		startedAt, err := time.Parse(time.RFC3339Nano, startTime)
		if err != nil {
			return nil, fmt.Errorf("parse request %q start time: %w", row.id, err)
		}
		row.completedAt = startedAt.Add(time.Duration(durationMS) * time.Millisecond)
		row.detailsKnown = detailsKnown == 1
		row.usageTrusted = usageTrusted == 1
		out = append(out, row)
	}
	return out, rows.Err()
}

func applyProviderRequestSync(ctx context.Context, tx *sql.Tx, providerRows []providerRequestSyncRow, candidates []providerRequestCandidate, requestIDs []string, mapped map[int]int, usedCandidates map[int]bool, canonicalProviders map[string]string, report *ProviderRequestSyncReport) error {
	for i, candidate := range candidates {
		if usedCandidates[i] {
			continue
		}
		result, err := tx.ExecContext(ctx, `DELETE FROM requests WHERE id = ?`, candidate.id)
		if err != nil {
			return err
		}
		removed, _ := result.RowsAffected()
		report.Removed += int(removed)
	}
	for providerIndex, row := range providerRows {
		provider := canonicalProvider(row.Provider, canonicalProviders)
		if candidateIndex, ok := mapped[providerIndex]; ok {
			candidate := candidates[candidateIndex]
			cost := row.costUSD()
			if providerRequestNeedsUpdate(candidate, cost, provider) {
				result, err := tx.ExecContext(ctx, `UPDATE requests SET provider = ?, cost_usd = ?, cost_source = ?, usage_trusted = 1 WHERE id = ?`, provider, cost, CostSourceProvider, candidate.id)
				if err != nil {
					return err
				}
				updated, _ := result.RowsAffected()
				report.Updated += int(updated)
			}
			continue
		}
		result, err := tx.ExecContext(ctx, `
			INSERT INTO requests (
				id, model, provider, scenario, start_time, duration_ms,
				input_tokens, output_tokens, cache_read_tokens, cache_creation_tokens,
				cost_usd, cost_source, details_known, usage_trusted, streaming, success, error_msg, attempt, created_at
			) VALUES (?, ?, ?, '', ?, 0, ?, ?, ?, ?, ?, ?, 0, 1, 0, 0, '', 1, ?)`,
			requestIDs[providerIndex], row.Model, provider, row.Time.UTC().Format(time.RFC3339Nano),
			row.InputTokens, row.OutputTokens, row.CacheReadTokens,
			row.CacheWrite5mTokens+row.CacheWrite1hTokens, row.costUSD(), CostSourceProvider,
			row.snapshotAt.UTC().Format(time.RFC3339Nano))
		if err != nil {
			return err
		}
		inserted, _ := result.RowsAffected()
		report.Inserted += int(inserted)
	}
	return nil
}

func providerRequestProviderMappings(providerRows []providerRequestSyncRow, candidates []providerRequestCandidate, mapped map[int]int) map[string]string {
	observed := map[string]map[string]bool{}
	for providerIndex, candidateIndex := range mapped {
		candidate := candidates[candidateIndex]
		if !candidate.detailsKnown || providerRows[providerIndex].Provider == "" || candidate.provider == "" {
			continue
		}
		raw := providerRows[providerIndex].Provider
		if observed[raw] == nil {
			observed[raw] = map[string]bool{}
		}
		observed[raw][candidate.provider] = true
	}
	canonical := map[string]string{}
	for raw, providers := range observed {
		if len(providers) != 1 {
			continue
		}
		for provider := range providers {
			canonical[raw] = provider
		}
	}
	return canonical
}

func canonicalProvider(provider string, mappings map[string]string) string {
	if canonical := mappings[provider]; canonical != "" {
		return canonical
	}
	return provider
}

func providerRequestNeedsUpdate(candidate providerRequestCandidate, cost float64, provider string) bool {
	return !candidate.usageTrusted || candidate.provider != provider || !candidate.cost.Valid ||
		!candidate.costSource.Valid || candidate.costSource.String != CostSourceProvider ||
		math.Abs(candidate.cost.Float64-cost) > 1e-12
}

func providerRequestCanonical(row ProviderCostRecord) string {
	return fmt.Sprintf("%s|%s|%s|%s|%d|%d|%d|%d|%d|%d|%d",
		row.Time.UTC().Format(time.RFC3339Nano), row.Model, row.Provider, row.Plan,
		row.InputTokens, row.OutputTokens, row.ReasoningTokens, row.CacheReadTokens,
		row.CacheWrite5mTokens, row.CacheWrite1hTokens, row.ProviderCostUnits)
}

func providerRequestID(canonical string, occurrence int) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%d", canonical, occurrence)))
	return "req_" + hex.EncodeToString(sum[:12])
}

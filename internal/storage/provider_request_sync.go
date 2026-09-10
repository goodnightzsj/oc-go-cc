package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"slices"
	"time"
)

const providerUsageTarget = "opencode-go"

// ProviderRequestSyncReport describes a non-destructive sync of the persisted
// OpenCode Go account snapshot. Unmatched local rows are never removed.
type ProviderRequestSyncReport struct {
	TargetProvider     string              `json:"target_provider"`
	SnapshotRows       int                 `json:"snapshot_rows"`
	SnapshotAt         time.Time           `json:"snapshot_at"`
	ObservedStart      time.Time           `json:"observed_start"`
	ObservedEnd        time.Time           `json:"observed_end"`
	SnapshotCostUSD    float64             `json:"snapshot_cost_usd"`
	CandidateRows      int                 `json:"candidate_rows"`
	PreservedUnmatched int                 `json:"preserved_unmatched"`
	Ambiguous          int                 `json:"ambiguous"`
	Conflicting        int                 `json:"conflicting"`
	MatchedDetails     int                 `json:"matched_details"`
	ExistingImported   int                 `json:"existing_imported"`
	WouldInsert        int                 `json:"would_insert"`
	WouldRemove        int                 `json:"would_remove"` // Always zero; retained for report compatibility.
	WouldUpdate        int                 `json:"would_update"`
	Inserted           int                 `json:"inserted"`
	Removed            int                 `json:"removed"` // Always zero; unmatched rows are preserved.
	Updated            int                 `json:"updated"`
	ProjectedRequests  int64               `json:"projected_requests"`
	ProjectedCostUSD   float64             `json:"projected_cost_usd"`
	IssueExamples      []ProviderCostIssue `json:"issue_examples,omitempty"`
	IssuesTruncated    bool                `json:"issues_truncated,omitempty"`
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
	scenario     string
	input        int64
	output       int64
	cacheRead    int64
	cacheNew     int64
	cost         sql.NullFloat64
	costSource   sql.NullString
	detailsKnown bool
	usageTrusted bool
}

// SyncProviderUsageRequests matches only OpenCode Go requests to the account
// snapshot. The export's provider field is a physical backend, not a platform
// identity. Missing rows may be imported, but incomplete snapshots cannot prove
// that unmatched local requests are duplicates. Ambiguous apply runs fail
// without writes; conflicting rows are reported and left unchanged.
func (d *Database) SyncProviderUsageRequests(ctx context.Context, apply bool) (ProviderRequestSyncReport, error) {
	report := ProviderRequestSyncReport{TargetProvider: providerUsageTarget}
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
	providerByIdentity := map[string][]int{}
	for i, row := range providerRows {
		requestIDs[i] = providerRequestID(providerRequestCanonical(row.ProviderCostRecord))
		fresh := row.InputTokens + row.CacheWrite5mTokens + row.CacheWrite1hTokens
		identity := providerCostFingerprint(row.Model, fresh, row.OutputTokens, row.CacheReadTokens)
		providerByIdentity[identity] = append(providerByIdentity[identity], i)
	}
	candidatesByIdentity := map[string][]int{}
	candidatesByTimeModel := map[string]int{}
	for i, candidate := range candidates {
		identity := providerCostFingerprint(candidate.model, candidate.input+candidate.cacheNew, candidate.output, candidate.cacheRead)
		candidatesByIdentity[identity] = append(candidatesByIdentity[identity], i)
		for _, observedAt := range []time.Time{candidate.completedAt, candidate.completedAt.Add(-time.Second)} {
			candidatesByTimeModel[providerCostTimeModel(observedAt, candidate.model)]++
		}
	}
	identities := make([]string, 0, len(providerByIdentity))
	for identity := range providerByIdentity {
		identities = append(identities, identity)
	}
	slices.Sort(identities)
	mapped := make(map[int]int, len(providerRows))
	usedCandidates := make(map[int]bool, len(candidates))
	skipped := make(map[int]bool)
	for _, identity := range identities {
		providerIndexes := providerByIdentity[identity]
		candidateIndexes := candidatesByIdentity[identity]
		if len(providerIndexes) > 1 || len(candidateIndexes) > 1 {
			report.Ambiguous += len(providerIndexes)
			for _, i := range providerIndexes {
				skipped[i] = true
				report.addIssue("ambiguous", providerRows[i].ProviderCostRecord, len(candidateIndexes))
			}
			continue
		}
		providerIndex := providerIndexes[0]
		row := providerRows[providerIndex]
		if len(candidateIndexes) == 0 {
			if count := candidatesByTimeModel[providerCostTimeModel(row.Time, row.Model)]; count > 0 {
				skipped[providerIndex] = true
				report.Conflicting++
				report.addIssue("token_conflict", row.ProviderCostRecord, count)
			}
			continue
		}
		candidateIndex := candidateIndexes[0]
		candidate := candidates[candidateIndex]
		skew := candidate.completedAt.UTC().Unix() - row.Time.UTC().Unix()
		if skew < 0 || skew > 1 {
			skipped[providerIndex] = true
			report.Conflicting++
			report.addIssue("completion_time_conflict", row.ProviderCostRecord, 1)
			continue
		}
		if providerCostConflicts(candidate.cost, candidate.costSource, row.costUSD()) {
			skipped[providerIndex] = true
			report.Conflicting++
			report.addIssue("provider_cost_conflict", row.ProviderCostRecord, 1)
			continue
		}
		mapped[providerIndex] = candidateIndex
		usedCandidates[candidateIndex] = true
		if candidate.detailsKnown {
			report.MatchedDetails++
		} else {
			report.ExistingImported++
		}
	}
	report.PreservedUnmatched = len(candidates) - len(usedCandidates)

	var currentRequests int64
	var currentCost float64
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(cost_usd), 0) FROM requests`).Scan(&currentRequests, &currentCost); err != nil {
		return report, err
	}
	report.ProjectedRequests = currentRequests
	report.ProjectedCostUSD = currentCost

	for providerIndex, row := range providerRows {
		if skipped[providerIndex] {
			continue
		}
		candidateIndex, ok := mapped[providerIndex]
		if !ok {
			report.WouldInsert++
			report.ProjectedRequests++
			report.ProjectedCostUSD += row.costUSD()
			continue
		}
		candidate := candidates[candidateIndex]
		cost := row.costUSD()
		if providerRequestNeedsUpdate(candidate, cost, providerUsageTarget) {
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
	if report.Ambiguous > 0 {
		return report, ErrAmbiguousProviderCosts
	}
	applied := report
	if err := applyProviderRequestSync(ctx, tx, providerRows, candidates, requestIDs, mapped, skipped, &applied); err != nil {
		return report, err
	}
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(cost_usd), 0) FROM requests`).Scan(&applied.ProjectedRequests, &applied.ProjectedCostUSD); err != nil {
		return report, err
	}
	if err := tx.Commit(); err != nil {
		return report, err
	}
	return applied, nil
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
		SELECT id, start_time, COALESCE(duration_ms, 0), model, COALESCE(provider, ''), COALESCE(scenario, ''),
		       COALESCE(input_tokens, 0), COALESCE(output_tokens, 0),
		       COALESCE(cache_read_tokens, 0), COALESCE(cache_creation_tokens, 0),
		       cost_usd, cost_source, details_known, usage_trusted
		FROM requests
		WHERE REPLACE(provider, '_', '-') = ?
		  AND julianday(start_time) + (COALESCE(duration_ms, 0) / 86400000.0) >= julianday(?)
		  AND julianday(start_time) + (COALESCE(duration_ms, 0) / 86400000.0) < julianday(?)
		  AND julianday(created_at) <= julianday(?)`,
		providerUsageTarget, minTime.UTC().Truncate(time.Second).Format(time.RFC3339Nano),
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
		var provider, scenario sql.NullString
		if err := rows.Scan(&row.id, &startTime, &durationMS, &row.model, &provider, &scenario, &row.input, &row.output,
			&row.cacheRead, &row.cacheNew, &row.cost, &row.costSource, &detailsKnown, &usageTrusted); err != nil {
			return nil, err
		}
		row.provider = provider.String
		row.scenario = scenario.String
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

func applyProviderRequestSync(ctx context.Context, tx *sql.Tx, providerRows []providerRequestSyncRow, candidates []providerRequestCandidate, requestIDs []string, mapped map[int]int, skipped map[int]bool, report *ProviderRequestSyncReport) error {
	for providerIndex, row := range providerRows {
		if skipped[providerIndex] {
			continue
		}
		if candidateIndex, ok := mapped[providerIndex]; ok {
			candidate := candidates[candidateIndex]
			cost := row.costUSD()
			if providerRequestNeedsUpdate(candidate, cost, providerUsageTarget) {
				scenario := candidate.scenario
				if !candidate.detailsKnown {
					scenario = "override"
				}
				result, err := tx.ExecContext(ctx, `UPDATE requests SET provider = ?, scenario = ?, cost_usd = ?, cost_source = ?, usage_trusted = 1 WHERE id = ?`, providerUsageTarget, scenario, cost, CostSourceProvider, candidate.id)
				if err != nil {
					return err
				}
				updated, err := result.RowsAffected()
				if err != nil {
					return err
				}
				report.Updated += int(updated)
			}
			continue
		}
		result, err := tx.ExecContext(ctx, `
			INSERT INTO requests (
				id, model, provider, scenario, start_time, duration_ms,
				input_tokens, output_tokens, cache_read_tokens, cache_creation_tokens,
				cost_usd, cost_source, details_known, usage_trusted, streaming, success, error_msg, attempt, created_at
			) VALUES (?, ?, ?, 'override', ?, 0, ?, ?, ?, ?, ?, ?, 0, 1, 0, 0, '', 1, ?)`,
			requestIDs[providerIndex], row.Model, providerUsageTarget, row.Time.UTC().Format(time.RFC3339Nano),
			row.InputTokens, row.OutputTokens, row.CacheReadTokens,
			row.CacheWrite5mTokens+row.CacheWrite1hTokens, row.costUSD(), CostSourceProvider,
			row.snapshotAt.UTC().Format(time.RFC3339Nano))
		if err != nil {
			return err
		}
		inserted, err := result.RowsAffected()
		if err != nil {
			return err
		}
		report.Inserted += int(inserted)
	}
	return nil
}

func providerRequestNeedsUpdate(candidate providerRequestCandidate, cost float64, provider string) bool {
	return !candidate.usageTrusted || candidate.provider != provider || !candidate.cost.Valid ||
		!candidate.costSource.Valid || candidate.costSource.String != CostSourceProvider ||
		math.Abs(candidate.cost.Float64-cost) > 1e-12 ||
		(!candidate.detailsKnown && candidate.scenario != "override")
}

func providerRequestCanonical(row ProviderCostRecord) string {
	return fmt.Sprintf("%s|%s|%s|%s|%d|%d|%d|%d|%d|%d|%d",
		row.Time.UTC().Format(time.RFC3339Nano), row.Model, row.Provider, row.Plan,
		row.InputTokens, row.OutputTokens, row.ReasoningTokens, row.CacheReadTokens,
		row.CacheWrite5mTokens, row.CacheWrite1hTokens, row.ProviderCostUnits)
}

func providerRequestID(canonical string) string {
	sum := sha256.Sum256([]byte(providerUsageTarget + "|" + canonical))
	return "req_" + hex.EncodeToString(sum[:12])
}

func (r *ProviderRequestSyncReport) addIssue(kind string, row ProviderCostRecord, candidates int) {
	if len(r.IssueExamples) >= maxProviderCostIssueExamples {
		r.IssuesTruncated = true
		return
	}
	r.IssueExamples = append(r.IssueExamples, ProviderCostIssue{
		Kind: kind, Time: row.Time, Model: row.Model, CandidateCount: candidates,
	})
}

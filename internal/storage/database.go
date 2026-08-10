// Package storage provides SQLite-based persistent storage for the proxy.
package storage

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type Database struct {
	db   *sql.DB
	path string
	mu   sync.RWMutex

	// analyticsBaseline is the parsed Config.AnalyticsBaseline; zero means the
	// full history is analysed.
	analyticsBaseline time.Time
}

// AnalyticsBaseline reports the configured cutoff for analytics aggregates.
// The zero time means no cutoff.
func (d *Database) AnalyticsBaseline() time.Time {
	return d.analyticsBaseline
}

type Config struct {
	DatabasePath    string `json:"database_path"`
	RetentionDays   int    `json:"retention_days"`
	VacuumOnStartup bool   `json:"vacuum_on_startup"`
	WALEnabled      bool   `json:"wal_enabled"`
	// AnalyticsBaseline optionally drops requests recorded before this instant
	// from every analytics aggregate, as an RFC3339 timestamp. Use it when an
	// older build recorded a token split that cannot be trusted — for example a
	// prompt billed entirely as fresh input because the upstream cache fields
	// were never parsed. Those rows cannot be recomputed (the hit/miss
	// breakdown was never stored), so excluding them is the only way to make
	// the cost figure match the provider's own billing. Empty means no cutoff.
	AnalyticsBaseline string `json:"analytics_baseline,omitempty"`
}

var DefaultConfig = Config{
	DatabasePath:    "~/.local/share/routatic-proxy/data.db",
	RetentionDays:   7,
	VacuumOnStartup: false,
	WALEnabled:      true,
}

// Overlay describes storage settings a caller explicitly configured. The zero
// value means "nothing set", so applying it changes nothing.
type Overlay struct {
	DatabasePath      string
	RetentionDays     int
	VacuumOnStartup   bool
	WALEnabled        *bool
	AnalyticsBaseline string
}

// WithOverlay returns cfg with the caller's configured fields applied on top.
//
// Callers must not assemble a Config from scratch out of user input. A config
// file that sets only analytics_baseline would otherwise leave DatabasePath
// empty, and Open is skipped entirely when that is empty — silently disabling
// persistence and every analytics endpoint along with it.
func (c Config) WithOverlay(o Overlay) Config {
	if o.DatabasePath != "" {
		c.DatabasePath = o.DatabasePath
	}
	if o.RetentionDays != 0 {
		c.RetentionDays = o.RetentionDays
	}
	if o.VacuumOnStartup {
		c.VacuumOnStartup = true
	}
	if o.WALEnabled != nil {
		c.WALEnabled = *o.WALEnabled
	}
	if o.AnalyticsBaseline != "" {
		c.AnalyticsBaseline = o.AnalyticsBaseline
	}
	return c
}

func Open(cfg Config) (*Database, error) {
	path := expandPath(cfg.DatabasePath)

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	// modernc.org/sqlite only parses the _pragma, _time_format and _txlock DSN
	// parameters. The bare _journal_mode / _synchronous / _busy_timeout form
	// used by mattn/go-sqlite3 is silently ignored here, which left WAL off
	// (journal_mode=delete) and busy_timeout at 0 — every write that hit
	// contention failed immediately with SQLITE_BUSY instead of waiting.
	dsn := path + "?_pragma=busy_timeout(5000)"
	if cfg.WALEnabled {
		dsn += "&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	database := &Database{
		db:   db,
		path: path,
	}

	// Parse the baseline here so a malformed timestamp surfaces at startup
	// rather than silently disabling the cutoff on every dashboard request.
	if raw := strings.TrimSpace(cfg.AnalyticsBaseline); raw != "" {
		baseline, parseErr := time.Parse(time.RFC3339, raw)
		if parseErr != nil {
			_ = db.Close()
			return nil, fmt.Errorf("parse analytics_baseline %q: %w", raw, parseErr)
		}
		database.analyticsBaseline = baseline
	}

	if err := database.initSchema(ctx); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}

	// Lightweight migrations for new columns (safe on existing DBs)
	if err := database.migrateAddAttemptColumn(ctx); err != nil {
		// Non-fatal; log and continue so the proxy still works
		slog.Warn("migration warning", "err", err)
	}
	if err := database.migrateAddCacheColumns(ctx); err != nil {
		slog.Warn("migration warning", "err", err)
	}
	if err := database.migrateAddCostColumn(ctx); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("migrate request cost column: %w", err)
	}

	// Seed default model prices so analytics dashboard shows meaningful
	// cost numbers immediately for new/existing installs. Idempotent.
	_ = database.SeedDefaultModelPrices(ctx)
	if _, err := database.BackfillRequestCosts(ctx); err != nil {
		slog.Warn("request cost backfill warning", "err", err)
	}

	if cfg.VacuumOnStartup {
		if _, err := db.ExecContext(ctx, "VACUUM"); err != nil {
			_ = database.Close()
			return nil, fmt.Errorf("vacuum: %w", err)
		}
	}

	return database, nil
}

func (d *Database) initSchema(ctx context.Context) error {
	schema := `
	CREATE TABLE IF NOT EXISTS requests (
		id TEXT PRIMARY KEY,
		model TEXT NOT NULL,
		provider TEXT,
		scenario TEXT,
		start_time TIMESTAMP NOT NULL,
		duration_ms INTEGER,
		input_tokens INTEGER,
		output_tokens INTEGER,
		cache_read_tokens INTEGER DEFAULT 0,
		cache_creation_tokens INTEGER DEFAULT 0,
		cost_usd REAL,
		streaming INTEGER,
		success INTEGER,
		error_msg TEXT,
		attempt INTEGER DEFAULT 1,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_requests_start_time ON requests(start_time);
	CREATE INDEX IF NOT EXISTS idx_requests_model ON requests(model);
	CREATE INDEX IF NOT EXISTS idx_requests_created_at ON requests(created_at);

	CREATE TABLE IF NOT EXISTS latency_samples (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		model TEXT NOT NULL,
		latency_ms INTEGER NOT NULL,
		recorded_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_latency_model_time ON latency_samples(model, recorded_at);
	CREATE INDEX IF NOT EXISTS idx_latency_recorded_at ON latency_samples(recorded_at);

	CREATE TABLE IF NOT EXISTS logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		level TEXT NOT NULL,
		message TEXT,
		field TEXT,
		value TEXT,
		recorded_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_logs_recorded_at ON logs(recorded_at);
	CREATE INDEX IF NOT EXISTS idx_logs_level ON logs(level);

	CREATE TABLE IF NOT EXISTS schema_info (
		key TEXT PRIMARY KEY,
		value TEXT
	);

	INSERT OR IGNORE INTO schema_info (key, value) VALUES ('version', '1');

	CREATE TABLE IF NOT EXISTS providers (
		name TEXT PRIMARY KEY,
		base_url TEXT,
		api_key TEXT,
		enabled INTEGER DEFAULT 1,
		anthropic_tools_disabled INTEGER DEFAULT 0,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_providers_enabled ON providers(enabled);

	CREATE TABLE IF NOT EXISTS models (
		id TEXT PRIMARY KEY,
		provider TEXT NOT NULL,
		name TEXT NOT NULL,
		display_name TEXT,
		context_window INTEGER,
		cost_input_per_m REAL,
		cost_output_per_m REAL,
		supports_tools INTEGER DEFAULT 1,
		supports_vision INTEGER DEFAULT 0,
		supports_reasoning INTEGER DEFAULT 0,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (provider) REFERENCES providers(name)
	);

	CREATE INDEX IF NOT EXISTS idx_models_provider ON models(provider);
	CREATE INDEX IF NOT EXISTS idx_models_name ON models(name);

	CREATE TABLE IF NOT EXISTS scenarios (
		key TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT,
		requires_tools INTEGER,
		requires_vision INTEGER,
		requires_reasoning INTEGER,
		min_context_window INTEGER,
		preferred_providers TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_scenarios_name ON scenarios(name);
	`

	_, err := d.db.ExecContext(ctx, schema)
	return err
}

// migrateAddAttemptColumn adds the 'attempt' column to the requests table if it does not exist.
// This is used for fallback-rate analytics.
func (d *Database) migrateAddAttemptColumn(ctx context.Context) error {
	// Try to add the column. SQLite will error if it already exists.
	_, err := d.db.ExecContext(ctx, `ALTER TABLE requests ADD COLUMN attempt INTEGER DEFAULT 1`)
	if err != nil {
		// Ignore "duplicate column" errors
		if strings.Contains(err.Error(), "duplicate column") {
			return nil
		}
		return err
	}
	return nil
}

// migrateAddCacheColumns adds the prompt-cache token columns to existing
// requests tables created before they existed. Idempotent: ignores duplicate
// column errors.
func (d *Database) migrateAddCacheColumns(ctx context.Context) error {
	for _, alter := range []string{
		`ALTER TABLE requests ADD COLUMN cache_read_tokens INTEGER DEFAULT 0`,
		`ALTER TABLE requests ADD COLUMN cache_creation_tokens INTEGER DEFAULT 0`,
	} {
		_, err := d.db.ExecContext(ctx, alter)
		if err != nil && !strings.Contains(err.Error(), "duplicate column") {
			return err
		}
	}
	return nil
}

// migrateAddCostColumn adds the per-request cost column to existing databases.
func (d *Database) migrateAddCostColumn(ctx context.Context) error {
	_, err := d.db.ExecContext(ctx, `ALTER TABLE requests ADD COLUMN cost_usd REAL`)
	if err != nil && !strings.Contains(err.Error(), "duplicate column") {
		return err
	}
	return nil
}

// BackfillRequestCosts fills missing trustworthy per-request costs using the
// same pricing rules as analytics aggregates.
func (d *Database) BackfillRequestCosts(ctx context.Context) (int64, error) {
	baseline := ""
	if !d.analyticsBaseline.IsZero() {
		baseline = d.analyticsBaseline.Format(time.RFC3339Nano)
	}
	rows, err := d.db.QueryContext(ctx, `
		SELECT r.id, r.model,
		       COALESCE(r.input_tokens, 0),
		       COALESCE(r.output_tokens, 0),
		       COALESCE(r.cache_read_tokens, 0),
		       COALESCE(r.cache_creation_tokens, 0),
		       COALESCE(m.cost_input_per_m, 0),
		       COALESCE(m.cost_output_per_m, 0)
		FROM requests r
		LEFT JOIN models m ON m.id = r.model
		WHERE r.cost_usd IS NULL
		  AND (? = '' OR r.start_time >= ?)
	`, baseline, baseline)
	if err != nil {
		return 0, err
	}
	type requestCostRow struct {
		id                                      string
		model                                   string
		input, output, cacheRead, cacheCreation int64
		modelsInputPerM, modelsOutputPerM       float64
	}
	var pending []requestCostRow
	for rows.Next() {
		var row requestCostRow
		if err := rows.Scan(&row.id, &row.model, &row.input, &row.output, &row.cacheRead, &row.cacheCreation, &row.modelsInputPerM, &row.modelsOutputPerM); err != nil {
			_ = rows.Close()
			return 0, err
		}
		pending = append(pending, row)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	if len(pending) == 0 {
		return 0, nil
	}

	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	stmt, err := tx.PrepareContext(ctx, `UPDATE requests SET cost_usd = ? WHERE id = ?`)
	if err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	defer func() { _ = stmt.Close() }()

	var updated int64
	for _, row := range pending {
		cost := costForTokens(row.model, row.input, row.output, row.cacheRead, row.cacheCreation, row.modelsInputPerM, row.modelsOutputPerM)
		res, err := stmt.ExecContext(ctx, cost, row.id)
		if err != nil {
			_ = tx.Rollback()
			return updated, err
		}
		n, _ := res.RowsAffected()
		updated += n
	}
	if err := tx.Commit(); err != nil {
		return updated, err
	}
	return updated, nil
}

func (d *Database) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.db != nil {
		return d.db.Close()
	}
	return nil
}

func (d *Database) DB() *sql.DB {
	return d.db
}

func (d *Database) Path() string {
	return d.path
}

func (d *Database) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	return d.db.BeginTx(ctx, opts)
}

func expandPath(path string) string {
	if len(path) >= 2 && path[:2] == "~/" {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

// priceEntry defines a pricing rule for models whose id/name/display_name
// contains the match substring. Applied only if current cost is 0/NULL.
type priceEntry struct {
	Match      string  `json:"match"`
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cache_read,omitempty"`
	CacheWrite float64 `json:"cache_write,omitempty"`
}

//go:embed seed_prices.json
var defaultModelPrices []byte

// PriceForModel returns the seeded per-1M-token prices (USD) for a model ID,
// matching the longest seed rule whose Match substring occurs in the model ID.
// This is independent of the catalog/models tables so cost figures are always
// available even when a model is absent from the catalog sync. Returns ok=false
// when no rule matches (or embedded data is unavailable).
func PriceForModel(model string) (inputPerM, outputPerM, cacheReadPerM, cacheWritePerM float64, ok bool) {
	if model == "" || len(defaultModelPrices) == 0 {
		return 0, 0, 0, 0, false
	}
	var entries []priceEntry
	if err := json.Unmarshal(defaultModelPrices, &entries); err != nil {
		return 0, 0, 0, 0, false
	}
	bestLen := -1
	for _, e := range entries {
		if e.Match == "" {
			continue
		}
		if strings.Contains(strings.ToLower(model), strings.ToLower(e.Match)) && len(e.Match) > bestLen {
			bestLen = len(e.Match)
			inputPerM, outputPerM, cacheReadPerM, cacheWritePerM, ok = e.Input, e.Output, e.CacheRead, e.CacheWrite, true
		}
	}
	return inputPerM, outputPerM, cacheReadPerM, cacheWritePerM, ok
}

// SeedDefaultModelPrices inserts realistic default pricing for common models
// (GLM, Kimi, Qwen, Grok, DeepSeek, Claude, GPT, MiniMax, Nemotron, MiMo, etc.)
// so that /api/analytics/* endpoints immediately show non-zero USD costs
// without requiring user config or catalog rates.
//
// The seeder is idempotent: it only updates rows where cost_input_per_m or
// cost_output_per_m is NULL or 0. This preserves any prices already set by
// catalog sync (internal/catalog) or user overrides.
//
// Prices are approximate current public list prices (per million tokens, USD)
// sourced from official provider documentation and pricing pages as of
// July 2026: OpenAI (GPT), Anthropic (Claude), Z.ai (GLM), Moonshot (Kimi),
// Alibaba (Qwen), xAI (Grok), DeepSeek, MiniMax, NVIDIA (Nemotron), and
// others. Free-tier variants are explicitly zeroed. Update seed_prices.json
// to refresh values; the JSON is embedded at build time.
func (d *Database) SeedDefaultModelPrices(ctx context.Context) error {
	if len(defaultModelPrices) == 0 {
		return nil
	}

	var entries []priceEntry
	if err := json.Unmarshal(defaultModelPrices, &entries); err != nil {
		return fmt.Errorf("parse seed_prices.json: %w", err)
	}

	for _, e := range entries {
		if e.Match == "" {
			continue
		}
		_, err := d.db.ExecContext(ctx, `
			UPDATE models
			SET cost_input_per_m = ?, cost_output_per_m = ?
			WHERE (cost_input_per_m IS NULL OR cost_input_per_m = 0)
			  AND (id LIKE '%' || ? || '%'
			    OR name LIKE '%' || ? || '%'
			    OR display_name LIKE '%' || ? || '%')
		`, e.Input, e.Output, e.Match, e.Match, e.Match)
		if err != nil {
			return fmt.Errorf("seed price for %q: %w", e.Match, err)
		}
	}
	return nil
}

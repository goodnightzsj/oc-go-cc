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

	"github.com/routatic/proxy/internal/history"
	"github.com/routatic/proxy/internal/site"
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
	RetentionDays   int    `json:"retention_days"` // Negative disables cleanup; zero uses the default.
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
	if err := prepareDatabaseFile(path); err != nil {
		return nil, err
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

	// Lightweight column migrations for databases created by older versions.
	if err := database.migrateColumns(ctx); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("migrate request columns: %w", err)
	}
	if err := database.clearCatalogAPIKeys(ctx); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("clear persisted catalog API keys: %w", err)
	}

	// Seed default model prices so analytics dashboard shows meaningful
	// cost numbers immediately for new/existing installs. Idempotent.
	_ = database.SeedDefaultModelPrices(ctx)
	if _, err := database.BackfillRequestCosts(ctx); err != nil {
		slog.Warn("request cost backfill warning", "err", err)
	}
	if n, err := database.BackfillPeakMultipliers(ctx); err != nil {
		slog.Warn("peak multiplier backfill warning", "err", err)
	} else if n > 0 {
		slog.Info("restored peak markers on imported requests", "rows", n)
	}

	if cfg.VacuumOnStartup {
		if _, err := db.ExecContext(ctx, "VACUUM"); err != nil {
			_ = database.Close()
			return nil, fmt.Errorf("vacuum: %w", err)
		}
	}
	if err := secureDatabaseFiles(path); err != nil {
		_ = database.Close()
		return nil, err
	}

	return database, nil
}

func prepareDatabaseFile(path string) error {
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return fmt.Errorf("prepare database file: %w", err)
	}
	if err := file.Chmod(0600); err != nil {
		_ = file.Close()
		return fmt.Errorf("secure database file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close database file: %w", err)
	}
	return nil
}

func secureDatabaseFiles(path string) error {
	for _, candidate := range []string{path, path + "-wal", path + "-shm"} {
		if err := os.Chmod(candidate, 0600); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("secure database file %q: %w", candidate, err)
		}
	}
	return nil
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
		cost_source TEXT,
		details_known INTEGER NOT NULL DEFAULT 1,
		usage_trusted INTEGER NOT NULL DEFAULT 0,
		streaming INTEGER,
		success INTEGER,
		error_msg TEXT,
		attempt INTEGER DEFAULT 1,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_requests_start_time ON requests(start_time);
	CREATE INDEX IF NOT EXISTS idx_requests_start_instant ON requests(julianday(start_time) DESC, id ASC);
	CREATE INDEX IF NOT EXISTS idx_requests_model ON requests(model);
	CREATE INDEX IF NOT EXISTS idx_requests_created_at ON requests(created_at);

	CREATE TABLE IF NOT EXISTS provider_usage (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		snapshot_at TIMESTAMP NOT NULL,
		observed_at TIMESTAMP NOT NULL,
		provider TEXT,
		plan TEXT,
		model TEXT NOT NULL,
		input_tokens INTEGER NOT NULL DEFAULT 0,
		output_tokens INTEGER NOT NULL DEFAULT 0,
		reasoning_tokens INTEGER NOT NULL DEFAULT 0,
		cache_read_tokens INTEGER NOT NULL DEFAULT 0,
		cache_write_5m_tokens INTEGER NOT NULL DEFAULT 0,
		cache_write_1h_tokens INTEGER NOT NULL DEFAULT 0,
		cost_units INTEGER NOT NULL DEFAULT 0
	);

	CREATE INDEX IF NOT EXISTS idx_provider_usage_observed_at ON provider_usage(observed_at);
	CREATE INDEX IF NOT EXISTS idx_provider_usage_model ON provider_usage(model);


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
	`

	_, err := d.db.ExecContext(ctx, schema)
	return err
}

// migrateColumns adds every column that a database created by an older version
// may be missing. Idempotent: SQLite reports "duplicate column name" when the
// column is already there, and that is the only error tolerated — anything else
// (locked, corrupt, missing table) aborts startup rather than leaving the schema
// half-migrated.
func (d *Database) migrateColumns(ctx context.Context) error {
	for _, alter := range []string{
		`ALTER TABLE requests ADD COLUMN attempt INTEGER DEFAULT 1`,
		`ALTER TABLE requests ADD COLUMN cache_read_tokens INTEGER DEFAULT 0`,
		`ALTER TABLE requests ADD COLUMN cache_creation_tokens INTEGER DEFAULT 0`,
		`ALTER TABLE requests ADD COLUMN cost_usd REAL`,
		`ALTER TABLE requests ADD COLUMN cost_source TEXT`,
		`ALTER TABLE requests ADD COLUMN details_known INTEGER NOT NULL DEFAULT 1`,
		`ALTER TABLE requests ADD COLUMN usage_trusted INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE requests ADD COLUMN peak_multiplier REAL NOT NULL DEFAULT 1`,
	} {
		if _, err := d.db.ExecContext(ctx, alter); err != nil && !strings.Contains(err.Error(), "duplicate column") {
			return fmt.Errorf("%s: %w", alter, err)
		}
	}
	// Rows priced before cost_source existed were all estimates.
	_, err := d.db.ExecContext(ctx, `
		UPDATE requests
		SET cost_source = 'estimated'
		WHERE cost_usd IS NOT NULL AND (cost_source IS NULL OR cost_source = '')
	`)
	return err
}

// clearCatalogAPIKeys removes credentials written by versions that treated the
// catalog as a credential source. Runtime authentication is config-owned.
func (d *Database) clearCatalogAPIKeys(ctx context.Context) error {
	_, err := d.db.ExecContext(ctx, `UPDATE providers SET api_key = NULL WHERE api_key IS NOT NULL`)
	return err
}

// BackfillRequestCosts fills missing trustworthy per-request costs when the
// provider's prices are known. Stored provider costs and unknown prices remain unchanged.
func (d *Database) BackfillRequestCosts(ctx context.Context) (int64, error) {
	baseline := ""
	if !d.analyticsBaseline.IsZero() {
		baseline = d.analyticsBaseline.Format(time.RFC3339Nano)
	}
	rows, err := d.db.QueryContext(ctx, `
		SELECT r.id, r.model, COALESCE(r.provider, ''),
		       COALESCE(r.input_tokens, 0),
		       COALESCE(r.output_tokens, 0),
		       COALESCE(r.cache_read_tokens, 0),
		       COALESCE(r.cache_creation_tokens, 0),
		       m.cost_input_per_m,
		       m.cost_output_per_m,
		       r.start_time
		FROM requests r
		LEFT JOIN models m ON m.name = r.model AND m.provider = COALESCE(NULLIF(r.provider, ''), 'opencode-go')
		WHERE r.cost_usd IS NULL
		  AND (? = '' OR julianday(r.start_time) >= julianday(?) OR r.usage_trusted = 1)
	`, baseline, baseline)
	if err != nil {
		return 0, err
	}
	type requestCostRow struct {
		id   string
		cost float64
	}
	var pending []requestCostRow
	for rows.Next() {
		var id, model, provider, startTime string
		var input, output, cacheRead, cacheCreation int64
		var inputRate, outputRate sql.NullFloat64
		if err := rows.Scan(&id, &model, &provider, &input, &output, &cacheRead, &cacheCreation, &inputRate, &outputRate, &startTime); err != nil {
			_ = rows.Close()
			return 0, err
		}
		cost, known := costForProviderTokensAt(provider, model, input, output, cacheRead, cacheCreation, inputRate, outputRate, parseRequestTime(startTime))
		if known {
			pending = append(pending, requestCostRow{id: id, cost: cost})
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, err
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
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.PrepareContext(ctx, `UPDATE requests SET cost_usd = ?, cost_source = ? WHERE id = ? AND cost_usd IS NULL`)
	if err != nil {
		return 0, err
	}
	defer func() { _ = stmt.Close() }()

	var updated int64
	for _, row := range pending {
		res, err := stmt.ExecContext(ctx, row.cost, CostSourceEstimated, row.id)
		if err != nil {
			return 0, err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return 0, err
		}
		updated += n
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return updated, nil
}

// BackfillPeakMultipliers restores the peak flag on OpenCode Go rows that never
// had it set. Imported billing history arrives with the column default, so a
// request that fell inside the DeepSeek peak window is stored as off-peak and
// everything reading the column - the API, the detail view, analytics - reports
// the off-peak rate for a peak-billed request.
//
// The multiplier is recomputed with history.ProviderPeakMultiplier, the same
// rule live inserts use, so the peak schedule keeps one owner instead of a
// second copy in SQL. Rows already marked peak are left alone: our start_time
// can sit a second or two away from the platform's billing clock at a window
// boundary, and when the two disagree the platform's own figure is the billed
// one.
func (d *Database) BackfillPeakMultipliers(ctx context.Context) (int64, error) {
	// ponytail: rescans the not-yet-peak rows on every startup. A few thousand
	// rows cost nothing; add a schema_info marker if the table grows large.
	rows, err := d.db.QueryContext(ctx, `
		SELECT id, model, COALESCE(provider, ''), start_time
		FROM requests
		WHERE peak_multiplier <= 1
	`)
	if err != nil {
		return 0, err
	}
	type peakRow struct {
		id         string
		multiplier float64
	}
	var pending []peakRow
	for rows.Next() {
		var id, model, provider, startTime string
		if err := rows.Scan(&id, &model, &provider, &startTime); err != nil {
			_ = rows.Close()
			return 0, err
		}
		if multiplier := history.ProviderPeakMultiplier(provider, model, parseRequestTime(startTime)); multiplier > 1 {
			pending = append(pending, peakRow{id: id, multiplier: multiplier})
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, err
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
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.PrepareContext(ctx, `UPDATE requests SET peak_multiplier = ? WHERE id = ? AND peak_multiplier <= 1`)
	if err != nil {
		return 0, err
	}
	defer func() { _ = stmt.Close() }()

	var updated int64
	for _, row := range pending {
		res, err := stmt.ExecContext(ctx, row.multiplier, row.id)
		if err != nil {
			return 0, err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return 0, err
		}
		updated += n
	}
	if err := tx.Commit(); err != nil {
		return 0, err
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
// contains the match substring. Catalog seeding only fills absent Go rates.
//
// Tiers are rates that replace the base ones once the request's input tokens
// reach Size. Both platforms publish them (OpenCode Go "Qwen3.7 Plus (> 256K
// tokens)", CommandCode "Long context > 512K"), and they are not optional
// detail: a long-context request priced at the base rate can be off by 3x.
type priceEntry struct {
	Match      string      `json:"match"`
	Input      float64     `json:"input"`
	Output     float64     `json:"output"`
	CacheRead  float64     `json:"cache_read,omitempty"`
	CacheWrite float64     `json:"cache_write,omitempty"`
	Tiers      []priceTier `json:"tiers,omitempty"`
}

// priceTier is one context-size band. Size is the threshold the band applies
// above, matching how both platforms word it ("Qwen3.7 Plus (> 256K tokens)",
// "Long context > 512K"): a request of exactly Size tokens still bills at the
// base rate, and the highest band the request exceeds wins.
type priceTier struct {
	Size       int64   `json:"size"`
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cache_read,omitempty"`
	CacheWrite float64 `json:"cache_write,omitempty"`
}

// ratesFor returns the band that applies to a request consuming inputTokens
// input tokens. Each field falls back to the base rate independently, because
// the published tables leave cache columns blank for bands that do not change
// them - treating a blank as zero would price cache reads as free.
func (e priceEntry) ratesFor(inputTokens int64) (input, output, cacheRead, cacheWrite float64) {
	input, output, cacheRead, cacheWrite = e.Input, e.Output, e.CacheRead, e.CacheWrite
	best := int64(-1)
	var chosen *priceTier
	for i := range e.Tiers {
		if t := &e.Tiers[i]; inputTokens > t.Size && t.Size > best {
			best, chosen = t.Size, t
		}
	}
	if chosen == nil {
		return input, output, cacheRead, cacheWrite
	}
	input, output = chosen.Input, chosen.Output
	if chosen.CacheRead != 0 {
		cacheRead = chosen.CacheRead
	}
	if chosen.CacheWrite != 0 {
		cacheWrite = chosen.CacheWrite
	}
	return input, output, cacheRead, cacheWrite
}

//go:embed seed_prices_opencode_go.json
var openCodeGoPrices []byte

//go:embed seed_prices_commandcode.json
var commandCodePrices []byte

//go:embed seed_prices_cline_pass.json
var clinePassPrices []byte

// rateTableFiles maps a site's RateTable name to its embedded table. A platform
// absent from this map has no published prices, which is different from having
// prices of zero.
var rateTableFiles = map[string][]byte{
	site.OpenCodeGo:  openCodeGoPrices,
	site.CommandCode: commandCodePrices,
	site.ClinePass:   clinePassPrices,
}

// rateTables parses every embedded table once. Pricing runs per model per
// aggregation row, so re-unmarshalling on every call showed up as pure
// overhead.
var rateTables = sync.OnceValues(func() (map[string][]priceEntry, error) {
	out := make(map[string][]priceEntry, len(rateTableFiles))
	for name, raw := range rateTableFiles {
		var entries []priceEntry
		if err := json.Unmarshal(raw, &entries); err != nil {
			return nil, fmt.Errorf("parse %s price table: %w", name, err)
		}
		out[name] = entries
	}
	return out, nil
})

// PriceForProviderModel returns the published per-1M-token prices (USD) for a
// model on the platform that publishes them, matching the longest rule whose
// Match substring occurs in the model ID.
//
// The provider is part of the lookup, not a filter applied afterwards: the same
// model name carries different rates on different platforms, so a match found
// in the wrong table is worse than no match at all. An unknown or unpriced
// platform therefore returns ok=false, which callers report as an unknown cost
// rather than as free usage.
//
// This is independent of the catalog/models tables so cost figures are always
// available even when a model is absent from the catalog sync.
// inputTokens selects the context band: a model published with several bands
// bills the one its request reaches, so the same model has more than one
// correct price and the caller must say which. Pass 0 for the base band.
func PriceForProviderModel(provider, model string, inputTokens int64) (inputPerM, outputPerM, cacheReadPerM, cacheWritePerM float64, ok bool) {
	descriptor, known := site.Lookup(provider)
	if !known || descriptor.RateTable == "" || model == "" {
		return 0, 0, 0, 0, false
	}
	entries, published := tableFor(descriptor.RateTable)
	if !published {
		return 0, 0, 0, 0, false
	}
	// ok means "a rule matched this model", not "the platform has a table": a
	// platform with a table and no rule for this model is unpriced, and must
	// report unknown rather than free.
	ok = false
	bestLen := -1
	for _, e := range entries {
		if e.Match == "" {
			continue
		}
		if strings.Contains(strings.ToLower(model), strings.ToLower(e.Match)) && len(e.Match) > bestLen {
			bestLen = len(e.Match)
			inputPerM, outputPerM, cacheReadPerM, cacheWritePerM = e.ratesFor(inputTokens)
			ok = true
		}
	}
	return inputPerM, outputPerM, cacheReadPerM, cacheWritePerM, ok
}

// SeedDefaultModelPrices fills missing OpenCode Go catalog rates. Explicit zero
// rates, partial rates, and other providers' prices are left untouched.
//
// Prices are approximate current public list prices (per million tokens, USD)
// sourced from official provider documentation and pricing pages as of
// July 2026: OpenAI (GPT), Anthropic (Claude), Z.ai (GLM), Moonshot (Kimi),
// Alibaba (Qwen), xAI (Grok), DeepSeek, MiniMax, NVIDIA (Nemotron), and
// others. Free-tier variants are explicitly zeroed. Update
// seed_prices_opencode_go.json to refresh values; the JSON is embedded at build
// time.
//
// Only the OpenCode Go table is seeded here: this writes into the synced
// catalog, which carries OpenCode Go's models. CommandCode's models are priced
// straight from its own table at read time and never enter the catalog.
func (d *Database) SeedDefaultModelPrices(ctx context.Context) error {
	tables, err := rateTables()
	if err != nil {
		return err
	}
	entries := tables[site.OpenCodeGo]

	for _, e := range entries {
		if e.Match == "" {
			continue
		}
		_, err := d.db.ExecContext(ctx, `
			UPDATE models
			SET cost_input_per_m = ?, cost_output_per_m = ?
			WHERE provider IN ('', 'opencode-go')
			  AND cost_input_per_m IS NULL AND cost_output_per_m IS NULL
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

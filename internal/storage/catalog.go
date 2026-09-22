package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// CatalogRepo provides methods for persisting and loading catalog provider and model records.
type CatalogRepo struct {
	db *Database
}

// NewCatalogRepo creates a new CatalogRepo backed by the given database.
func NewCatalogRepo(db *Database) *CatalogRepo {
	return &CatalogRepo{db: db}
}

// ProviderRecord represents a provider entry persisted in the catalog.
type ProviderRecord struct {
	Name                   string
	BaseURL                string
	Enabled                *bool
	AnthropicToolsDisabled bool
}

// ModelRecord represents a model entry persisted in the catalog.
type ModelRecord struct {
	ID            string
	Name          string
	Reasoning     bool
	ToolCall      bool
	Vision        bool
	ContextWindow int64
	// MaxOutputTokens is the model's published output ceiling. It is persisted
	// separately from ContextWindow because the two answer different questions:
	// the context window gates whether a request fits, and this gates how much
	// the model may produce. Dropping it lets a per-request max_tokens through
	// that the upstream will reject.
	MaxOutputTokens int64
	Rates           *Rates
	Tiers           []PromptTier
}

// Provider holds a provider's configuration as loaded from the database.
type Provider struct {
	Name                   string
	BaseURL                string
	Enabled                *bool
	AnthropicToolsDisabled bool
}

// Modalities describes the input and output data types a model supports.
type Modalities struct {
	Input  []string
	Output []string
}

// Limit describes the context window and output token limits for a model.
type Limit struct {
	Context int64
	Output  int64
}

// Rates holds per-million-token pricing for a model.
type Rates struct {
	Input  float64
	Output float64
}

// Model holds a full model definition with nested limit and pricing details.
type Model struct {
	ID         string
	Name       string
	Reasoning  bool
	ToolCall   bool
	Vision     bool
	Modalities Modalities
	Limit      *Limit
	Rates      *Rates
	// Tiers are the long-context price bands, ordered by ascending threshold.
	// They live beside Rates because they are the same kind of fact - what a
	// token costs - conditioned on how many tokens there are. A model with no
	// tiers is flat-rated, which is different from one whose tiers failed to
	// load, so the empty slice and the nil slice are not distinguished here but
	// the flag below is.
	Tiers []PromptTier
}

// PromptTier is one long-context band, as stored.
type PromptTier struct {
	MinPromptTokens int64   `json:"min_prompt_tokens"`
	Input           float64 `json:"input"`
	Output          float64 `json:"output"`
	CacheRead       float64 `json:"cache_read,omitempty"`
}

// Catalog holds the parsed provider and model maps as loaded from the database.
type Catalog struct {
	Providers map[string]Provider
	Models    map[string]Model
}

// IndexedCatalog extends Catalog with an index from provider name to its models.
type IndexedCatalog struct {
	Catalog
	ProviderModels map[string][]Model
}

// ContextWindow returns the model's context window limit, or 0 if unknown.
func (m Model) ContextWindow() int64 {
	if m.Limit != nil {
		return m.Limit.Context
	}
	return 0
}

// MaxOutputTokens returns the model's published output ceiling, or 0 if
// unknown. Zero means "not recorded", never "may not produce output".
func (m Model) MaxOutputTokens() int64 {
	if m.Limit != nil {
		return m.Limit.Output
	}
	return 0
}

// CostInputPerM returns the input cost per million tokens, or 0 if unknown.
func (m Model) CostInputPerM() float64 {
	if m.Rates != nil {
		return m.Rates.Input
	}
	return 0
}

// CostOutputPerM returns the output cost per million tokens, or 0 if unknown.
func (m Model) CostOutputPerM() float64 {
	if m.Rates != nil {
		return m.Rates.Output
	}
	return 0
}

// ReplaceBatch atomically replaces the provider and model catalog.
func (r *CatalogRepo) ReplaceBatch(ctx context.Context, providers []ProviderRecord, models []ModelRecord) error {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC().Format(time.RFC3339)

	if _, err := tx.ExecContext(ctx, `DELETE FROM models`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM providers`); err != nil {
		return err
	}

	for _, p := range providers {
		enabled := 1
		if p.Enabled != nil && !*p.Enabled {
			enabled = 0
		}
		anthropicToolsDisabled := 0
		if p.AnthropicToolsDisabled {
			anthropicToolsDisabled = 1
		}

		_, err := tx.ExecContext(ctx, `
			INSERT INTO providers (name, base_url, api_key, enabled, anthropic_tools_disabled, created_at)
			VALUES (?, ?, NULL, ?, ?, ?)
			ON CONFLICT(name) DO UPDATE SET
				base_url = excluded.base_url,
				api_key = NULL,
				enabled = excluded.enabled,
				anthropic_tools_disabled = excluded.anthropic_tools_disabled
		`, p.Name, p.BaseURL, enabled, anthropicToolsDisabled, now)
		if err != nil {
			return err
		}
	}

	for _, m := range models {
		provider := providerFromModelKey(m.ID)
		modelName := ModelNameFromKey(m.ID)
		var costInput, costOutput, maxOutput any
		if m.Rates != nil {
			costInput, costOutput = m.Rates.Input, m.Rates.Output
		}
		// Written as NULL rather than 0 when unpublished, so a reader can tell
		// "no ceiling recorded" from "ceiling is zero".
		if m.MaxOutputTokens > 0 {
			maxOutput = m.MaxOutputTokens
		}

		supportsTools := 1
		if !m.ToolCall {
			supportsTools = 0
		}
		supportsVision := 0
		if m.Vision {
			supportsVision = 1
		}
		supportsReasoning := 0
		if m.Reasoning {
			supportsReasoning = 1
		}

		_, err := tx.ExecContext(ctx, `
			INSERT OR REPLACE INTO models (id, provider, name, display_name, context_window, max_output_tokens, cost_input_per_m, cost_output_per_m, cost_tiers, supports_tools, supports_vision, supports_reasoning, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, COALESCE((SELECT created_at FROM models WHERE id = ?), ?))
		`,
			m.ID, provider, modelName, m.Name, m.ContextWindow, maxOutput, costInput, costOutput,
			encodeTiers(m.Tiers), supportsTools, supportsVision, supportsReasoning, m.ID, now)
		if err != nil {
			return err
		}
	}

	_, err = tx.ExecContext(ctx, `
		INSERT OR REPLACE INTO schema_info (key, value) VALUES ('catalog_last_sync', ?)
	`, now)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// Load reads all providers and models from the database and returns an indexed catalog.
func (r *CatalogRepo) Load(ctx context.Context) (*IndexedCatalog, error) {
	providers := make(map[string]Provider)
	models := make(map[string]Model)

	rows, err := r.db.DB().QueryContext(ctx, `
		SELECT name, base_url, enabled, anthropic_tools_disabled
		FROM providers
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var p Provider
		var enabled sql.NullBool
		var anthropicToolsDisabled int

		if err := rows.Scan(&p.Name, &p.BaseURL, &enabled, &anthropicToolsDisabled); err != nil {
			return nil, err
		}
		if enabled.Valid {
			p.Enabled = &enabled.Bool
		}
		p.AnthropicToolsDisabled = anthropicToolsDisabled == 1
		providers[p.Name] = p
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	rows, err = r.db.DB().QueryContext(ctx, `
		SELECT id, provider, name, context_window, max_output_tokens, cost_input_per_m, cost_output_per_m, cost_tiers,
		       supports_tools, supports_vision, supports_reasoning
		FROM models
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var m Model
		var provider string
		var displayName string
		var contextWindow, maxOutputTokens sql.NullInt64
		var costInput, costOutput sql.NullFloat64
		var tiersJSON sql.NullString
		var supportsTools, supportsVision, supportsReasoning int

		if err := rows.Scan(&m.ID, &provider, &displayName, &contextWindow, &maxOutputTokens, &costInput, &costOutput, &tiersJSON,
			&supportsTools, &supportsVision, &supportsReasoning); err != nil {
			return nil, err
		}
		m.Name = displayName
		m.ToolCall = supportsTools == 1
		m.Reasoning = supportsReasoning == 1
		m.Vision = supportsVision == 1

		if contextWindow.Valid || maxOutputTokens.Valid {
			// A limit carrying only one of the two is still meaningful: the
			// caller treats a zero as unknown, not as "cannot produce output".
			m.Limit = &Limit{Context: contextWindow.Int64, Output: maxOutputTokens.Int64}
		}
		if costInput.Valid && costOutput.Valid {
			m.Rates = &Rates{Input: costInput.Float64, Output: costOutput.Float64}
		}
		m.Tiers = decodeTiers(tiersJSON)

		if m.Vision {
			m.Modalities.Input = []string{"text", "image"}
		} else {
			m.Modalities.Input = []string{"text"}
		}
		m.Modalities.Output = []string{"text"}

		models[m.ID] = m
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(providers) == 0 {
		return nil, errors.New("catalog providers map is empty")
	}
	if len(models) == 0 {
		return nil, errors.New("catalog models map is empty")
	}

	for key := range models {
		prov := providerFromModelKey(key)
		if prov == "" {
			return nil, errors.New("model key missing provider prefix")
		}
		if _, ok := providers[prov]; !ok {
			return nil, errors.New("model references unknown provider")
		}
	}

	cat := &Catalog{
		Providers: providers,
		Models:    models,
	}

	idx := &IndexedCatalog{
		Catalog:        *cat,
		ProviderModels: make(map[string][]Model, len(providers)),
	}

	for key, model := range models {
		prov := providerFromModelKey(key)
		if prov != "" {
			idx.ProviderModels[prov] = append(idx.ProviderModels[prov], model)
		}
	}

	return idx, nil
}

// LastSync returns the timestamp of the last catalog sync, or zero time if never synced.
func (r *CatalogRepo) LastSync(ctx context.Context) (time.Time, error) {
	var syncedAt sql.NullString
	err := r.db.DB().QueryRowContext(ctx, `
		SELECT value FROM schema_info WHERE key = 'catalog_last_sync'
	`).Scan(&syncedAt)
	if err == sql.ErrNoRows {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}

	if !syncedAt.Valid {
		return time.Time{}, nil
	}

	return time.Parse(time.RFC3339, syncedAt.String)
}

func providerFromModelKey(key string) string {
	provider, _, ok := strings.Cut(key, "/")
	if !ok {
		return ""
	}
	return provider
}

// ProviderModel is a flattened model entry used for display in provider listings.
type ProviderModel struct {
	ModelID     string
	DisplayName string
	ToolCall    bool
	Reasoning   bool
	Vision      bool
	Context     int64
	CostInput   float64
	CostOutput  float64
}

// ListProviderModels returns a flattened ProviderModel slice for the given provider.
func (ic *IndexedCatalog) ListProviderModels(provider string) []ProviderModel {
	models := ic.ProviderModels[provider]
	if models == nil {
		return nil
	}
	result := make([]ProviderModel, len(models))
	for i, m := range models {
		result[i] = ProviderModel{
			ModelID:     ModelNameFromKey(m.ID),
			DisplayName: m.Name,
			ToolCall:    m.ToolCall,
			Reasoning:   m.Reasoning,
			Vision:      m.Vision,
		}
		if m.Limit != nil {
			result[i].Context = m.Limit.Context
		}
		if m.Rates != nil {
			result[i].CostInput = m.Rates.Input
			result[i].CostOutput = m.Rates.Output
		}
	}
	return result
}

// ModelNameFromKey extracts the model name portion from a model key of the form "provider/model-name".
func ModelNameFromKey(key string) string {
	_, name, ok := strings.Cut(key, "/")
	if !ok {
		return key
	}
	return name
}

// encodeTiers stores the bands as JSON, or NULL when there are none. NULL and
// "[]" both decode to no tiers; NULL is written so a flat-rated model is
// distinguishable in the file from one that was never synced with this column.
func encodeTiers(tiers []PromptTier) any {
	if len(tiers) == 0 {
		return nil
	}
	raw, err := json.Marshal(tiers)
	if err != nil {
		// Unreachable for this shape, and a silently dropped tier is a wrong
		// price rather than a missing one, so fail loudly instead.
		panic(fmt.Sprintf("encode prompt tiers: %v", err))
	}
	return string(raw)
}

// decodeTiers reads the bands back. A malformed value yields no tiers rather
// than an error: the caller is a pricing path, and the honest answer for "the
// bands could not be read" is the base rate, which is what no tiers means.
// Storing a corrupt value is guarded at the write, not the read.
func decodeTiers(raw sql.NullString) []PromptTier {
	if !raw.Valid || raw.String == "" {
		return nil
	}
	var out []PromptTier
	if err := json.Unmarshal([]byte(raw.String), &out); err != nil {
		slog.Warn("model cost tiers are unreadable; pricing at the base rate", "error", err)
		return nil
	}
	return out
}

// tierRatesFor returns the band that applies to a prompt of this size, or nil
// when no band does.
//
// A band applies strictly above its threshold, matching how models.dev words
// the condition, and the highest applicable threshold wins. The prompt size
// passed in must be the whole prompt - fresh input plus cache reads plus cache
// writes - because that is the quantity the published threshold is written
// against.
func tierRatesFor(tiers []PromptTier, promptTokens int64) *PromptTier {
	var best *PromptTier
	for i := range tiers {
		t := &tiers[i]
		if t.MinPromptTokens <= 0 || promptTokens <= t.MinPromptTokens {
			continue
		}
		if best == nil || t.MinPromptTokens > best.MinPromptTokens {
			best = t
		}
	}
	return best
}

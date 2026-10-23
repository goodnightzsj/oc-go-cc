package router

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/routatic/proxy/internal/catalog"
	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/site"
	"github.com/routatic/proxy/internal/storage"
)

var ErrUnknownProvider = errors.New("unknown provider")

type ModelRouter struct {
	atomic      *config.AtomicConfig
	db          *storage.Database
	catalogPath string
	catMu       sync.Mutex
	cat         *catalog.IndexedCatalog
	catErr      error
	catCache    time.Time

	siteMu          sync.Mutex
	siteCache       []catalog.SiteModel
	siteAt          time.Time
	siteCacheActive string
}

// siteModelsTTL bounds how often a platform's own model list is re-read. The
// endpoints are not meant to be polled, and a model list changes on the order
// of days, so the listing is served from this cache.
const siteModelsTTL = 10 * time.Minute

// siteModelsFailureTTL is how long a failed fetch is remembered. Retrying on
// every client refresh would add a round trip to each one, but caching the
// failure for the full TTL would hide a transient outage for ten minutes.
const siteModelsFailureTTL = time.Minute

// siteModelsClient is separate from the sending client: this call carries no
// credential and must not inherit the retry or timeout policy of an inference
// request.
var siteModelsClient = &http.Client{Timeout: 10 * time.Second}

// activeSiteScope returns the platform the model listing is restricted to, or
// "" when routing is unrestricted.
//
// The value is the normalized id, so it compares directly against what
// NormalizeProvider yields for a target's provider - the same comparison
// RestrictToActiveSite makes on the routing chain. Deriving both from one
// helper is what keeps the listing and routing from disagreeing: a model the
// listing offers but the chain drops is a picker entry that fails on use.
func activeSiteScope(cfg *config.Config) string {
	if cfg == nil || strings.TrimSpace(cfg.ActiveSite) == "" {
		return ""
	}
	return site.Normalize(cfg.ActiveSite)
}

// siteModels returns the models a platform publishes on its own API.
// CommandCode is the reason this exists: it is absent from the models.dev
// catalog, so without it the model listing offered nothing for that platform
// and a client switching to it saw an empty picker.
//
// When routing is scoped to one platform, only that platform is asked. The
// others cannot serve a request in that state, so fetching them would spend a
// round trip per platform to fill a picker with entries that fail on use.
//
// A platform is only asked when it has credentials and a derivable endpoint,
// and a failure is logged rather than returned: this feeds a listing, and one
// unreachable platform must not empty the whole answer.
func (r *ModelRouter) siteModels(ctx context.Context) []catalog.SiteModel {
	r.siteMu.Lock()
	defer r.siteMu.Unlock()

	cfg := r.atomic.Get()
	active := activeSiteScope(cfg)
	ttl := siteModelsTTL
	if r.siteCache == nil {
		ttl = siteModelsFailureTTL
	}
	// The cache holds one platform's list under a scope, so a change of active
	// site must re-read rather than serve the previous platform's models.
	if r.siteCacheActive != active {
		r.siteCache, r.siteAt = nil, time.Time{}
	}
	if r.siteCache != nil || !r.siteAt.IsZero() {
		if time.Since(r.siteAt) < ttl {
			return r.siteCache
		}
	}

	var out []catalog.SiteModel
	for _, descriptor := range scopedDescriptors(active) {
		if len(cfg.ProviderAPIKeys(descriptor.ID)) == 0 {
			continue
		}
		endpoint, err := catalog.SiteModelsURL(descriptor.ID, cfg.ProviderModelEndpoint(descriptor.ID))
		if err != nil {
			continue // this platform has no model-list endpoint of its own
		}
		fetchCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		models, err := catalog.FetchSiteModelsWithKey(fetchCtx, siteModelsClient, descriptor.ID, endpoint,
			modelListKey(cfg, descriptor.ID))
		cancel()
		if err != nil {
			slog.Warn("platform model list unavailable", "platform", descriptor.ID, "err", err)
			continue
		}
		out = append(out, models...)
	}
	r.siteCache, r.siteAt, r.siteCacheActive = out, time.Now(), active
	return out
}

// scopedDescriptors lists the platforms whose published models belong in the
// listing: every visible one when routing is unrestricted, otherwise exactly
// the active platform. Lookup rather than a visible-only scan so a platform
// hidden from the dashboard still reports its models while it is the active
// site - routing does not consult visibility, and the listing must not
// contradict it.
// modelListKey returns the credential to present when fetching a platform's
// model roster, or "" when the roster is public.
//
// The key is only needed where the list belongs to an account rather than the
// platform. Sending it unconditionally would hand a credential to the public
// endpoints that never asked for one, so the answer is a per-platform fact and
// not "always send the first key".
func modelListKey(cfg *config.Config, provider string) string {
	if cfg == nil || site.Normalize(provider) != site.WorkBuddy {
		return ""
	}
	if keys := cfg.ProviderAPIKeys(provider); len(keys) > 0 {
		return keys[0]
	}
	return ""
}

func scopedDescriptors(active string) []site.Descriptor {
	if active == "" {
		return site.Visible()
	}
	if descriptor, ok := site.Lookup(active); ok {
		return []site.Descriptor{descriptor}
	}
	return nil
}

func NewModelRouter(atomic *config.AtomicConfig) *ModelRouter {
	return &ModelRouter{atomic: atomic}
}

func NewModelRouterWithDB(atomic *config.AtomicConfig, db *storage.Database) *ModelRouter {
	return &ModelRouter{atomic: atomic, db: db}
}

func NewModelRouterWithCatalog(atomic *config.AtomicConfig, catalogPath string) *ModelRouter {
	return &ModelRouter{atomic: atomic, catalogPath: catalogPath}
}

func (r *ModelRouter) catalog(ctx context.Context) (*catalog.IndexedCatalog, error) {
	if r.db == nil && r.catalogPath == "" {
		slog.Warn("catalog not available — model resolution falling back to legacy config")
		return nil, nil
	}

	r.catMu.Lock()
	defer r.catMu.Unlock()

	if r.cat != nil && time.Since(r.catCache) < 30*time.Second {
		return r.cat, nil
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if r.db != nil {
		r.cat, r.catErr = catalog.LoadFromSQLite(ctx, r.db)
	} else if r.catalogPath != "" {
		r.cat, r.catErr = catalog.Load(r.catalogPath)
	}
	if r.catErr == nil {
		r.catCache = time.Now()
	}
	return r.cat, r.catErr
}

// isRespectRequestedModel returns true when the client-specified model should be
// used as the primary routing target.  nil (unset in config) defaults to true;
// an explicit *false from the user config is honoured.
func isRespectRequestedModel(cfg *config.Config) bool {
	if cfg.RespectRequestedModel == nil {
		return true // default when not explicitly set
	}
	return *cfg.RespectRequestedModel
}

// RouteResult contains the selected model and fallback chain.
type RouteResult struct {
	Primary   config.ModelConfig
	Fallbacks []config.ModelConfig
	Scenario  Scenario
}

// resolveRequestedModel checks if the user-specified model should override
// scenario-based routing. Returns the route result and true if it matched,
// or zero value and false if scenario routing should proceed normally.
func (r *ModelRouter) resolveRequestedModel(cfg *config.Config, requestedModel string, needsVision bool) (RouteResult, bool, error) {
	if !isRespectRequestedModel(cfg) || requestedModel == "" {
		return RouteResult{}, false, nil
	}

	// Look up the requested model in config to inherit its settings
	primary, ok := cfg.Models[requestedModel]
	canonicalRequestedModel := config.CanonicalModelID(requestedModel)
	if !ok && canonicalRequestedModel != requestedModel {
		primary, ok = cfg.Models[canonicalRequestedModel]
	}
	if !ok {
		// Not in legacy config — try the catalog before falling back to the
		// legacy unknown-model behavior. Provider-qualified references that
		// fail catalog resolution are rejected with a clear error instead of
		// silently falling back to a bogus provider.
		sel, parseErr := catalog.ParseModelRef(requestedModel)
		providerQualified := parseErr == nil && sel.Provider != ""

		cat, _ := r.catalog(context.Background())
		if cat != nil {
			if catalogPrimary, catalogOk := r.resolveFromCatalog(cat, requestedModel, sel); catalogOk {
				primary = catalogPrimary
			} else if providerQualified {
				return RouteResult{}, false, fmt.Errorf("model reference %q uses unknown provider %q: %w", requestedModel, sel.Provider, ErrUnknownProvider)
			} else {
				primary = r.legacyUnknownModelConfig(cfg, canonicalRequestedModel)
			}
		} else if providerQualified {
			return RouteResult{}, false, fmt.Errorf("model reference %q uses unknown provider %q: %w", requestedModel, sel.Provider, ErrUnknownProvider)
		} else {
			primary = r.legacyUnknownModelConfig(cfg, canonicalRequestedModel)
		}
	}
	primary = config.ResolveModelConfig(primary)
	// Only a model known to lack vision is refused here. An unstated capability
	// must not refuse the request - the platform can say no itself, and it is the
	// only party that actually knows.
	if vision, known := config.ModelHasVision(primary); needsVision && known && !vision {
		return RouteResult{}, false, fmt.Errorf("requested model %s does not support vision", primary.ModelID)
	}

	fallbacks := cfg.Fallbacks["default"]

	return RouteResult{
		Primary:   primary,
		Fallbacks: fallbacks,
		Scenario:  ScenarioDefault,
	}, true, nil
}

// resolvedModelToConfig converts a catalog resolved model into a runtime
// ModelConfig used by the router.
func resolvedModelToConfig(resolved catalog.ResolvedModel) config.ModelConfig {
	supportsTools := resolved.Tools
	vision := resolved.Vision
	return config.ModelConfig{
		Provider:      resolved.Provider,
		ModelID:       resolved.ModelID,
		ModelRef:      resolved.CanonicalName,
		Vision:        &vision,
		ContextWindow: int(resolved.ContextWindow),
		// Carried so clampOutputTokens can honour the upstream's real ceiling
		// instead of clamping a client's request down to the built-in default.
		MaxOutputTokens: int(resolved.MaxOutputTokens),
		SupportsTools:   &supportsTools,
		// The catalog records this per platform. Dropping it here was the second
		// half of a broken chain: core.ModelWireFormat already learned to honour
		// the flag, and it never saw a target that carried it.
		AnthropicToolsDisabled: resolved.AnthropicToolsDisabled,
	}
}

// requestConstraints maps request-level requirements to scenario constraints
// used by the cost-based selector.
func requestConstraints(messages []MessageContent, tokenCount int) ScenarioConstraints {
	facts := AnalyzeRequestFacts(messages)
	constraints := ScenarioConstraints{
		Vision:  facts.NeedsVision,
		Context: int64(tokenCount),
	}
	latest := latestUserMessages(messages)
	if hasThinkingPattern(latest) {
		constraints.Reasoning = true
	}
	if hasToolUsage(messages) {
		constraints.Tools = true
	}
	return constraints
}

// hasToolUsage reports whether the request likely requires tool support based
// on message roles or tool-related keywords.
func hasToolUsage(messages []MessageContent) bool {
	toolKeywords := []string{
		"tool", "function", "execute", "run command",
	}
	for _, msg := range messages {
		if msg.ToolsDeclared {
			return true
		}
		if msg.Role == "tool" || msg.Role == "function" {
			return true
		}
		lower := strings.ToLower(msg.Content)
		for _, kw := range toolKeywords {
			if strings.Contains(lower, kw) {
				return true
			}
		}
	}
	return false
}

// resolveFromCatalog attempts to resolve a requested model string through the
// catalog. It returns the model config and true on success, otherwise false.
func (r *ModelRouter) resolveFromCatalog(cat *catalog.IndexedCatalog, requestedModel string, sel catalog.Selector) (config.ModelConfig, bool) {
	var resolved catalog.ResolvedModel
	var err error
	if sel.Provider != "" {
		resolved, err = cat.Resolve(sel)
	} else {
		resolved, err = cat.ResolveShort(requestedModel)
	}
	if err != nil {
		// The client may have used the platform's own spelling, which carries
		// the platform's prefix where the catalog carries the vendor's. Falling
		// through to legacyUnknownModelConfig here is what left such a request
		// with no capabilities at all - and a vision-capable model was then
		// refused for not supporting vision.
		//
		// The name is resolved, not the target: the catalog lists this id under
		// many providers, so asking it which one to use has no right answer and
		// ResolveShort refuses. The client already said which platform it meant.
		platform, short := splitPlatformPrefix(requestedModel)
		if platform == "" {
			return config.ModelConfig{}, false
		}
		model, ok := cat.ModelByName(short)
		if !ok {
			return config.ModelConfig{}, false
		}
		// The wire id goes through WireModelID for the same reason every other
		// path does - that prefix is part of the id on ClinePass and a
		// namespace everywhere else.
		target := config.ModelConfig{
			Provider: platform,
			ModelID:  catalog.WireModelID(platform + "/" + short),
			// ModelRef marks these capabilities as having an authority behind
			// them, so ResolveModelConfig does not overwrite them with the
			// built-in family table.
			ModelRef: platform + "/" + short,
		}
		return applyCatalogModel(target, model), true
	}

	cfg := resolvedModelToConfig(resolved)
	cfg.ModelRef = requestedModel
	return cfg, true
}

// splitPlatformPrefix returns the platform and the remainder when a model id
// carries a known platform's own prefix, or two empty strings when it does not.
//
// Only a registered platform counts. A vendor namespace looks identical
// ("deepseek/deepseek-v4.1-flash-1017") and must be left alone: stripping it
// would search for a model named after the vendor, not after the model.
func splitPlatformPrefix(modelID string) (string, string) {
	idx := strings.IndexByte(modelID, '/')
	if idx <= 0 || idx == len(modelID)-1 {
		return "", ""
	}
	prefix := modelID[:idx]
	if !site.IsKnown(prefix) {
		return "", ""
	}
	return site.Normalize(prefix), modelID[idx+1:]
}

// legacyUnknownModelConfig builds a bare target for a model no source knows,
// inheriting Temperature and MaxTokens from the default model when available.
// The platform is the active site when routing is scoped - the operator has
// already said which platform they want - and the default platform otherwise,
// which is what an unset provider has always meant.
func (r *ModelRouter) legacyUnknownModelConfig(cfg *config.Config, requestedModel string) config.ModelConfig {
	provider := site.DefaultID()
	if cfg.ActiveSite != "" {
		provider = cfg.ActiveSite
	}
	primary := config.ModelConfig{
		Provider: provider,
		ModelID:  requestedModel,
	}
	if def, ok := cfg.Models["default"]; ok {
		primary.Temperature = def.Temperature
		primary.MaxTokens = def.MaxTokens
	}
	return primary
}

// Route determines which model to use for a request.
// If respect_requested_model is enabled and requestedModel is provided, it overrides scenario-based routing.
func (r *ModelRouter) Route(messages []MessageContent, tokenCount int, requestedModel string) (RouteResult, error) {
	cfg := r.atomic.Get()
	facts := AnalyzeRequestFacts(messages)

	if result, ok, err := r.resolveRequestedModel(cfg, requestedModel, facts.NeedsVision); err != nil {
		return RouteResult{}, err
	} else if ok {
		return result, nil
	}

	// Otherwise, use scenario-based routing
	scenario := DetectScenario(messages, tokenCount, cfg)
	scenarioKey := string(scenario)

	// Get primary model for scenario. When cost-based routing is enabled and
	// a non-empty catalog is available, prefer the cheapest matching catalog
	// model while preserving the legacy fallback chain.
	primary, ok := cfg.Models[scenarioKey]
	if cat, catErr := r.catalog(context.Background()); cfg.CostBasedRoutingEnabled() && cat != nil && catErr == nil && len(cat.Models) > 0 {
		constraints := requestConstraints(messages, tokenCount)
		selector := NewSelector(cat, cfg)
		resolved, err := selector.SelectCheapest(scenarioKey, constraints)
		switch err {
		case nil:
			primary = resolvedModelToConfig(resolved)
			ok = true
		default:
			// Never swallow this: a silent failure here is indistinguishable
			// from cost routing working, which is how it went unnoticed that
			// the feature was a no-op.
			slog.Warn("cost-based routing found no model, using scenario model",
				"scenario", scenarioKey, "error", err)
		}
	}

	if !ok {
		if isVisionScenario(scenario) {
			return RouteResult{}, fmt.Errorf("vision scenario %s is not configured", scenario)
		}
		// Fall back to default if scenario model not configured
		primary, ok = cfg.Models["default"]
		if !ok {
			return RouteResult{}, fmt.Errorf("no default model configured")
		}
	}

	// Get fallbacks for scenario
	fallbacks := cfg.Fallbacks[scenarioKey]
	if len(fallbacks) == 0 {
		if isVisionScenario(scenario) {
			return RouteResult{}, fmt.Errorf("vision scenario %s has no configured vision fallbacks", scenario)
		}
		// Fall back to default fallbacks
		fallbacks = cfg.Fallbacks["default"]
	}

	return RouteResult{
		Primary:   primary,
		Fallbacks: fallbacks,
		Scenario:  scenario,
	}, nil
}

// IsStreamingScenarioRoutingEnabled returns whether streaming requests should use
// scenario-based routing instead of always routing to the fast model.
func (r *ModelRouter) IsStreamingScenarioRoutingEnabled() bool {
	return r.atomic.Get().EnableStreamingScenarioRouting
}

// RouteWithOverride checks if the requested model matches a model_overrides entry.
//
// When matched, the returned RouteResult uses the override ModelConfig as the
// primary. The fallback chain is fallbacks[<requestedModel>], falling back to
// fallbacks["default"] when the override key has no entry (matching the
// behavior of Route and RouteForStreaming). The caller (MessagesHandler) is
// expected to merge a scenario-derived safety-net chain on top.
//
// Returns the override RouteResult and true if matched, or a zero value and
// false if the requested model has no entry in model_overrides.
func (r *ModelRouter) RouteWithOverride(requestedModel string) (RouteResult, bool) {
	cfg := r.atomic.Get()
	if cfg.ModelOverrides == nil {
		return RouteResult{}, false
	}
	override, ok := cfg.ModelOverrides[requestedModel]
	if !ok {
		return RouteResult{}, false
	}
	return buildOverrideResult(cfg, override, requestedModel), true
}

// RouteWithFamilyOverride checks whether the requested model string contains a
// Claude family keyword configured in model_family_overrides (e.g. "opus",
// "sonnet", "haiku"). Matching is a case-insensitive substring test, so it maps
// the versioned model IDs Claude Code sends (claude-opus-4-20250514) without
// requiring cc-switch to rewrite the model to an exact string.
//
// Family keys are evaluated longest-first so that overlapping keys resolve
// deterministically to the most specific match. The fallback chain is
// fallbacks[<family>], falling back to fallbacks["default"] when the family key
// has no entry (matching RouteWithOverride behavior).
//
// Returns the RouteResult and true if matched, or a zero value and false when
// no family keyword appears in the requested model.
func (r *ModelRouter) RouteWithFamilyOverride(requestedModel string) (RouteResult, bool) {
	cfg := r.atomic.Get()
	if len(cfg.ModelFamilyOverrides) == 0 || requestedModel == "" {
		return RouteResult{}, false
	}

	families := slices.Collect(maps.Keys(cfg.ModelFamilyOverrides))
	slices.SortFunc(families, func(a, b string) int {
		// Longest key first, then lexicographic for determinism.
		return cmp.Or(cmp.Compare(len(b), len(a)), cmp.Compare(a, b))
	})

	lower := strings.ToLower(requestedModel)
	for _, family := range families {
		if family == "" {
			continue
		}
		if strings.Contains(lower, strings.ToLower(family)) {
			return buildOverrideResult(cfg, cfg.ModelFamilyOverrides[family], family), true
		}
	}
	return RouteResult{}, false
}

// buildOverrideResult constructs a RouteResult for an override match. The
// fallback chain is fallbacks[fallbackKey], falling back to fallbacks["default"]
// when the key has no entry.
func buildOverrideResult(cfg *config.Config, override config.ModelConfig, fallbackKey string) RouteResult {
	fallbacks := cfg.Fallbacks[fallbackKey]
	if len(fallbacks) == 0 {
		fallbacks = cfg.Fallbacks["default"]
	}
	return RouteResult{
		Primary:   override,
		Fallbacks: fallbacks,
		Scenario:  ScenarioOverride,
	}
}

// ModelInfo describes a model that clients can request by name. It is the
// data source for the OpenAI-compatible /v1/models listing consumed by tools
// such as CC-Switch's "Fetch Models" button.
type ModelInfo struct {
	// ID is the string a client puts in the request "model" field.
	ID string
	// DisplayName is a human-readable label when available.
	DisplayName string
	// Provider is the upstream provider that serves the model, when known.
	Provider string
}

// ListModels returns the set of model identifiers a client may request,
// deduplicated and sorted by ID. The list is assembled from:
//
//   - legacy config "models" aliases,
//   - "model_overrides" keys (the Claude aliases users pin),
//   - catalog canonical names (provider/model), when a catalog is available.
//
// Any of these is a valid value for the request "model" field, so surfacing
// all of them lets a picker present every route the proxy understands.
//
// When the config scopes routing to one platform, the listing is scoped the
// same way: offering an id that RestrictToActiveSite would drop creates a
// picker entry that fails when used, which is worse than not offering it.
// The alias sources are filtered by their resolved target rather than by their
// name, because an alias is a client-facing label - "claude-opus-4-8" says
// nothing about which platform serves it.
//
// ctx bounds the catalog load; when the caller (e.g. an HTTP handler) cancels
// it, an in-flight catalog read is abandoned rather than churning to completion.
func (r *ModelRouter) ListModels(ctx context.Context) []ModelInfo {
	cfg := r.atomic.Get()
	active := activeSiteScope(cfg)
	seen := make(map[string]ModelInfo)

	add := func(id, name, provider string) {
		if id == "" {
			return
		}
		// A scoped listing drops every other platform, including the catalog
		// entries: they name models the active site cannot serve.
		if active != "" && site.Normalize(provider) != active {
			return
		}
		existing, ok := seen[id]
		if !ok {
			seen[id] = ModelInfo{ID: id, DisplayName: name, Provider: provider}
			return
		}
		// Fill in missing fields from later sources without overwriting.
		if existing.DisplayName == "" {
			existing.DisplayName = name
		}
		if existing.Provider == "" {
			existing.Provider = provider
		}
		seen[id] = existing
	}

	// ModelOverrides is walked before Models so that, for a key present in
	// both, the override's provider is what surfaces in the listing — matching
	// the routing precedence (model_overrides wins). add() keeps the first
	// source's fields, so first-write must be the winning source.
	for alias, mc := range cfg.ModelOverrides {
		add(alias, "", mc.Provider)
	}
	for alias, mc := range cfg.Models {
		add(alias, "", mc.Provider)
	}

	// A platform that publishes its own model list is the authority for its own
	// models, and the catalog does not know it at all.
	//
	// It is also the authority on which of its models still exist. The catalog
	// mirrors a third party and lags: models.dev still carries four cline-pass
	// models the platform retired, and they would otherwise stay in the picker
	// long after they stopped being callable. So a platform's own roster
	// restricts what the catalog may contribute for that platform.
	//
	// Only a roster that arrived may restrict anything. An unreachable platform
	// reports no models, and treating that as "the platform has none" would
	// empty a picker every time the endpoint blipped - the same distinction
	// FetchSiteModels draws between an empty list and a failed fetch.
	siteModels := r.siteModels(ctx)
	published := map[string]map[string]bool{}
	for _, model := range siteModels {
		if published[model.Provider] == nil {
			published[model.Provider] = map[string]bool{}
		}
		published[model.Provider][model.ID] = true
	}

	if cat, err := r.catalog(ctx); err == nil && cat != nil {
		for key, model := range cat.Models {
			provider := catalog.ProviderFromModelKey(key)
			if known, ok := published[provider]; ok && !known[key] {
				continue
			}
			add(key, model.DisplayName(), provider)
		}
	}

	for _, model := range siteModels {
		add(model.ID, model.DisplayName, model.Provider)
	}

	return slices.SortedFunc(maps.Values(seen), func(a, b ModelInfo) int {
		return cmp.Compare(a.ID, b.ID)
	})
}

// GetModelChain returns the full chain of models to try (primary + fallbacks).
func (rr *RouteResult) GetModelChain() []config.ModelConfig {
	chain := []config.ModelConfig{rr.Primary}
	chain = append(chain, rr.Fallbacks...)
	return chain
}

// RouteForStreaming determines which model to use for streaming requests.
// Prioritizes fast TTFT (time-to-first-token) over capability.
// If respect_requested_model is enabled and requestedModel is provided, it overrides scenario-based routing.
func (r *ModelRouter) RouteForStreaming(messages []MessageContent, tokenCount int, requestedModel string) (RouteResult, error) {
	cfg := r.atomic.Get()

	if result, ok, err := r.resolveRequestedModel(cfg, requestedModel, false); err != nil {
		return RouteResult{}, err
	} else if ok {
		return result, nil
	}

	// Otherwise, use scenario-based routing for streaming
	scenario := RouteForStreaming(messages, tokenCount, cfg)
	scenarioKey := string(scenario)

	// Get primary model for scenario. When cost-based routing is enabled and
	// a non-empty catalog is available, prefer the cheapest matching catalog
	// model while preserving the legacy fallback chain.
	primary, ok := cfg.Models[scenarioKey]
	if cat, catErr := r.catalog(context.Background()); cfg.CostBasedRoutingEnabled() && cat != nil && catErr == nil && len(cat.Models) > 0 {
		constraints := requestConstraints(messages, tokenCount)
		selector := NewSelector(cat, cfg)
		resolved, err := selector.SelectCheapest(scenarioKey, constraints)
		switch err {
		case nil:
			primary = resolvedModelToConfig(resolved)
			ok = true
		default:
			// Never swallow this: a silent failure here is indistinguishable
			// from cost routing working, which is how it went unnoticed that
			// the feature was a no-op.
			slog.Warn("cost-based routing found no model, using scenario model",
				"scenario", scenarioKey, "error", err)
		}
	}
	if !ok {
		if isVisionScenario(scenario) {
			return RouteResult{Scenario: scenario}, fmt.Errorf("vision scenario %s is not configured", scenario)
		}
		// Fall back to fast scenario if not configured
		primary, ok = cfg.Models["fast"]
		if !ok {
			// Fall back to default
			primary = cfg.Models["default"]
		}
	}
	if primary.ModelID == "" {
		return RouteResult{}, fmt.Errorf("no model configured for streaming; neither scenario %q, \"fast\", nor \"default\" exist in models map", scenario)
	}

	// Get fallbacks for scenario
	fallbacks := cfg.Fallbacks[scenarioKey]
	if len(fallbacks) == 0 {
		if isVisionScenario(scenario) {
			fallbacks = nil
		} else {
			// Fall back to fast fallbacks
			fallbacks = cfg.Fallbacks["fast"]
		}
	}

	return RouteResult{
		Primary:   primary,
		Fallbacks: fallbacks,
		Scenario:  scenario,
	}, nil
}

func isVisionScenario(s Scenario) bool {
	return s == ScenarioVision || s == ScenarioVisionComplex || s == ScenarioVisionLongContext
}

// ActiveSite is the platform routing is restricted to, or "" when the config
// does not restrict it. An unrestricted config behaves exactly as it did before
// this field existed: every configured target stays a candidate.
// Config returns the live configuration. Callers that need to reason about a
// platform's credentials - whether a cross-platform fallback is reachable at
// all - ask here rather than keeping a second copy of the config.
func (r *ModelRouter) Config() *config.Config {
	return r.atomic.Get()
}

func (r *ModelRouter) ActiveSite() string {
	return r.atomic.Get().ActiveSite
}

// PublishedByActiveSite returns the configured target and true when that site
// publishes this model id. The active site is the authority for its own model
// names, so a client that picked a model from the listing gets exactly that
// model - it does not get re-mapped by a configured alias.
//
// Capabilities come from the catalog when it knows the model. The site's own
// listing carries ids only, and the built-in registry is keyed by model family,
// so a model absent from both would route with no context window and no output
// ceiling at all. The catalog is the one source that has the platform's real
// numbers for models it does not otherwise recognize.
func (r *ModelRouter) PublishedByActiveSite(ctx context.Context, modelID string) (config.ModelConfig, bool) {
	cfg := r.atomic.Get()
	active := cfg.ActiveSite
	if active == "" || strings.TrimSpace(modelID) == "" {
		return config.ModelConfig{}, false
	}
	for _, m := range r.siteModels(ctx) {
		if m.Provider == active && m.ID == modelID {
			target := config.ModelConfig{Provider: active, ModelID: m.ID}
			// Keep explicit capabilities/limits only for this exact target.
			// An alias that remaps its name to another model or site still loses.
			for _, configured := range []config.ModelConfig{cfg.ModelOverrides[modelID], cfg.Models[modelID], cfg.Models["default"]} {
				if config.ModelKey(configured) == config.ModelKey(target) {
					return config.ResolveModelConfig(configured), true
				}
			}
			if resolved, err := r.catalogResolved(ctx, active, m.ID); err == nil {
				// Capabilities only. The catalog may file this model under a
				// vendor's namespace, and adopting its provider and id would
				// send a request the operator scoped to this platform somewhere
				// else - on ClinePass that is a different billing pool, not a
				// different spelling of the same target.
				target = applyCatalogCapabilities(target, resolved)
				// ModelRef records that these capabilities have an authority
				// behind them, which stops ResolveModelConfig from overriding
				// them with the built-in family table. The catalog's canonical
				// name is that authority, and it is single-prefixed - composing
				// one from this target would double a platform prefix the id
				// already carries.
				target.ModelRef = resolved.CanonicalName
			}
			return config.ResolveModelConfig(target), true
		}
	}
	return config.ModelConfig{}, false
}

// catalogResolved resolves a model id through the catalog: first as the active
// site names it, then by the catalog's own resolution rules.
//
// The second attempt matters because a platform's listing uses the platform's
// namespace while the catalog may file the same upstream model under the
// vendor's. ResolveShort is what handles that - it searches every provider for
// a short name and is the same path a client's model string takes.
//
// The third attempt strips the platform's own prefix. Neither lookup above
// removes anything, so an id the listing just published - Cline answers
// "cline-pass/deepseek-v4.1-flash-1017" while models.dev files the same model
// as "deepseek/deepseek-v4.1-flash-1017" - matches nothing at all. The failure
// is silent and expensive: the caller falls back to the hardcoded metadata
// table, which does not know the model, so it reports Vision=false and a zero
// context window, and every image request against a vision-capable model is
// then discarded as "vision_not_supported".
//
// Only a prefix that names the provider is stripped. A vendor namespace such
// as "deepseek/deepseek-v4-pro" on a platform that does not prefix its ids
// must be left alone, or it would resolve to another platform's entry.
func (r *ModelRouter) catalogResolved(ctx context.Context, provider, modelID string) (catalog.ResolvedModel, error) {
	cat, err := r.catalog(ctx)
	if err != nil || cat == nil {
		return catalog.ResolvedModel{}, fmt.Errorf("catalog unavailable")
	}
	if resolved, err := cat.Resolve(catalog.Selector{Provider: provider, Model: modelID}); err == nil {
		return resolved, nil
	}
	if resolved, err := cat.ResolveShort(modelID); err == nil {
		return resolved, nil
	}
	// Stripping is keyed on a registered platform, not on "whatever precedes the
	// slash": a vendor namespace is also slash-separated and removing it would
	// search for a model named after its vendor.
	//
	// This resolves the name only, never the target: a real catalog lists this
	// id under 38 providers, so asking it which provider to use is a question
	// with no right answer and ResolveShort rightly refuses it. The caller wants
	// the model's capabilities, so the name is all that is taken from it.
	if platform, short := splitPlatformPrefix(modelID); platform != "" && platform == site.Normalize(provider) {
		if model, ok := cat.ModelByName(short); ok {
			return catalog.ResolvedModel{
				Provider:        site.Normalize(provider),
				CanonicalName:   short,
				Vision:          model.SupportsVision(),
				ContextWindow:   model.ContextWindow(),
				MaxOutputTokens: model.MaxOutputTokens(),
				Tools:           model.SupportsTools(),
			}, nil
		}
	}
	return catalog.ResolvedModel{}, fmt.Errorf("catalog has no entry for %q", modelID)
}

// applyCatalogCapabilities takes the capability facts from a catalog resolution
// onto a target without letting the resolution change which platform is called
// or which model id is sent.
//
// PublishedByActiveSite must not adopt the resolution wholesale: the catalog may
// file the model under the vendor's namespace, and adopting it would send a
// request the operator scoped to one platform to a different one - for ClinePass
// that is not even the same billing pool. The listing decides where the request
// goes; the catalog only fills in what the model can do.
func applyCatalogCapabilities(target config.ModelConfig, resolved catalog.ResolvedModel) config.ModelConfig {
	vision := resolved.Vision
	target.Vision = &vision
	target.ContextWindow = int(resolved.ContextWindow)
	target.MaxOutputTokens = int(resolved.MaxOutputTokens)
	supportsTools := resolved.Tools
	target.SupportsTools = &supportsTools
	return target
}

// applyCatalogModel is applyCatalogCapabilities for a raw catalog entry. Both
// exist because the caller may hold either a resolved model (which carries a
// provider) or the model alone (when only the name was looked up).
func applyCatalogModel(target config.ModelConfig, model catalog.Model) config.ModelConfig {
	vision := model.SupportsVision()
	target.Vision = &vision
	target.ContextWindow = int(model.ContextWindow())
	target.MaxOutputTokens = int(model.MaxOutputTokens())
	supportsTools := model.SupportsTools()
	target.SupportsTools = &supportsTools
	return target
}

// RestrictToActiveSite keeps only the targets belonging to the active site. The
// active site is a scope rather than an override: it decides which configured
// targets may be used, and the caller reports an empty result as an error naming
// the site instead of quietly routing somewhere the operator did not choose.
func RestrictToActiveSite(active string, chain []config.ModelConfig) []config.ModelConfig {
	if active == "" {
		return chain
	}
	out := make([]config.ModelConfig, 0, len(chain))
	for _, target := range chain {
		if client.Provider(target) == active {
			out = append(out, target)
		}
	}
	return out
}

// SplitPrimaryAndFallbacks applies the active site to the roles differently,
// because they mean different things.
//
// The primary is where the request goes by default: the operator chose this
// platform, and silently sending their traffic elsewhere is not a fallback, it
// is a different decision. It stays scoped.
//
// A fallback is a plan for the case where the primary cannot answer. Restricting
// it to the same platform defeats the purpose: when the subscription itself is
// rate-limited or down, a second model on that subscription fails the same way.
// An operator who configured a fallback on another platform, and holds a
// credential for it, has already made that decision - so it is kept.
//
// Same-platform fallbacks stay first: quota already paid for should be spent
// before reaching into another account, and only then the cross-platform ones.
func SplitPrimaryAndFallbacks(active string, primary config.ModelConfig, fallbacks []config.ModelConfig) (config.ModelConfig, []config.ModelConfig) {
	_, ordered := ClassifyChain(active, primary, fallbacks)
	return primary, ordered
}

// ClassifyChain orders the fallbacks and reports which ones cross the active
// site boundary.
//
// The count is what makes the feature measurable before it is enabled: an
// operator can watch how often a cross-platform fallback would have been used
// without spending another account's quota to find out.
func ClassifyChain(active string, primary config.ModelConfig, fallbacks []config.ModelConfig) (crossPlatform int, ordered []config.ModelConfig) {
	if active == "" {
		return 0, fallbacks
	}
	samePlatform := make([]config.ModelConfig, 0, len(fallbacks))
	cross := make([]config.ModelConfig, 0, len(fallbacks))
	for _, target := range fallbacks {
		if config.ModelKey(target) == config.ModelKey(primary) {
			continue
		}
		if client.Provider(target) == active {
			samePlatform = append(samePlatform, target)
			continue
		}
		cross = append(cross, target)
	}
	return len(cross), append(samePlatform, cross...)
}

package provider

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/routatic/proxy/internal/catalog"
	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/internal/site"
	"github.com/routatic/proxy/internal/sse"
	"github.com/routatic/proxy/internal/transformer"
)

// Cline routes a model through one of two pipelines, each with its own pin
// field and its own backend: the planner pipeline reads
// providerOptions.gateway.only and runs on Vercel AI Gateway, the direct one
// reads the top-level provider.only and runs on OpenRouter.
//
// These are independent diagnostic experiments, not enabled request policies.
// A match on either path cannot establish an enforcement contract for Cline.
//
// They also behave differently, which is what makes separating them worth the
// extra request: measured on 2026-10-18 with 6 repeats per pin, the planner
// pipeline followed every servable pin it was given (18/18 across three
// channels on two models) while the direct pipeline never moved off the channel
// it had already chosen (0/12).
const (
	pipelinePlanner = "planner"
	pipelineDirect  = "direct"
)

var probePipelines = []string{pipelinePlanner, pipelineDirect}

const probeInterval = 6 * time.Hour
const maxProbeModels = 3

var errProbeResponseTooLarge = errors.New("probe response exceeds 64 KiB")

// ChannelProbeResult is a safe metadata projection, never a response or error body.
type ChannelProbeResult struct {
	Model         string    `json:"model"`
	Pipeline      string    `json:"pipeline"`
	Verdict       string    `json:"verdict"`
	Reason        string    `json:"reason,omitempty"`
	Baseline      string    `json:"baseline,omitempty"`
	Target        string    `json:"target,omitempty"`
	Actual        string    `json:"actual,omitempty"`
	CanonicalSlug string    `json:"canonical_slug,omitempty"`
	Status        int       `json:"status"`
	Endpoint      string    `json:"endpoint"`
	KeyHint       string    `json:"key_hint"`
	CheckedAt     time.Time `json:"checked_at"`
	ExpiresAt     time.Time `json:"expires_at"`
}

type ChannelDiagnosticsSnapshot struct {
	Enabled             bool                 `json:"enabled"`
	EnforcementVerified bool                 `json:"enforcement_verified"`
	Results             []ChannelProbeResult `json:"results"`
}

// ChannelDiagnostics reads existing results; it never starts a paid probe.
func (p *ClinePassProvider) ChannelDiagnostics() ChannelDiagnosticsSnapshot {
	cfg := p.atomic.Get()
	result := ChannelDiagnosticsSnapshot{Enabled: cfg.ClinePass.ChannelProbeEnabled, Results: []ChannelProbeResult{}}
	if !result.Enabled {
		return result
	}
	p.probeMu.Lock()
	defer p.probeMu.Unlock()
	if p.recentProbeScope == clineProbeScope(cfg) {
		result.Results = append(result.Results, p.probeResults...)
	}
	return result
}

// Caller holds probeMu; endpoint/key changes invalidate both kinds of evidence.
func (p *ClinePassProvider) setProbeScope(scope [32]byte) {
	if scope != p.recentProbeScope {
		p.recentProbeModels, p.probeResults = nil, nil
		p.recentProbeScope = scope
	}
}

func (p *ClinePassProvider) recordProbeResult(cfg *config.Config, result ChannelProbeResult) {
	p.probeMu.Lock()
	defer p.probeMu.Unlock()
	current := p.atomic.Get()
	scope := clineProbeScope(cfg)
	if !current.ClinePass.ChannelProbeEnabled || scope != clineProbeScope(current) {
		return // A completed old round cannot publish evidence for a changed configuration.
	}
	p.setProbeScope(scope)
	p.probeResults = slices.DeleteFunc(p.probeResults, func(r ChannelProbeResult) bool {
		return r.Model == result.Model && r.Pipeline == result.Pipeline
	})
	p.probeResults = append([]ChannelProbeResult{result}, p.probeResults...)
	if limit := maxProbeModels * len(probePipelines); len(p.probeResults) > limit {
		p.probeResults = p.probeResults[:limit]
	}
}

// probeOutputTokens must clear the reasoning budget. Small values make a
// reasoning model return "empty response content" with HTTP 500, which would be
// read as a routing verdict when it is not one.
//
// This is why the probe does not follow the other implementations, which send
// 16: that budget is spent by the reasoning pass before any content is
// produced, and the resulting 500 is indistinguishable from a routing refusal.
const probeOutputTokens = 64

// channelPinProbe asks the upstream whether it still honours a pin field, once
// per pipeline, and logs each answer.
//
// This exists because the setting can fail without failing: from 2026-09-20 the
// gateway started discarding the whole gateway object for some accounts, after
// which pinning, exclusion and sorting all became no-ops while every request
// still succeeded. The per-request observation in clineChannelBody can say
// "this answer did not come from the pinned channel", but it cannot say whether
// that is because the gateway ignored us or because the channel was merely
// unavailable - and those call for different responses.
//
// It only reports. Nothing here changes routing, and a failing probe must never
// be able to fail a request.
func (p *ClinePassProvider) channelPinProbeLoop(ctx context.Context) {
	// The first pass runs immediately: a verdict that arrives six hours after
	// start is useless during the incident it is meant to explain.
	p.runChannelPinProbe(ctx)

	ticker := time.NewTicker(probeInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.runChannelPinProbe(ctx)
		}
	}
}

// runChannelPinProbe probes each cline-pass model once per pipeline with the
// same key the proxy would use for it, and logs one line per model per
// pipeline.
func (p *ClinePassProvider) runChannelPinProbe(ctx context.Context) {
	cfg := p.atomic.Get()
	if cfg == nil {
		return
	}
	keys := cfg.ProviderAPIKeys(p.Name())
	if len(keys) == 0 {
		return
	}
	if !cfg.ClinePass.ChannelProbeEnabled {
		slog.Debug("cline-pass channel pin probe skipped: diagnostics are off")
		return
	}

	probeCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	models := p.clinePassProbeModels(probeCtx, cfg)
	if len(models) == 0 {
		return
	}

	var wg sync.WaitGroup
	for _, model := range models {
		for _, pipeline := range probePipelines {
			wg.Add(1)
			go func(model, pipeline string) {
				defer wg.Done()
				p.probeOneModel(probeCtx, cfg, keys, model, pipeline)
			}(model, pipeline)
		}
	}
	wg.Wait()
}

func clineProbeScope(cfg *config.Config) [32]byte {
	data, _ := json.Marshal(append([]string{cfg.ClinePass.BaseURL}, cfg.ProviderAPIKeys("cline-pass")...))
	return sha256.Sum256(data)
}

func (p *ClinePassProvider) rememberProbeModel(cfg *config.Config, model string) {
	if model == "" {
		return
	}
	p.probeMu.Lock()
	defer p.probeMu.Unlock()
	scope := clineProbeScope(cfg)
	if scope != clineProbeScope(p.atomic.Get()) {
		return // A delayed request must not rewind the current evidence scope.
	}
	p.setProbeScope(scope)
	p.recentProbeModels = slices.DeleteFunc(p.recentProbeModels, func(id string) bool { return id == model })
	p.recentProbeModels = append([]string{model}, p.recentProbeModels...)
	if len(p.recentProbeModels) > maxProbeModels {
		p.recentProbeModels = p.recentProbeModels[:maxProbeModels]
	}
}

// Prefer recently requested and explicitly configured models (including
// overrides); use the public roster only when neither identifies a target.
func (p *ClinePassProvider) clinePassProbeModels(ctx context.Context, cfg *config.Config) []string {
	p.probeMu.Lock()
	var ids []string
	if p.recentProbeScope == clineProbeScope(cfg) {
		ids = slices.Clone(p.recentProbeModels)
	}
	p.probeMu.Unlock()
	add := func(model config.ModelConfig) {
		if client.Provider(model) == p.Name() && model.ModelID != "" && !slices.Contains(ids, model.ModelID) && len(ids) < maxProbeModels {
			ids = append(ids, model.ModelID)
		}
	}
	for _, entries := range []map[string]config.ModelConfig{cfg.ModelOverrides, cfg.Models} {
		names := make([]string, 0, len(entries))
		for name := range entries {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			add(entries[name])
		}
	}
	names := make([]string, 0, len(cfg.Fallbacks))
	for name := range cfg.Fallbacks {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		for _, model := range cfg.Fallbacks[name] {
			add(model)
		}
	}
	if len(ids) > 0 {
		return ids
	}
	// Two steps, and both are required: ProviderModelEndpoint returns the
	// platform's API endpoint, and SiteModelsURL derives the list URL from it.
	// Passing the API endpoint straight to FetchSiteModels silently queries
	// /chat/completions instead of the roster.
	endpoint, err := catalog.SiteModelsURL(site.ClinePass, cfg.ProviderModelEndpoint(site.ClinePass))
	if err != nil {
		slog.Debug("cline-pass channel pin probe skipped: no model-list endpoint", "error", err)
		return nil
	}
	models, err := catalog.FetchSiteModels(ctx, p.httpClient, site.ClinePass, endpoint)
	if err != nil {
		// Not a verdict about pinning; the probe simply has nothing to ask about.
		slog.Debug("cline-pass channel pin probe skipped: model list unavailable", "error", err)
		return nil
	}
	ids = make([]string, 0, len(models))
	for _, model := range models {
		if id := strings.TrimSpace(model.ID); id != "" {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	// Bound paid samples; these results never stand for untested models.
	if len(ids) > maxProbeModels {
		ids = ids[:maxProbeModels]
	}
	return ids
}

// probeOneModel answers one question: does this pipeline, for this model,
// actually route to the channel we pin?
//
// It answers it by comparison, not by sentinel. The impossible channel cannot
// answer it (see probeSlug): an unservable pin is accepted and silently falls
// back, so it looks identical whether pinning works or not. What distinguishes
// the two is whether a *servable* pin moves the request, so the probe sends two
// requests - one unpinned to learn the channels this model can use, one pinned
// to a channel the baseline did NOT use - and compares where each was served.
//
//	baseline unpinned  -> served=minimax
//	pinned to nebius   -> served=nebius    => matched (one observation)
//	pinned to nebius   -> served=minimax   => ignored
//
// Missing alternatives leave the experiment inconclusive, not locked.
func (p *ClinePassProvider) probeOneModel(ctx context.Context, cfg *config.Config, keys []string, model, pipeline string) {
	sel, err := p.reserveKeyChecked(p.Name(), keys)
	if err != nil {
		slog.Debug("cline-pass channel pin probe skipped: no usable key")
		return
	}
	defer sel.Release() // Diagnostics neither reset nor penalize request health.
	endpoint := ""
	if u, err := url.Parse(cfg.ClinePass.BaseURL); err == nil {
		endpoint = u.Scheme + "://" + u.Host + u.EscapedPath()
	}
	var baseline, target, actual, canonical string
	logProbe := func(model, pipeline, verdict string, status int, reason, detail string) {
		checked := time.Now().UTC()
		p.recordProbeResult(cfg, ChannelProbeResult{Model: model, Pipeline: pipeline, Verdict: verdict, Reason: reason,
			Baseline: baseline, Target: target, Actual: actual, CanonicalSlug: canonical,
			Status: status, Endpoint: endpoint, KeyHint: sel.KeyHint(), CheckedAt: checked, ExpiresAt: checked.Add(probeInterval)})
		slog.Info("cline-pass channel pin probe", "model", model, "pipeline", pipeline,
			"verdict", verdict, "status", status, "reason", reason, "detail", detail,
			"baseline", baseline, "target", target, "actual", actual, "canonical_slug", canonical,
			"endpoint", endpoint, "key_hint", sel.KeyHint(), "checked_at", checked,
			"expires_at", checked.Add(probeInterval), "enforcement_verified", false,
			"session_policy", "fresh-per-request", "capabilities", "none")
	}
	status, text, err := p.sendProbe(ctx, cfg, sel.Key(), model, pipeline, nil)
	if err != nil {
		logProbe(model, pipeline, "unknown", status, probeReadReason(status, err), "probe request: "+err.Error())
		return
	}
	if reason := accountRefusal(status, text); reason != "" {
		logProbe(model, pipeline, "unknown", status, "account_refused", reason)
		return
	}
	if status < 200 || status >= 300 {
		// A refusal of the unpinned request is not about pinning at all.
		logProbe(model, pipeline, "unknown", status, "upstream_refused", "unpinned baseline refused: "+candidateList(text))
		return
	}
	if reason := probeStreamFailure(text); reason != "" {
		logProbe(model, pipeline, "unknown", status, "stream_failed", "unpinned baseline failed: "+reason)
		return
	}

	served, slug, candidates, err := routingSummary(text)
	if err != nil {
		logProbe(model, pipeline, "unknown", status, "invalid_metadata", "unpinned baseline has invalid routing metadata")
		return
	}
	baseline, actual, canonical = served, served, slug
	if served == "" {
		logProbe(model, pipeline, "unknown", status, "missing_channel", "unpinned baseline reported no serving channel")
		return
	}
	target = firstCandidateOtherThan(candidates, served)
	if target == "" {
		logProbe(model, pipeline, "unknown", status, "no_alternative", "baseline served by "+served+"; no alternative channel reported")
		return
	}

	// A failed variant must not display the baseline's model/channel as its result.
	actual, canonical = "", ""
	pinStatus, pinText, err := p.sendProbe(ctx, cfg, sel.Key(), model, pipeline, []string{target})
	if err != nil {
		logProbe(model, pipeline, "unknown", pinStatus, probeReadReason(pinStatus, err), "pinned probe request: "+err.Error())
		return
	}
	if clineRoutingRejected(pinStatus, pinText) {
		logProbe(model, pipeline, "unsupported", pinStatus, "", "upstream rejects client-supplied routing fields")
		return
	}
	if reason := accountRefusal(pinStatus, pinText); reason != "" {
		logProbe(model, pipeline, "unknown", pinStatus, "account_refused", reason)
		return
	}
	if pinStatus < 200 || pinStatus >= 300 {
		logProbe(model, pipeline, "unknown", pinStatus, "upstream_refused", "pinned request refused: "+candidateList(pinText))
		return
	}
	if reason := probeStreamFailure(pinText); reason != "" {
		logProbe(model, pipeline, "unknown", pinStatus, "stream_failed", "pin to "+target+" failed in-stream: "+reason)
		return
	}
	pinned, slug, _, err := routingSummary(pinText)
	if err != nil {
		logProbe(model, pipeline, "unknown", pinStatus, "invalid_metadata", "pinned request has invalid routing metadata")
		return
	}
	actual, canonical = pinned, slug
	switch {
	case canonicalClineChannel(pinned) == canonicalClineChannel(target):
		logProbe(model, pipeline, "matched", pinStatus, "", "pin to "+target+" honoured in this sample (baseline was "+served+"); enforcement unverified")
	case pinned == "":
		logProbe(model, pipeline, "unknown", pinStatus, "missing_channel", "pinned request reported no routing metadata")
	default:
		logProbe(model, pipeline, "ignored", pinStatus, "", "pin to "+target+" ignored; still served by "+pinned)
	}
}

func probeReadReason(status int, err error) string {
	var timeout net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &timeout) && timeout.Timeout()) {
		return "timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "cancelled"
	}
	if errors.Is(err, errProbeResponseTooLarge) {
		return "response_too_large"
	}
	if status != 0 {
		return "stream_failed"
	}
	return "transport_error"
}

// probeStreamFailure rejects failed or incomplete evidence without logging vendor messages.
func probeStreamFailure(text string) string {
	var decoder sse.Decoder
	var done bool
	var failure string
	err := decoder.Feed([]byte(text), func(event string, payload []byte) error {
		if string(bytes.TrimSpace(payload)) == "[DONE]" {
			done = true
			return nil
		}
		var chunk struct {
			Error *struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(payload, &chunk); err != nil {
			return err
		}
		if event == "error" || chunk.Error != nil {
			failure = "upstream error without a code"
			if chunk.Error != nil && chunk.Error.Code != "" {
				failure = chunk.Error.Code
			}
		}
		return nil
	})
	if err != nil {
		return "invalid SSE payload"
	}
	if failure != "" {
		return failure
	}
	if decoder.Finish() != nil || !done {
		return "incomplete SSE response"
	}
	completion := transformer.StreamTermination{Format: core.WireFormatOpenAIChat, RequireDone: true}
	if completion.Feed([]byte(text)) != nil || completion.Finish() != nil {
		return "invalid or incomplete Chat stream"
	}
	return ""
}

// firstCandidateOtherThan picks the channel to pin to: any reported candidate
// that the unpinned baseline did not use. Pinning to the channel the request
// would have used anyway proves nothing - the gateway looks obedient while
// having made no decision at all.
func firstCandidateOtherThan(candidates []string, baseline string) string {
	for _, candidate := range candidates {
		if canonicalClineChannel(candidate) != canonicalClineChannel(baseline) {
			return candidate
		}
	}
	return ""
}

// sendProbe issues one probe request. A nil only means the request carries no
// pin at all, which is the baseline the pinned request is compared against.
func (p *ClinePassProvider) sendProbe(ctx context.Context, cfg *config.Config, key, model, pipeline string, only []string) (int, string, error) {
	payload, err := json.Marshal(probeBody(model, pipeline, only))
	if err != nil {
		return 0, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.ClinePass.BaseURL, bytes.NewReader(payload))
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "text/event-stream")
	client.SetProviderHeaders(req, p.Name())
	req.Header.Set("X-Session-Id", uuid.NewString())

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	const maxProbeBody = 1 << 16
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxProbeBody+1))
	if err != nil {
		return resp.StatusCode, "", err
	}
	if len(body) > maxProbeBody {
		return resp.StatusCode, "", errProbeResponseTooLarge
	}
	return resp.StatusCode, string(body), nil
}

// probeBody builds one pipeline's probe request.
//
// Each pipeline carries only its own field. Sending both at once cannot tell which one the
// rejection came from, and the two are separate questions with separate
// answers: an account can have one pipeline honouring the pin and the other
// discarding it.
func probeBody(model, pipeline string, only []string) map[string]any {
	body := map[string]any{
		"model":      model,
		"messages":   []map[string]string{{"role": "user", "content": "reply with the single word ok"}},
		"max_tokens": probeOutputTokens,
		// The proxy's own request path always streams; probing the other shape
		// would answer a question about a path nothing uses.
		"stream": true,
	}
	if only == nil {
		return body
	}
	if pipeline == pipelineDirect {
		body["provider"] = map[string]any{"only": only}
		return body
	}
	body["providerOptions"] = map[string]any{"gateway": map[string]any{"only": only}}
	return body
}

// routingSummary reads the serving channel, canonical model and candidates out of an SSE
// body. The gateway sends the block in the last chunk before [DONE], so this
// scans the whole body.
//
// It reads the same block the per-request observer does, through the same
// struct, so the two cannot disagree about which field names the channel.
func routingSummary(text string) (served, canonical string, candidates []string, err error) {
	seen := map[string]bool{}
	var decoder sse.Decoder
	err = decoder.Feed([]byte(text), func(_ string, payload []byte) error {
		if string(bytes.TrimSpace(payload)) == "[DONE]" {
			return nil
		}
		var chunk struct {
			clineRoutingMetadata
			Choices []struct {
				Delta   clineRoutingMetadata `json:"delta"`
				Message clineRoutingMetadata `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(payload, &chunk); err != nil {
			return err
		}
		remember := func(m clineRoutingMetadata) error {
			routing := m.ProviderMetadata.Gateway.Routing
			v, err := m.channel()
			if err != nil {
				return err
			}
			if v != "" {
				served = v
			}
			if routing.CanonicalSlug != "" {
				canonical = routing.CanonicalSlug
			}
			for _, candidate := range routing.FallbacksAvailable {
				candidate = strings.TrimSpace(candidate)
				identity := canonicalClineChannel(candidate)
				if candidate != "" && !seen[identity] {
					seen[identity] = true
					candidates = append(candidates, candidate)
				}
			}
			return nil
		}
		if err := remember(chunk.clineRoutingMetadata); err != nil {
			return err
		}
		for _, choice := range chunk.Choices {
			if err := remember(choice.Message); err != nil {
				return err
			}
			if err := remember(choice.Delta); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return "", "", nil, err
	}
	if err := decoder.Finish(); err != nil {
		return "", "", nil, err
	}
	return served, canonical, candidates, nil
}

// accountRefusal reports why a response is an account-level refusal rather than
// a routing verdict, or "" when it is not one.
//
// These responses are about the credential or the subscription, not about the
// channel the request asked for, so no verdict about pinning can be read from
// them. A 429 is included wholesale rather than by code because the gateway
// documents that its window code drifts (the same minute reports 5-HOUR and
// WEEKLY for the same rejection), so the code is not something to key on.
//
// Only statuses and codes that have actually been observed are listed. A
// guessed code would be a rule that never fires, which is indistinguishable
// from no rule until the day it matters.
func accountRefusal(status int, text string) string {
	switch status {
	case http.StatusUnauthorized, http.StatusPaymentRequired, http.StatusForbidden:
		return "account refused the credential (HTTP " + strconv.Itoa(status) + ")"
	case http.StatusTooManyRequests:
		if code := errorCode(text); code != "" {
			return "account quota exhausted or rate limited: " + code
		}
		return "account quota exhausted or rate limited (HTTP 429)"
	}
	return ""
}

// errorCode extracts the gateway's own error code from a JSON error body, so a
// quota refusal is reported as INFERENCE_CAP_ERROR rather than as a bare status.
func errorCode(text string) string {
	start := strings.IndexByte(text, '{')
	if start < 0 {
		return ""
	}
	var payload struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal([]byte(text[start:]), &payload) != nil {
		return ""
	}
	return payload.Error.Code
}

// candidateList extracts the channel list a rejection may carry. This is the
// one moment the gateway names the channels it would have accepted, so it is
// the whole reason a rejected probe is worth more than an accepted one.
//
// The two pipelines publish it in different shapes, which is the other reason
// they have to be probed separately: the planner pipeline (Vercel) puts it in
// prose ("Available providers are: alibaba, deepseek, ..."), the direct one
// (OpenRouter) in an error object's metadata. Only the tail of either is kept -
// the surrounding error body is noise.
func candidateList(text string) string {
	const marker = "Available providers are:"
	if i := strings.Index(text, marker); i >= 0 {
		list := text[i+len(marker):]
		if j := strings.IndexAny(list, "\"\n\\"); j >= 0 {
			list = list[:j]
		}
		if list = strings.TrimSpace(list); list != "" {
			return "candidates:" + list
		}
	}
	if list := jsonAvailableProviders(text); list != "" {
		return "candidates:" + list
	}
	if strings.Contains(text, "No allowed providers") {
		return "no allowed providers"
	}
	return "rejected"
}

// jsonAvailableProviders reads the direct pipeline's channel list out of an
// error body, which carries it as error.metadata.available_providers rather
// than in prose.
//
// The body is not always pure JSON - the gateway prefixes it with an HTTP
// status line - so parsing starts at the first brace. A body that is not JSON
// at all yields "", which leaves the prose path's answer standing.
func jsonAvailableProviders(text string) string {
	start := strings.IndexByte(text, '{')
	if start < 0 {
		return ""
	}
	var payload struct {
		Error struct {
			Metadata struct {
				AvailableProviders []string `json:"available_providers"`
			} `json:"metadata"`
		} `json:"error"`
	}
	if json.Unmarshal([]byte(text[start:]), &payload) != nil {
		return ""
	}
	return strings.Join(payload.Error.Metadata.AvailableProviders, ", ")
}

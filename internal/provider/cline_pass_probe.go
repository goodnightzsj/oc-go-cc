package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/routatic/proxy/internal/catalog"
	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/site"
)

// probeSlug is a well-formed provider slug that no gateway can serve. It is the
// whole point of the probe: a channel name the router must reject if it is
// still reading the request, and one it cannot reject if the field is being
// discarded. Underscored names are not usable here - the gateway drops those
// silently for unrelated reasons.
const probeSlug = "zzz-not-a-provider"

const probeInterval = 6 * time.Hour

// probeOutputTokens must clear the reasoning budget. Small values make a
// reasoning model return "empty response content" with HTTP 500, which would be
// read as a routing verdict when it is not one.
const probeOutputTokens = 64

// channelPinProbe asks the upstream whether it still honours
// providerOptions.gateway.only, and logs the answer.
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

// runChannelPinProbe probes each cline-pass model once with the same key the
// proxy would use for it, and logs one line per model.
func (p *ClinePassProvider) runChannelPinProbe(ctx context.Context) {
	cfg := p.atomic.Get()
	if cfg == nil {
		return
	}
	keys := cfg.ClinePass.EffectiveAPIKeys()
	if len(keys) == 0 {
		return
	}
	// The probe is a request against a paid quota. A pinned configuration is
	// the only reason to spend it.
	if !cfg.ClinePass.ChannelPinEnabled {
		slog.Debug("cline-pass channel pin probe skipped: pinning is off")
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
		wg.Add(1)
		go func(model string) {
			defer wg.Done()
			p.probeOneModel(probeCtx, cfg, keys, model)
		}(model)
	}
	wg.Wait()
}

// clinePassProbeModels lists the models to probe.
//
// It asks the platform for its own roster rather than reading cfg.Models or the
// overrides. Both were wrong on the deployment this was written for: cfg.Models
// held only CommandCode entries, and the ClinePass model ids arrived through
// model_overrides, so a config scan found nothing to probe and the loop
// silently did no work. The roster endpoint is the same source the model picker
// uses, so it cannot disagree with what the platform actually serves.
func (p *ClinePassProvider) clinePassProbeModels(ctx context.Context, cfg *config.Config) []string {
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
	ids := make([]string, 0, len(models))
	for _, model := range models {
		if id := strings.TrimSpace(model.ID); id != "" {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	// A platform-wide probe costs one request per model. If the roster is large,
	// probing all of it every cycle spends quota to answer a question one model
	// answers just as well: the gateway applies the same policy to the platform.
	const maxProbeModels = 3
	if len(ids) > maxProbeModels {
		ids = ids[:maxProbeModels]
	}
	return ids
}

func (p *ClinePassProvider) probeOneModel(ctx context.Context, cfg *config.Config, keys []string, model string) {
	payload, err := json.Marshal(map[string]any{
		"model":      model,
		"messages":   []map[string]string{{"role": "user", "content": "reply with the single word ok"}},
		"max_tokens": probeOutputTokens,
		// The proxy's own request path always streams; probing the other shape
		// would answer a question about a path nothing uses.
		"stream": true,
		"providerOptions": map[string]any{
			"gateway": map[string]any{"only": []string{probeSlug}},
		},
	})
	if err != nil {
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.ClinePass.BaseURL, bytes.NewReader(payload))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+keys[0])
	req.Header.Set("Accept", "text/event-stream")
	client.SetProviderHeaders(req, p.Name())

	resp, err := p.httpClient.Do(req)
	if err != nil {
		p.logProbe(model, "unknown", 0, "transport: "+err.Error())
		return
	}
	defer func() { _ = resp.Body.Close() }()

	// A rejected probe may carry the candidate list in its error body, which is
	// the one moment the gateway tells us what it would have accepted.
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	text := string(body)

	// "empty response content" arrives as HTTP 500, so this must be settled
	// before any status-code branch reads a 5xx as a refusal. Small token
	// budgets make a reasoning model produce it, and mistaking it for a
	// rejection would report a dropped field as an enforced one.
	if strings.Contains(text, "empty response content") {
		p.logProbe(model, "unknown", resp.StatusCode, "empty response content: probe token budget too small")
		return
	}

	switch {
	case resp.StatusCode >= 400:
		// Refused the impossible channel, so the field is still read.
		p.logProbe(model, "enforced", resp.StatusCode, candidateList(text))
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		// The impossible channel was accepted, so providerOptions never reached
		// the router. Confirmed by a real completion rather than an empty body,
		// which would instead mean the probe's own token budget was too small.
		if final := firstFinalProvider(text); final != "" {
			p.logProbe(model, "ignored", resp.StatusCode, "gateway routed to "+final)
			return
		}
		p.logProbe(model, "ignored", resp.StatusCode, "impossible channel accepted without routing metadata")
	default:
		p.logProbe(model, "unknown", resp.StatusCode, "unexpected status")
	}
}

// logProbe emits one line per model per run.
//
// It is deliberately Info, not Warn: a verdict of "ignored" is the steady state
// on an affected account, and a warning that repeats every six hours forever is
// a warning nobody reads. The signal a reader needs is in the field.
func (p *ClinePassProvider) logProbe(model, verdict string, status int, detail string) {
	slog.Info("cline-pass channel pin probe",
		"model", model,
		"probe_channel", probeSlug,
		"verdict", verdict,
		"status", status,
		"detail", detail)
}

// candidateList extracts the "Available providers are: ..." list a rejection
// may carry. Only the tail is kept: the whole error body is noise.
func candidateList(text string) string {
	const marker = "Available providers are:"
	i := strings.Index(text, marker)
	if i < 0 {
		if strings.Contains(text, "No allowed providers") {
			return "no allowed providers"
		}
		return "rejected"
	}
	list := text[i+len(marker):]
	if j := strings.IndexAny(list, "\"\n\\"); j >= 0 {
		list = list[:j]
	}
	return "candidates:" + strings.TrimSpace(list)
}

// firstFinalProvider finds the routing metadata in an SSE body. The gateway
// sends it in the last chunk before [DONE], so this scans the whole body.
func firstFinalProvider(text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var chunk struct {
			clineRoutingMetadata
			Choices []struct {
				Delta   clineRoutingMetadata `json:"delta"`
				Message clineRoutingMetadata `json:"message"`
			} `json:"choices"`
		}
		if json.Unmarshal([]byte(strings.TrimSpace(line[len("data:"):])), &chunk) != nil {
			continue
		}
		if v := chunk.ProviderMetadata.Gateway.Routing.FinalProvider; v != "" {
			return v
		}
		for _, choice := range chunk.Choices {
			if v := choice.Message.ProviderMetadata.Gateway.Routing.FinalProvider; v != "" {
				return v
			}
			if v := choice.Delta.ProviderMetadata.Gateway.Routing.FinalProvider; v != "" {
				return v
			}
		}
	}
	return ""
}

package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/routatic/proxy/internal/catalog"
	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/site"
)

// Cline routes a model through one of two pipelines, each with its own pin
// field and its own backend: the planner pipeline reads
// providerOptions.gateway.only and runs on Vercel AI Gateway, the direct one
// reads the top-level provider.only and runs on OpenRouter.
//
// The gateway never says which pipeline served a request, and the two do not
// share a channel list, so a single probe cannot answer for both. The proxy's
// own send path already writes both fields for exactly this reason; the probe
// has to ask each separately, because the question is which of the two is still
// being read.
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
//	pinned to nebius   -> served=nebius    => enforced
//	pinned to nebius   -> served=minimax   => ignored
//
// A model with no candidate list has one channel, so there is nothing to pin to
// and no experiment to run; that is reported as locked rather than as a failure.
func (p *ClinePassProvider) probeOneModel(ctx context.Context, cfg *config.Config, keys []string, model, pipeline string) {
	status, text, err := p.sendProbe(ctx, cfg, keys[0], model, pipeline, nil)
	if err != nil {
		p.logProbe(model, pipeline, "unknown", 0, "transport: "+err.Error())
		return
	}
	if reason := accountRefusal(status, text); reason != "" {
		p.logProbe(model, pipeline, "unknown", status, reason)
		return
	}
	if status < 200 || status >= 300 {
		// A refusal of the unpinned request is not about pinning at all.
		p.logProbe(model, pipeline, "unknown", status, "unpinned baseline refused: "+candidateList(text))
		return
	}

	served, candidates := routingSummary(text)
	target := firstCandidateOtherThan(candidates, served)
	if target == "" {
		// One channel, or none reported. There is no second channel to ask for,
		// so this model cannot be pinned on this pipeline whatever the account's
		// policy is - and saying "ignored" here would blame the gateway for a
		// model that simply has nowhere else to go.
		if served == "" {
			p.logProbe(model, pipeline, "unknown", status, "unpinned baseline reported no routing metadata")
			return
		}
		p.logProbe(model, pipeline, "locked", status, "single channel "+served+": nothing to pin to")
		return
	}

	pinStatus, pinText, err := p.sendProbe(ctx, cfg, keys[0], model, pipeline, []string{target})
	if err != nil {
		p.logProbe(model, pipeline, "unknown", 0, "transport on pinned request: "+err.Error())
		return
	}
	if reason := accountRefusal(pinStatus, pinText); reason != "" {
		p.logProbe(model, pipeline, "unknown", pinStatus, reason)
		return
	}
	// A pin to a channel this model cannot use is a hard failure, and it arrives
	// as HTTP 200 with an SSE error frame rather than a status code:
	//
	//	data: {"error":{"code":"stream_initialization_failed",
	//	  "message":"... failed to invoke model 'qwen/qwen3.7-max' ..."}}
	//
	// Read as "no routing metadata" that would report unknown, which is wrong:
	// the gateway did read the pin and did try to honour it, so this is the
	// strongest available evidence that pinning is enforced. It is also what an
	// operator sees when channel_pin names a channel the model cannot serve.
	if code := streamErrorCode(pinText); code != "" {
		p.logProbe(model, pipeline, "enforced", pinStatus, "pin to "+target+" rejected in-stream: "+code)
		return
	}
	pinned, _ := routingSummary(pinText)
	switch {
	case pinned == target:
		p.logProbe(model, pipeline, "enforced", pinStatus, "pin to "+target+" honoured (baseline was "+served+")")
	case pinned == "":
		p.logProbe(model, pipeline, "unknown", pinStatus, "pinned request reported no routing metadata")
	default:
		p.logProbe(model, pipeline, "ignored", pinStatus, "pin to "+target+" ignored; still served by "+pinned)
	}
}

// streamErrorCode reads an error the gateway reported inside the SSE stream.
//
// The Cline API answers some failures with HTTP 200 and an error frame, so a
// status-only reader treats them as successes. Only a code is returned, never
// the message: the message embeds the upstream vendor's own wording and is not
// something to copy into a log line.
func streamErrorCode(text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(line[len("data:"):])
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var chunk struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if json.Unmarshal([]byte(payload), &chunk) != nil {
			continue
		}
		if chunk.Error.Code != "" {
			return chunk.Error.Code
		}
	}
	return ""
}

// firstCandidateOtherThan picks the channel to pin to: any reported candidate
// that the unpinned baseline did not use. Pinning to the channel the request
// would have used anyway proves nothing - the gateway looks obedient while
// having made no decision at all.
func firstCandidateOtherThan(candidates []string, baseline string) string {
	for _, candidate := range candidates {
		if candidate != baseline {
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

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	return resp.StatusCode, string(body), nil
}

// probeBody builds one pipeline's probe request.
//
// Each pipeline carries only its own field. Sending both at once - which the
// proxy's real send path does, deliberately - cannot tell which one the
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

// routingSummary reads the serving channel and the candidate list out of an SSE
// body. The gateway sends the block in the last chunk before [DONE], so this
// scans the whole body.
//
// It reads the same block the per-request observer does, through the same
// struct, so the two cannot disagree about which field names the channel.
func routingSummary(text string) (served string, candidates []string) {
	seen := map[string]bool{}
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
		remember := func(m clineRoutingMetadata) {
			routing := m.ProviderMetadata.Gateway.Routing
			if v := m.channel(); v != "" {
				served = v
			}
			for _, candidate := range routing.FallbacksAvailable {
				candidate = strings.TrimSpace(candidate)
				if candidate != "" && !seen[candidate] {
					seen[candidate] = true
					candidates = append(candidates, candidate)
				}
			}
		}
		remember(chunk.clineRoutingMetadata)
		for _, choice := range chunk.Choices {
			remember(choice.Message)
			remember(choice.Delta)
		}
	}
	return served, candidates
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

// logProbe emits one line per model per pipeline per run.
//
// It is deliberately Info, not Warn: a verdict of "ignored" is the steady state
// on an affected account, and a warning that repeats every six hours forever is
// a warning nobody reads. The signal a reader needs is in the field.
func (p *ClinePassProvider) logProbe(model, pipeline, verdict string, status int, detail string) {
	slog.Info("cline-pass channel pin probe",
		"model", model,
		"pipeline", pipeline,
		"verdict", verdict,
		"status", status,
		"detail", detail)
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

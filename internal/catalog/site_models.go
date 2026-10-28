package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/internal/site"
)

// SiteModel is one model a platform's own API reports.
type SiteModel struct {
	ID          string
	DisplayName string
	Provider    string
	Reasoning   *config.PublishedReasoning
}

// siteModelPathSuffix is the tail a platform's API endpoint must end with
// before its model list can be derived from it. Only platforms whose API
// documents a model-list endpoint appear here; OpenCode Go is absent because
// its models come from the models.dev catalog.
//
// The suffix is checked rather than assumed. Deriving a URL from whatever
// happens to be configured would aim a request at a host that never published
// that endpoint, which is exactly the mistake the account-API derivation in
// internal/quota guards against the same way.
var siteModelPathSuffix = map[string]string{
	site.CommandCode: "chat/completions",
}

// clinePassModelsPath is the Cline API's subscription model list, relative to
// the configured endpoint's origin.
//
// It is not derivable from the endpoint by the suffix rule above, and deriving
// it by swapping the last segment would be worse than not deriving it: that
// asks for /api/v1/models, which answers 200 with 446 models and no cline-pass
// entry at all, because it lists the usage-billing pool. An empty picker is a
// visible failure; a picker full of models this credential cannot call is not.
const clinePassModelsPath = "/api/v1/ai/cline/recommended-models"

// clinePassBucket names the bucket inside that response holding this
// platform's roster. The payload carries four (recommended, free, clinePass,
// clineCloud) and a model's bucket decides how it is billed - not its id
// prefix, which the free bucket mixes (cline-free/..., z-ai/..., poolside/...).
const clinePassBucket = "clinePass"

// SiteModelsURL derives a platform's model-list endpoint from its configured
// API endpoint, or reports that this platform has no derivable one.
func SiteModelsURL(provider, apiEndpoint string) (string, error) {
	name := site.Normalize(provider)
	if name == site.ClinePass {
		return clinePassModelsURL(apiEndpoint)
	}
	if name == site.WorkBuddy {
		return workBuddyModelsURL(apiEndpoint)
	}
	suffix, ok := siteModelPathSuffix[name]
	if !ok {
		return "", fmt.Errorf("no model-list endpoint is known for %s", provider)
	}
	parsed, err := url.Parse(apiEndpoint)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("%s api endpoint must be an absolute URL without credentials, query or fragment", provider)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("%s api endpoint must use http or https", provider)
	}
	path := strings.TrimSuffix(parsed.Path, "/")
	if !strings.HasSuffix(path, "/"+suffix) {
		return "", fmt.Errorf("%s api endpoint must end with %q so its model list can be derived", provider, suffix)
	}
	parsed.Path = strings.TrimSuffix(path, "/"+suffix) + "/models"
	parsed.RawPath = ""
	return parsed.String(), nil
}

// clinePassModelsURL keeps the configured origin and gateway mount, replacing
// only the path below it. A base URL that is not the Cline API one is rejected
// rather than rewritten, so a key meant for a private mirror is never sent to
// the public host.
func clinePassModelsURL(apiEndpoint string) (string, error) {
	parsed, err := url.Parse(apiEndpoint)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("cline-pass api endpoint must be an absolute URL without credentials, query or fragment")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("cline-pass api endpoint must use http or https")
	}
	path := strings.TrimSuffix(parsed.Path, "/")
	if path != "/api/v1/chat/completions" {
		return "", fmt.Errorf("cline-pass api endpoint must end with %q so its model list can be located", "/api/v1/chat/completions")
	}
	parsed.Path = clinePassModelsPath
	parsed.RawPath = ""
	return parsed.String(), nil
}

// workBuddyModelsURL derives the model list from a workbuddy2api-panel
// gateway, accepting every spelling an operator actually types.
//
// The generic suffix rule would reject all but one of them: it insists the
// endpoint end in "chat/completions", while the gateway's own panel shows a
// bare host and the OpenAI convention stops at "/v1". Forcing the operator to
// remember which one this particular platform wants is a papercut with no
// upside, and the derivation is unambiguous either way - the list always lives
// at <origin>/v1/models.
//
// Unlike ClinePass there is no second host to guard against: this endpoint is
// a gateway the operator runs, so rewriting the path cannot send their
// credential somewhere they did not intend.
func workBuddyModelsURL(apiEndpoint string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(apiEndpoint))
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("workbuddy api endpoint must be an absolute URL without credentials, query or fragment")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("workbuddy api endpoint must use http or https")
	}
	// Strip to the origin, then append the one path the gateway publishes.
	// Every accepted spelling - bare host, /v1, /v1/chat/completions - reduces
	// to the same origin here, which is why this does not need the suffix check
	// the generic rule uses.
	parsed.Path = "/v1/models"
	parsed.RawPath = ""
	return parsed.String(), nil
}

// Vendor reasoning metadata is decoded only for WorkBuddy, whose schema is
// known; other platforms retain their existing ID/name-only contract.
type siteModelsResponse struct {
	Data []struct {
		ID                 string          `json:"id"`
		Name               string          `json:"name"`
		ReasoningEfforts   json.RawMessage `json:"reasoning_supported_efforts"`
		ReasoningDefault   json.RawMessage `json:"reasoning_default_effort"`
		CanDisableThinking json.RawMessage `json:"can_disable_thinking"`
	} `json:"data"`
}

// maxSiteModelsBytes bounds the response so a mirror answering with something
// other than a model list cannot be read into memory unbounded.
const maxSiteModelsBytes = 4 << 20

// FetchSiteModels reads a platform's model list. The endpoint is public: no
// credential is attached, so this call cannot leak one to a host that did not
// ask for it.
func FetchSiteModels(ctx context.Context, client *http.Client, provider, endpoint string) ([]SiteModel, error) {
	return FetchSiteModelsWithKey(ctx, client, provider, endpoint, "")
}

// FetchSiteModelsWithKey is FetchSiteModels for a platform whose roster is
// authenticated.
//
// CommandCode and ClinePass publish their lists publicly, so the plain form has
// always worked and stays the default. A workbuddy2api-panel gateway answers
// 401 without a bearer key - its roster belongs to the account, not to the
// platform - so the caller has to pass one.
func FetchSiteModelsWithKey(ctx context.Context, client *http.Client, provider, endpoint, apiKey string) ([]SiteModel, error) {
	raw, err := fetchSiteModelsBody(ctx, client, provider, endpoint, apiKey)
	if err != nil {
		return nil, err
	}
	return parseSiteModels(provider, raw)
}

// parseSiteModels reads the platform's own response shape. Cline's list carries
// its rosters in named buckets and needs the bucket selected first; every other
// platform answers with a flat {data:[...]}.
func parseSiteModels(provider string, raw []byte) ([]SiteModel, error) {
	if site.Normalize(provider) == site.ClinePass {
		return parseClinePassModels(provider, raw)
	}
	var decoded siteModelsResponse
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, fmt.Errorf("decode %s model list: %w", provider, err)
	}
	out := make([]SiteModel, 0, len(decoded.Data))
	for _, m := range decoded.Data {
		if model, ok := newSiteModel(provider, m.ID, m.Name); ok {
			if model.Provider == site.WorkBuddy {
				var err error
				model.Reasoning, err = workBuddyReasoning(m.ReasoningEfforts, m.ReasoningDefault, m.CanDisableThinking)
				if err != nil {
					return nil, fmt.Errorf("decode workbuddy model list reasoning: %w", err)
				}
			}
			out = append(out, model)
		}
	}
	return requireSiteModels(provider, out)
}

func workBuddyReasoning(rawLevels, rawDefault, rawDisable json.RawMessage) (*config.PublishedReasoning, error) {
	var levels []string
	var defaultEffort string
	var canDisable *bool
	for _, field := range []struct {
		name string
		raw  json.RawMessage
		dst  any
	}{
		{"reasoning_supported_efforts", rawLevels, &levels},
		{"reasoning_default_effort", rawDefault, &defaultEffort},
		{"can_disable_thinking", rawDisable, &canDisable},
	} {
		if len(field.raw) > 0 {
			if err := json.Unmarshal(field.raw, field.dst); err != nil {
				return nil, fmt.Errorf("invalid %s", field.name)
			}
		}
	}
	if len(levels) == 0 && defaultEffort == "" && canDisable == nil {
		return nil, nil
	}
	result := &config.PublishedReasoning{CanDisableThinking: canDisable}
	for _, raw := range levels {
		if strings.EqualFold(strings.TrimSpace(raw), "off") {
			result.CanDisableThinking = config.BoolPtr(true)
			if !slices.Contains(result.Levels, "off") {
				result.Levels = append(result.Levels, "off")
			}
			continue
		}
		level, ok := core.NormalizeEffort(strings.ToLower(strings.TrimSpace(raw)))
		if !ok || level == "" {
			return nil, fmt.Errorf("reasoning_supported_efforts contains an unrecognized level")
		}
		if !slices.Contains(result.Levels, level) {
			result.Levels = append(result.Levels, level)
		}
	}
	if defaultEffort != "" {
		level, ok := core.NormalizeEffort(strings.ToLower(strings.TrimSpace(defaultEffort)))
		if !ok || !slices.Contains(result.Levels, level) {
			return nil, fmt.Errorf("reasoning_default_effort is not in reasoning_supported_efforts")
		}
		result.DefaultEffort = level
	}
	return result, nil
}

// parseClinePassModels reads the clinePass bucket out of Cline's recommended-
// models payload.
//
// The buckets are the billing pools, and the bucket is what decides a model's
// billing - not its id prefix, which the free bucket mixes. Reading only this
// bucket is what keeps the listing to models the subscription actually covers.
func parseClinePassModels(provider string, raw []byte) ([]SiteModel, error) {
	var payload struct {
		ClinePass []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"clinePass"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("decode %s model list: %w", provider, err)
	}
	out := make([]SiteModel, 0, len(payload.ClinePass))
	for _, m := range payload.ClinePass {
		if model, ok := newSiteModel(provider, m.ID, m.Name); ok {
			out = append(out, model)
		}
	}
	return requireSiteModels(provider, out)
}

// newSiteModel reports whether an entry is usable. An entry with a blank id is
// skipped, but one with a blank name is kept: the listing falls back to the id
// rather than dropping a model that is really there.
func newSiteModel(provider, id, name string) (SiteModel, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return SiteModel{}, false
	}
	return SiteModel{ID: id, DisplayName: strings.TrimSpace(name), Provider: site.Normalize(provider)}, true
}

func requireSiteModels(provider string, out []SiteModel) ([]SiteModel, error) {
	if len(out) == 0 {
		// An empty list and a failed fetch are different answers: the first says
		// the platform offers nothing, the second says we do not know. Reporting
		// the second as the first would silently empty a client's model picker.
		return nil, fmt.Errorf("%s model list was empty", provider)
	}
	return out, nil
}

func fetchSiteModelsBody(ctx context.Context, client *http.Client, provider, endpoint, apiKey string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build %s model list request: %w", provider, err)
	}
	req.Header.Set("Accept", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s model list: %w", provider, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("%s model list returned HTTP %d: %s", provider, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxSiteModelsBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read %s model list: %w", provider, err)
	}
	if len(raw) > maxSiteModelsBytes {
		return nil, fmt.Errorf("%s model list exceeded %d bytes", provider, maxSiteModelsBytes)
	}
	return raw, nil
}

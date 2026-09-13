package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/routatic/proxy/internal/site"
)

// SiteModel is one model a platform's own API reports.
type SiteModel struct {
	ID          string
	DisplayName string
	Provider    string
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

// SiteModelsURL derives a platform's model-list endpoint from its configured
// API endpoint, or reports that this platform has no derivable one.
func SiteModelsURL(provider, apiEndpoint string) (string, error) {
	suffix, ok := siteModelPathSuffix[site.Normalize(provider)]
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

// siteModelsResponse is the shape the platform APIs publish. Only the fields
// the listing shows are read; an unknown extra field is not an error, because
// these endpoints carry vendor metadata this proxy has no use for.
type siteModelsResponse struct {
	Data []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"data"`
}

// maxSiteModelsBytes bounds the response so a mirror answering with something
// other than a model list cannot be read into memory unbounded.
const maxSiteModelsBytes = 4 << 20

// FetchSiteModels reads a platform's model list. The endpoint is public: no
// credential is attached, so this call cannot leak one to a host that did not
// ask for it.
func FetchSiteModels(ctx context.Context, client *http.Client, provider, endpoint string) ([]SiteModel, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build %s model list request: %w", provider, err)
	}
	req.Header.Set("Accept", "application/json")
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
	var decoded siteModelsResponse
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, fmt.Errorf("decode %s model list: %w", provider, err)
	}
	out := make([]SiteModel, 0, len(decoded.Data))
	for _, m := range decoded.Data {
		id := strings.TrimSpace(m.ID)
		if id == "" {
			continue
		}
		out = append(out, SiteModel{ID: id, DisplayName: strings.TrimSpace(m.Name), Provider: site.Normalize(provider)})
	}
	if len(out) == 0 {
		// An empty list and a failed fetch are different answers: the first says
		// the platform offers nothing, the second says we do not know. Reporting
		// the second as the first would silently empty a client's model picker.
		return nil, fmt.Errorf("%s model list was empty", provider)
	}
	return out, nil
}

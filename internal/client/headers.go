package client

import (
	"net/http"

	"github.com/routatic/proxy/internal/core"
)

// SetProviderHeaders keeps provider attribution and session identity scoped to
// their owning upstream. Inbound authentication headers are never forwarded.
func SetProviderHeaders(req *http.Request, provider string) {
	req.Header.Set("User-Agent", "routatic-proxy")
	switch provider {
	case ProviderOpenCodeGo, ProviderOpenCodeZen:
		req.Header.Set("User-Agent", "opencode/routatic-proxy")
		if provider == ProviderOpenCodeGo {
			if id := core.RequestMetadataFromContext(req.Context()).SessionID; id != "" {
				req.Header.Set("x-opencode-session", id)
			}
		}
	case ProviderOpenRouter:
		req.Header.Set("HTTP-Referer", "https://github.com/routatic/proxy")
		req.Header.Set("X-OpenRouter-Title", "routatic-proxy")
		req.Header.Set("X-OpenRouter-Categories", "cli-agent")
	case ProviderClinePass:
		// The Cline gateway identifies a Cline client by this set, and it
		// gates some roster entries on it. These are the values Cline's own SDK
		// sends (its DEFAULT_CLINE_REQUEST_HEADERS), so a request carrying them
		// is indistinguishable from the official client rather than a partial
		// mirror that one gateway change turns into a 403.
		req.Header.Set("HTTP-Referer", "https://cline.bot")
		req.Header.Set("X-Title", "Cline")
		req.Header.Set("X-IS-MULTIROOT", "false")
		req.Header.Set("X-CLIENT-TYPE", "cline-sdk")
	}
}

func SetAnthropicHeaders(req *http.Request) {
	metadata := core.RequestMetadataFromContext(req.Context())
	version := metadata.AnthropicVersion
	if version == "" {
		version = "2023-06-01"
	}
	req.Header.Set("anthropic-version", version)
	if metadata.AnthropicBeta != "" {
		req.Header.Set("anthropic-beta", metadata.AnthropicBeta)
	}
}

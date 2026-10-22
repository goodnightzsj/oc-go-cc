package gui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTestCatalogDir writes a catalog shaped like the real one: cline-pass
// models under the provider's own namespace, commandcode only in its nested
// map - which is exactly how models.dev publishes it.
func writeTestCatalogDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	body := `{
	  "providers": {
	    "cline-pass": {"name": "cline-pass", "enabled": true,
	      "models": {"deepseek-v4.1-flash": {"name": "DeepSeek V4.1 Flash", "limit": {"context": 1000000, "output": 384000}, "tool_call": true}}},
	    "commandcode": {"name": "commandcode", "enabled": true,
	      "models": {"deepseek/deepseek-v4-pro": {"name": "DeepSeek V4 Pro", "limit": {"context": 1000000, "output": 8192}, "tool_call": true}}}
	  },
	  "models": {
	    "cline-pass/cline-pass": {"name": "placeholder", "limit": {"context": 1, "output": 1}}
	  }
	}`
	if err := os.WriteFile(filepath.Join(dir, "catalog.json"), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return dir
}

// The fallback picker needs the platforms this instance could fall back to,
// which is not the same as the ones currently routed. /v1/models answers the
// second question and hides everything outside the active site, so a
// cross-platform fallback could not be configured at all.
func TestCatalogModelsScopeAllIgnoresTheActiveSite(t *testing.T) {
	for _, cfg := range []string{
		`{"active_site":"cline-pass","cline_pass":{"api_keys":["synthetic-a"]},"commandcode":{"api_keys":["synthetic-b"]}}`,
		// And a platform with no credential must not appear: a fallback there
		// could never be sent.
		`{"active_site":"cline-pass","cline_pass":{"api_keys":["synthetic-a"]},"commandcode":{"api_keys":[]}}`,
	} {
		srv, _ := configTestServer(t, cfg)
		srv.catalogDir = writeTestCatalogDir(t)
		rec := httptest.NewRecorder()
		srv.handleCatalogModels(rec, httptest.NewRequest(http.MethodGet, "/api/catalog/models?scope=all", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d: %s", rec.Code, rec.Body)
		}
		var payload struct {
			Models []struct {
				ID       string `json:"id"`
				Provider string `json:"provider"`
			} `json:"models"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		seen := map[string]bool{}
		for _, m := range payload.Models {
			seen[m.Provider] = true
			if !strings.Contains(m.ID, "/") {
				t.Errorf("id %q is not a sendable wire id", m.ID)
			}
		}
		hasCommandCode := strings.Contains(cfg, `"commandcode":{"api_keys":["synthetic-b"]}`)
		if hasCommandCode && !seen["commandcode"] {
			t.Error("a platform holding a credential must be offered as a fallback target even when it is not the active site")
		}
		if !hasCommandCode && seen["commandcode"] {
			t.Error("a platform with no credential must not be offered; a fallback there can never be sent")
		}
	}
}

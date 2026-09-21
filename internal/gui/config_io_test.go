package gui

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/site"
)

func TestConfigExportAlwaysRedactsSecrets(t *testing.T) {
	cfg := &config.Config{APIKey: "sk-export-secret", Host: "127.0.0.1", Port: 3456}
	cfg.OpenCodeGo.APIKey = "sk-provider-secret"
	srv := &Server{atomicCfg: config.NewAtomicConfig(cfg, "")}

	for _, target := range []string{"/api/config/export", "/api/config/export?anonymize=false"} {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		rec := httptest.NewRecorder()
		srv.handleConfigExport(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d, want 200: %s", target, rec.Code, rec.Body)
		}
		if rec.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("GET %s Cache-Control = %q, want no-store", target, rec.Header().Get("Cache-Control"))
		}
		body := rec.Body.String()
		for _, secret := range []string{"sk-export-secret", "sk-provider-secret"} {
			if strings.Contains(body, secret) {
				t.Errorf("GET %s leaked %q: %s", target, secret, body)
			}
		}
		if !strings.Contains(body, keyMask) {
			t.Errorf("GET %s did not contain the redaction mask: %s", target, body)
		}
	}
}

func TestConfigImportRedactedExportPreservesRawSecret(t *testing.T) {
	t.Setenv("ROUTATIC_PROXY_API_KEY", "sk-resolved-secret")

	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	original := `{"host":"127.0.0.1","port":3456,"api_key":"${ROUTATIC_PROXY_API_KEY}"}` + "\n"
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := config.LoadFromPath(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	srv := &Server{atomicCfg: config.NewAtomicConfig(cfg, path)}

	payload, err := json.Marshal(map[string]interface{}{
		"config": map[string]interface{}{
			"host":    "127.0.0.1",
			"port":    3457,
			"api_key": keyMask,
		},
		"apply": true,
	})
	if err != nil {
		t.Fatalf("marshal import request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/config/import", bytes.NewReader(payload))
	rec := httptest.NewRecorder()
	srv.handleConfigImport(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("import status = %d, want 200: %s", rec.Code, rec.Body)
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read saved config: %v", err)
	}
	got := string(saved)
	if strings.Contains(got, "sk-resolved-secret") || strings.Contains(got, keyMask) {
		t.Fatalf("import wrote a resolved secret or mask to disk: %s", got)
	}
	if !strings.Contains(got, "${ROUTATIC_PROXY_API_KEY}") {
		t.Fatalf("import removed the raw environment placeholder: %s", got)
	}
	if !strings.Contains(got, `"port": 3457`) {
		t.Fatalf("import did not apply non-secret fields: %s", got)
	}
}

// A platform block must be mergeable, or a partial patch replaces the whole
// object and silently drops the settings the user did not touch. Deriving the
// set from the registry means a new platform cannot be added without it.
func TestEveryPlatformBlockIsMergedPartially(t *testing.T) {
	for _, descriptor := range site.All() {
		if !isPartialProviderBlock(descriptor.ID) {
			t.Errorf("platform %q is not merged partially, so a partial save would drop its other settings", descriptor.ID)
		}
	}
	for _, field := range []string{"anthropic_first", "logging", "catalog", "storage"} {
		if !isPartialProviderBlock(field) {
			t.Errorf("%q is not merged partially", field)
		}
	}
	// Routing maps are full replacements by design.
	for _, field := range []string{"models", "fallbacks", "model_overrides", "active_site"} {
		if isPartialProviderBlock(field) {
			t.Errorf("%q must stay a full replacement", field)
		}
	}
}

// The predicate and the merge have to agree: a block reported as partial must
// actually keep its untouched siblings.
func TestPartialMergeKeepsUntouchedPlatformFields(t *testing.T) {
	current := json.RawMessage(`{"base_url":"https://api.cline.bot/api/v1/chat/completions","api_key":"kept","timeout_ms":5000,"stream_timeout_ms":6000}`)
	patch := json.RawMessage(`{"timeout_ms":9000}`)
	merged, err := mergeConfigSettings(current, patch)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(merged, &fields); err != nil {
		t.Fatal(err)
	}
	if fields["timeout_ms"] != float64(9000) {
		t.Errorf("patched field was not applied: %v", fields["timeout_ms"])
	}
	for field, want := range map[string]any{
		"base_url": "https://api.cline.bot/api/v1/chat/completions",
		"api_key":  "kept", "stream_timeout_ms": float64(6000),
	} {
		if fields[field] != want {
			t.Errorf("%s = %v, want %v: a partial save dropped an untouched setting", field, fields[field], want)
		}
	}
}

// The user-visible flow: type a ClinePass key in Settings and save, then adjust
// one field later. This covers the save path end to end -- the key reaches the
// file and survives a subsequent partial edit.
//
// The merge half is guarded separately by TestEveryPlatformBlockIsMergedPartially,
// which fails if the block stops being merged. This test cannot see that on its
// own: the block is absent from the file before the first save, so the merge
// takes its nothing-to-merge branch either way.
func TestClinePassSettingsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	original := `{"api_key":"synthetic-global","host":"127.0.0.1","port":3456,"commandcode":{"api_key":"synthetic-cc"}}`
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	srv := &Server{atomicCfg: config.NewAtomicConfig(cfg, path)}

	// First save: what app.js sends when the key is typed in.
	first := map[string]json.RawMessage{
		"cline_pass": json.RawMessage(`{"api_key":"synthetic-cline-key","base_url":"https://api.cline.bot/api/v1/chat/completions"}`),
	}
	if _, err := srv.updateProxyConfig(first, true); err != nil {
		t.Fatalf("first save rejected: %v", err)
	}
	afterFirst, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(afterFirst), "synthetic-cline-key") {
		t.Fatalf("the first save did not persist the credential: %s", afterFirst)
	}
	// Second save: a later edit touches one timeout only. The key must survive.
	second := map[string]json.RawMessage{
		"cline_pass": json.RawMessage(`{"stream_timeout_ms":45000}`),
	}
	if _, err := srv.updateProxyConfig(second, true); err != nil {
		t.Fatalf("second save rejected: %v", err)
	}

	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"synthetic-cline-key", "api.cline.bot", "45000"} {
		if !strings.Contains(string(saved), want) {
			t.Errorf("saved config is missing %q: a partial save dropped an untouched field", want)
		}
	}
	if !strings.Contains(string(saved), "synthetic-cc") {
		t.Error("saving ClinePass settings dropped the CommandCode settings")
	}
	reloaded, err := config.LoadFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if keys := reloaded.ProviderAPIKeys("cline-pass"); len(keys) == 0 {
		t.Error("cline-pass has no credential after saving one, so it can never be selected")
	}
}

// The same flow over the HTTP endpoint the dashboard actually calls, so the
// whole path is covered rather than only the function behind it.
func TestClinePassSaveOverHTTP(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	original := `{"api_key":"synthetic-global","host":"127.0.0.1","port":3456,"active_site":"commandcode"}`
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	srv := &Server{atomicCfg: config.NewAtomicConfig(cfg, path)}

	patch := `{"cline_pass":{"api_key":"synthetic-cline","base_url":"https://api.cline.bot/api/v1/chat/completions","timeout_ms":300000}}`
	req := httptest.NewRequest(http.MethodPost, "/api/proxy/config", bytes.NewReader([]byte(patch)))
	rec := httptest.NewRecorder()
	srv.handleProxyConfig(rec, req)

	if rec.Code != http.StatusOK && rec.Code != http.StatusNoContent {
		t.Fatalf("save returned %d: %s", rec.Code, rec.Body)
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"synthetic-cline", "api.cline.bot"} {
		if !strings.Contains(string(saved), want) {
			t.Errorf("saved config is missing %q", want)
		}
	}
	reloaded, err := config.LoadFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if keys := reloaded.ProviderAPIKeys("cline-pass"); len(keys) == 0 {
		t.Error("cline-pass still has no credential after an HTTP save")
	}
}

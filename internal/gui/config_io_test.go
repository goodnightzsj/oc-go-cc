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

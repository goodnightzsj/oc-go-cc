package gui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/routatic/proxy/internal/config"
)

func configTestServer(t *testing.T, raw string) (*Server, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	return &Server{atomicCfg: config.NewAtomicConfig(cfg, path)}, path
}

func TestProxyConfigPatchPreservesProviderSiblings(t *testing.T) {
	t.Setenv("GUI_TEST_PROVIDER_KEY", "synthetic-provider-key")
	original := `{"api_key":"synthetic-global-key","host":"127.0.0.1","port":3456,"opencode_go":{"api_key":"${GUI_TEST_PROVIDER_KEY}","base_url":"https://example.invalid/chat","timeout_ms":5000}}`
	srv, path := configTestServer(t, original)
	patch := `{"opencode_go":{"timeout_ms":9000,"api_key":"` + keyMask + `"}}`
	rec := httptest.NewRecorder()
	srv.handleProxyConfig(rec, httptest.NewRequest(http.MethodPost, "/api/proxy/config", strings.NewReader(patch)))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("save status = %d: %s", rec.Code, rec.Body)
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(saved, []byte("${GUI_TEST_PROVIDER_KEY}")) || !bytes.Contains(saved, []byte("https://example.invalid/chat")) {
		t.Fatalf("partial provider patch removed a sibling: %s", saved)
	}
	if bytes.Contains(saved, []byte("synthetic-provider-key")) || bytes.Contains(saved, []byte(keyMask)) {
		t.Fatal("save persisted a resolved secret or redaction mask")
	}
	got := srv.atomicCfg.Get().OpenCodeGo
	if got.TimeoutMs != 9000 || got.APIKey != "synthetic-provider-key" || got.BaseURL != "https://example.invalid/chat" {
		t.Fatal("saved provider settings were not published together")
	}
}

func TestProxyConfigPatchValidationLeavesDiskAndRuntimeUnchanged(t *testing.T) {
	for _, patch := range []string{
		`{"models":{"default":{"provider":"typo","model_id":"test-model"}}}`,
		`{"models":{"default":{"provider":"opencode-go","model_id":"test-model","wire_format":"typo"}}}`,
		`{"api_keys":[""]}`,
	} {
		t.Run(patch, func(t *testing.T) {
			original := `{"api_key":"synthetic-key","host":"127.0.0.1","port":3456}`
			srv, path := configTestServer(t, original)
			before := srv.atomicCfg.Get()
			rec := httptest.NewRecorder()
			srv.handleProxyConfig(rec, httptest.NewRequest(http.MethodPost, "/api/proxy/config", strings.NewReader(patch)))
			if rec.Code != http.StatusBadRequest {
				t.Errorf("invalid save status = %d, want 400: %s", rec.Code, rec.Body)
			}
			saved, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(saved) != original || srv.atomicCfg.Get() != before {
				t.Fatal("rejected config changed disk or live config")
			}
		})
	}
}

func TestConfigImportPreviewUsesSaveValidationAndRedacts(t *testing.T) {
	original := `{"api_key":"synthetic-key","host":"127.0.0.1","port":3456}`
	srv, path := configTestServer(t, original)
	for _, tc := range []struct {
		name   string
		config string
		status int
	}{
		{"invalid provider", `{"host":"127.0.0.1","port":3456,"model_overrides":{"test":{"provider":"typo","model_id":"test"}}}`, http.StatusBadRequest},
		{"valid partial preview", `{"opencode_go":{"api_key":"synthetic-import-key","timeout_ms":9000}}`, http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			body := `{"config":` + tc.config + `,"apply":false}`
			srv.handleConfigImport(rec, httptest.NewRequest(http.MethodPost, "/api/config/import", strings.NewReader(body)))
			if rec.Code != tc.status {
				t.Fatalf("preview status = %d, want %d: %s", rec.Code, tc.status, rec.Body)
			}
			if strings.Contains(rec.Body.String(), "synthetic-import-key") || strings.Contains(rec.Body.String(), "synthetic-key") {
				t.Fatal("preview response exposed credentials")
			}
			if tc.status == http.StatusOK && !strings.Contains(rec.Body.String(), keyMask) {
				t.Fatal("preview response is missing the redaction mask")
			}
		})
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(saved) != original || srv.atomicCfg.Get().OpenCodeGo.TimeoutMs == 9000 {
		t.Fatal("preview modified disk or runtime")
	}
}

func TestProxyConfigPatchReplacesRoutingMaps(t *testing.T) {
	original := `{"api_key":"synthetic-key","host":"127.0.0.1","port":3456,"fallbacks":{"default":[{"model_id":"old-model"}],"fast":[{"model_id":"another-model"}]},"model_overrides":{"old-alias":{"model_id":"old-model"}}}`
	srv, path := configTestServer(t, original)
	patch := `{"fallbacks":{"default":[]},"model_overrides":{}}`
	rec := httptest.NewRecorder()
	srv.handleProxyConfig(rec, httptest.NewRequest(http.MethodPost, "/api/proxy/config", strings.NewReader(patch)))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("save status = %d: %s", rec.Code, rec.Body)
	}
	cfg, err := config.LoadFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Fallbacks) != 1 || len(cfg.Fallbacks["default"]) != 0 || len(cfg.ModelOverrides) != 0 {
		t.Fatal("full routing-map update retained deleted entries")
	}
}

func TestConfigConcurrentUpdatesPreserveAllFields(t *testing.T) {
	srv, path := configTestServer(t, `{"api_key":"synthetic-key","host":"127.0.0.1","port":3456}`)
	providers := []string{"opencode_go", "opencode_zen", "aws_bedrock", "openrouter"}
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i, provider := range providers {
		for j, field := range []string{"timeout_ms", "stream_timeout_ms"} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				patch := fmt.Sprintf(`{"%s":{"%s":%d}}`, provider, field, 9000+i*100+j)
				rec := httptest.NewRecorder()
				if j == 0 {
					srv.handleProxyConfig(rec, httptest.NewRequest(http.MethodPost, "/api/proxy/config", strings.NewReader(patch)))
					if rec.Code != http.StatusNoContent {
						t.Errorf("save status = %d: %s", rec.Code, rec.Body)
					}
				} else {
					body := `{"config":` + patch + `,"apply":true}`
					srv.handleConfigImport(rec, httptest.NewRequest(http.MethodPost, "/api/config/import", strings.NewReader(body)))
					if rec.Code != http.StatusOK {
						t.Errorf("import status = %d: %s", rec.Code, rec.Body)
					}
				}
			}()
		}
	}
	close(start)
	wg.Wait()
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Only provider sections have numeric object fields.
	var disk map[string]json.RawMessage
	if err := json.Unmarshal(saved, &disk); err != nil {
		t.Fatal(err)
	}
	for i, provider := range providers {
		var fields map[string]int
		if err := json.Unmarshal(disk[provider], &fields); err != nil {
			t.Fatalf("missing provider %s: %v", provider, err)
		}
		if fields["timeout_ms"] != 9000+i*100 || fields["stream_timeout_ms"] != 9001+i*100 {
			t.Errorf("concurrent updates lost fields for %s: %v", provider, fields)
		}
	}
	diskConfig, err := config.LoadFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := json.Marshal(diskConfig)
	got, _ := json.Marshal(srv.atomicCfg.Get())
	if !bytes.Equal(got, want) {
		t.Fatal("published config differs from persisted config")
	}
}

func TestCommandCodeConfigRoundTripPreservesIndependentSecrets(t *testing.T) {
	t.Setenv("GUI_TEST_COMMANDCODE_KEY", "synthetic-commandcode-key")
	t.Setenv("GUI_TEST_COMMANDCODE_POOL", "synthetic-commandcode-pool")
	original := `{"api_key":"synthetic-global-key","host":"127.0.0.1","port":3456,"commandcode":{"base_url":"https://commandcode.invalid/v1/chat/completions","anthropic_base_url":"https://commandcode.invalid/v1/messages","api_key":"${GUI_TEST_COMMANDCODE_KEY}","api_keys":["${GUI_TEST_COMMANDCODE_POOL}"],"timeout_ms":5000,"stream_timeout_ms":6000,"streaming_timeout_ms":7000}}`
	srv, path := configTestServer(t, original)
	patch := `{"commandcode":{"timeout_ms":9000,"zero_data_retention":true,"api_key":"` + keyMask + `","api_keys":["` + keyMask + `"]}}`
	rec := httptest.NewRecorder()
	srv.handleProxyConfig(rec, httptest.NewRequest(http.MethodPost, "/api/proxy/config", strings.NewReader(patch)))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("save status = %d: %s", rec.Code, rec.Body)
	}
	for _, endpoint := range []string{"/api/proxy/config", "/api/config/export"} {
		t.Run(endpoint, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, endpoint, nil)
			if endpoint == "/api/proxy/config" {
				srv.handleProxyConfig(rec, req)
			} else {
				srv.handleConfigExport(rec, req)
			}
			if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("read status = %d, cache = %q", rec.Code, rec.Header().Get("Cache-Control"))
			}
			var response map[string]json.RawMessage
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			var fields map[string]any
			if err := json.Unmarshal(response["commandcode"], &fields); err != nil {
				t.Fatalf("CommandCode settings missing from %s: %v", endpoint, err)
			}
			if fields["base_url"] != "https://commandcode.invalid/v1/chat/completions" || fields["anthropic_base_url"] != "https://commandcode.invalid/v1/messages" || fields["timeout_ms"] != float64(9000) || fields["stream_timeout_ms"] != float64(6000) || fields["streaming_timeout_ms"] != float64(7000) || fields["zero_data_retention"] != true {
				t.Fatal("partial save lost CommandCode endpoint, timeout or ZDR settings")
			}
			if fields["api_key"] != keyMask || !bytes.Contains(response["commandcode"], []byte(keyMask)) {
				t.Fatal("CommandCode keys were not redacted")
			}
			for _, secret := range []string{"synthetic-global-key", "synthetic-commandcode-key", "synthetic-commandcode-pool"} {
				if bytes.Contains(rec.Body.Bytes(), []byte(secret)) {
					t.Fatal("settings response exposed a credential")
				}
			}
			body := `{"config":` + rec.Body.String() + `,"apply":true}`
			imported := httptest.NewRecorder()
			srv.handleConfigImport(imported, httptest.NewRequest(http.MethodPost, "/api/config/import", strings.NewReader(body)))
			if imported.Code != http.StatusOK {
				t.Fatalf("redacted import status = %d: %s", imported.Code, imported.Body)
			}
		})
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, placeholder := range []string{"${GUI_TEST_COMMANDCODE_KEY}", "${GUI_TEST_COMMANDCODE_POOL}"} {
		if !bytes.Contains(saved, []byte(placeholder)) {
			t.Fatal("redacted round trip removed the original key placeholder")
		}
	}
	if bytes.Contains(saved, []byte("synthetic-commandcode-key")) || bytes.Contains(saved, []byte("synthetic-commandcode-pool")) || bytes.Contains(saved, []byte(keyMask)) {
		t.Fatal("redacted round trip persisted an expanded key or a mask")
	}
}

func TestProxyConfigRejectsMixedRedactedKeyPool(t *testing.T) {
	original := `{"api_key":"synthetic-global-key","host":"127.0.0.1","port":3456}`
	srv, path := configTestServer(t, original)
	rec := httptest.NewRecorder()
	patch := `{"commandcode":{"api_keys":["` + keyMask + `","synthetic-new-key"]}}`
	srv.handleProxyConfig(rec, httptest.NewRequest(http.MethodPost, "/api/proxy/config", strings.NewReader(patch)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("mixed pool save status = %d, want 400", rec.Code)
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(saved) != original {
		t.Fatal("rejected key pool changed persisted configuration")
	}
}

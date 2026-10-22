package gui

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestProxyConfigPreservesEnvironmentKeyPool(t *testing.T) {
	for _, suffix := range []string{"API_KEY", "API_KEYS"} {
		t.Run(suffix, func(t *testing.T) {
			t.Setenv("ROUTATIC_PROXY_CLINE_PASS_"+suffix, "synthetic-env")
			t.Setenv("GUI_MIGRATION_KEY", "synthetic-file")
			srv, path := configTestServer(t, `{"cline_pass":{"api_key":"${GUI_MIGRATION_KEY}","api_keys":["synthetic-array"]}}`)
			before := srv.atomicCfg.Get().ClinePass.EffectiveAPIKeys()
			rec := httptest.NewRecorder()
			srv.handleProxyConfig(rec, httptest.NewRequest(http.MethodPost, "/api/proxy/config", strings.NewReader(`{"cline_pass":{"timeout_ms":9000}}`)))
			if rec.Code != http.StatusNoContent {
				t.Fatalf("save: %d %s", rec.Code, rec.Body)
			}
			if !reflect.DeepEqual(before, srv.atomicCfg.Get().ClinePass.EffectiveAPIKeys()) {
				t.Fatal("save changed the pool")
			}
			masked, err := anonymizeConfig(srv.atomicCfg.Get())
			if err != nil {
				t.Fatal(err)
			}
			redacted, err := json.Marshal(masked)
			if err != nil {
				t.Fatal(err)
			}
			body := append([]byte(`{"apply":true,"config":`), redacted...)
			body = append(body, '}')
			rec = httptest.NewRecorder()
			srv.handleConfigImport(rec, httptest.NewRequest(http.MethodPost, "/api/proxy/config/import", bytes.NewReader(body)))
			if rec.Code != http.StatusOK {
				t.Fatalf("import: %d %s", rec.Code, rec.Body)
			}
			if !reflect.DeepEqual(before, srv.atomicCfg.Get().ClinePass.EffectiveAPIKeys()) {
				t.Fatal("redacted replay changed the pool")
			}
			saved, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(saved, []byte("${GUI_MIGRATION_KEY}")) || bytes.Contains(saved, []byte("synthetic-env")) {
				t.Fatal("raw placeholder lost or environment credential persisted")
			}
		})
	}
}

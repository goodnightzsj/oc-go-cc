package gui

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestNonemptyFallbackSaveAndImport(t *testing.T) {
	const original = `{"api_keys":["synthetic-global"]}`
	const patch = `{"fallbacks":{"default":[{"provider":"opencode-go","model_id":"synthetic-model"}]}}`
	for _, mode := range []string{"save", "preview", "import"} {
		t.Run(mode, func(t *testing.T) {
			srv, path := configTestServer(t, original)
			rec := httptest.NewRecorder()
			want := http.StatusOK
			if mode == "save" {
				srv.handleProxyConfig(rec, httptest.NewRequest(http.MethodPost, "/api/proxy/config", strings.NewReader(patch)))
				want = http.StatusNoContent
			} else {
				apply := "false"
				if mode == "import" {
					apply = "true"
				}
				srv.handleConfigImport(rec, httptest.NewRequest(http.MethodPost, "/api/proxy/config/import", strings.NewReader(`{"apply":`+apply+`,"config":`+patch+`}`)))
			}
			if rec.Code != want {
				t.Fatalf("status %d: %s", rec.Code, rec.Body)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "preview" {
				if string(raw) != original || len(srv.atomicCfg.Get().Fallbacks) != 0 {
					t.Fatal("preview changed config")
				}
			} else if got := srv.atomicCfg.Get().Fallbacks["default"]; len(got) != 1 || got[0].ModelID != "synthetic-model" {
				t.Fatal("fallback not published")
			}
		})
	}
}

package gui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/routatic/proxy/internal/site"
)

// The dashboard must not offer a platform this deployment cannot call: picking
// one would route every request into an authentication failure. The answer is
// computed server-side because the credential rule lives in config.
func TestSitesEndpointReportsSelectability(t *testing.T) {
	raw := `{"api_key":"synthetic-global","commandcode":{"api_key":"synthetic-commandcode"}}`
	srv, _ := configTestServer(t, raw)

	rec := httptest.NewRecorder()
	srv.handleSites(rec, httptest.NewRequest(http.MethodGet, "/api/sites", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Error("selectability changes with the config and must not be browser-cached")
	}
	var body struct {
		Sites []siteView `json:"sites"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Sites) != len(site.Visible()) {
		t.Fatalf("listed %d platforms, registry offers %d", len(body.Sites), len(site.Visible()))
	}
	for i, want := range site.Visible() {
		got := body.Sites[i]
		if got.ID != want.ID || got.Name != want.DisplayName || got.Order != want.Order {
			t.Errorf("site[%d] = %+v, want %s/%s", i, got, want.ID, want.DisplayName)
		}
		switch got.ID {
		case site.OpenCodeGo:
			// No key of its own, so it falls back to the global key.
			if !got.Selectable {
				t.Error("a platform using the global key was reported as unconfigured")
			}
		case site.CommandCode:
			if !got.Selectable {
				t.Error("a platform with its own key was reported as unconfigured")
			}
		}
	}

	// Without a global key, the platform that depends on it stops being offered.
	bare, _ := configTestServer(t, `{"commandcode":{"api_key":"synthetic-commandcode"}}`)
	rec = httptest.NewRecorder()
	bare.handleSites(rec, httptest.NewRequest(http.MethodGet, "/api/sites", nil))
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for _, view := range body.Sites {
		wantSelectable := view.ID == site.CommandCode
		if view.Selectable != wantSelectable {
			t.Errorf("%s selectable = %v, want %v", view.ID, view.Selectable, wantSelectable)
		}
	}
}

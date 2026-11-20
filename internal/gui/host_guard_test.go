package gui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The dashboard holds the operator's credentials and binds to loopback, but a
// browser does not treat "the same machine" as a boundary: DNS rebinding points
// a public name at 127.0.0.1, after which the attacker's page is same-origin
// and CORS, CSP and Sec-Fetch-Site all pass. The Host header is the one thing
// rebinding cannot forge.
func TestSecurityHeadersRejectForeignHost(t *testing.T) {
	var reached bool
	handler := securityHeadersMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	}), map[string]bool{"dashboard.example": true})

	for _, tc := range []struct {
		host string
		want int
	}{
		{"evil.com:3445", http.StatusForbidden},
		{"127.0.0.1.nip.io:3445", http.StatusForbidden},
		{"", http.StatusForbidden},
		{"localhost:3445", http.StatusOK},
		{"127.0.0.1:3445", http.StatusOK},
		{"[::1]:3445", http.StatusOK},
		{"LOCALHOST", http.StatusOK},
		{"dashboard.example:443", http.StatusOK},
	} {
		reached = false
		req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
		req.Host = tc.host
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Errorf("Host %q: status = %d, want %d", tc.host, rec.Code, tc.want)
		}
		if (tc.want == http.StatusOK) != reached {
			t.Errorf("Host %q reached the handler = %v", tc.host, reached)
		}
	}
}

// A page on any site can POST to 127.0.0.1:3445 with a legitimate Host header,
// and the JSON decoders do not look at Content-Type, so a text/plain form body
// would be accepted. That is plain CSRF and needs no rebinding.
func TestSecurityHeadersRejectCrossSiteWrite(t *testing.T) {
	handler := securityHeadersMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), nil)

	crossSite := httptest.NewRequest(http.MethodPost, "/api/proxy/config", nil)
	crossSite.Host = "127.0.0.1:3445"
	crossSite.Header.Set("Sec-Fetch-Site", "cross-site")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, crossSite)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("cross-site POST = %d, want 403", rec.Code)
	}

	sameSite := httptest.NewRequest(http.MethodPost, "/api/proxy/config", nil)
	sameSite.Host = "localhost:3445"
	sameSite.Header.Set("Sec-Fetch-Site", "same-origin")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, sameSite)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("same-origin POST = %d, want 204", rec.Code)
	}
}

func TestSecurityHeadersRefuseFraming(t *testing.T) {
	handler := securityHeadersMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), nil)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "localhost:3445"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if got := rec.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("X-Frame-Options = %q", got)
	}
	if csp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "frame-ancestors 'none'") {
		t.Fatalf("CSP lacks frame-ancestors: %q", csp)
	}
}

// A dashboard behind a reverse proxy sees the public name, not loopback. That
// name has to be declared, or every page is refused - which is how this shipped
// broken once.
func TestGuiAllowedHostsFromEnvironment(t *testing.T) {
	t.Setenv("ROUTATIC_PROXY_GUI_ALLOWED_HOSTS", " dash.example , [::2]:8443 ,,")
	extra := guiAllowedHosts()
	if !extra["dash.example"] || !extra["::2"] || len(extra) != 2 {
		t.Fatalf("parsed hosts = %v", extra)
	}
	if !dashboardHostAllowed("dash.example:443", extra) {
		t.Fatal("declared host refused")
	}
	if dashboardHostAllowed("evil.example:443", extra) {
		t.Fatal("undeclared host allowed")
	}
}

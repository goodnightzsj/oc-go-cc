package catalog

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/site"
)

func TestSiteModelsURLDerivation(t *testing.T) {
	for _, tc := range []struct{ endpoint, want string }{
		{"https://api.commandcode.ai/provider/v1/chat/completions", "https://api.commandcode.ai/provider/v1/models"},
		{"https://api.commandcode.ai/provider/v1/chat/completions/", "https://api.commandcode.ai/provider/v1/models"},
		{"http://127.0.0.1:3456/provider/v1/chat/completions", "http://127.0.0.1:3456/provider/v1/models"},
	} {
		got, err := SiteModelsURL(site.CommandCode, tc.endpoint)
		if err != nil || got != tc.want {
			t.Errorf("SiteModelsURL(%q) = %q, %v; want %q", tc.endpoint, got, err, tc.want)
		}
	}

	// Anything that is not the published shape must be refused rather than
	// guessed at: a derived URL is still a request aimed at a real host.
	for _, endpoint := range []string{
		"",
		"ftp://api.commandcode.ai/provider/v1/chat/completions",
		"https://user:pass@api.commandcode.ai/provider/v1/chat/completions",
		"https://api.commandcode.ai/provider/v1/chat/completions?key=secret",
		"https://api.commandcode.ai/provider/v1/chat/completions#frag",
		"https://api.commandcode.ai/provider/v1/messages",
		"https://api.commandcode.ai/other",
	} {
		if got, err := SiteModelsURL(site.CommandCode, endpoint); err == nil || got != "" {
			t.Errorf("unsafe or unrelated endpoint derived %q from %q", got, endpoint)
		}
	}

	// A platform without a published model list has none derived for it, even
	// if its endpoint happens to end the same way.
	if got, err := SiteModelsURL(site.OpenCodeGo, "https://opencode.ai/zen/go/v1/chat/completions"); err == nil || got != "" {
		t.Errorf("OpenCode Go derived a model endpoint: %q %v", got, err)
	}
}

func TestFetchSiteModels(t *testing.T) {
	valid := `{"object":"list","data":[{"id":"deepseek/deepseek-v4-flash","name":"DeepSeek V4 Flash"},{"id":"zai-org/GLM-5.2","name":"GLM-5.2"},{"id":""}]}`
	for _, tc := range []struct {
		name    string
		status  int
		body    string
		wantIDs []string
		wantErr bool
	}{
		{name: "list", status: 200, body: valid, wantIDs: []string{"deepseek/deepseek-v4-flash", "zai-org/GLM-5.2"}},
		{name: "http error", status: 503, body: `{"error":"down"}`, wantErr: true},
		{name: "not json", status: 200, body: `<html>`, wantErr: true},
		// An empty list and a failed fetch are different answers; reporting the
		// first for the second would silently empty a client's model picker.
		{name: "empty list", status: 200, body: `{"data":[]}`, wantErr: true},
		{name: "all ids blank", status: 200, body: `{"data":[{"id":"  "}]}`, wantErr: true},
		{name: "oversize", status: 200, body: strings.Repeat(" ", maxSiteModelsBytes+1), wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
					t.Error("the model list is public and must not carry a credential")
				}
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprint(w, tc.body)
			}))
			defer server.Close()

			client := &http.Client{Timeout: 5 * time.Second}
			models, err := FetchSiteModels(context.Background(), client, site.CommandCode, server.URL)
			if tc.wantErr {
				if err == nil || models != nil {
					t.Fatalf("FetchSiteModels = %+v, %v; want an error", models, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var ids []string
			for _, m := range models {
				ids = append(ids, m.ID)
				if m.Provider != site.CommandCode {
					t.Errorf("%s carries provider %q", m.ID, m.Provider)
				}
			}
			if strings.Join(ids, ",") != strings.Join(tc.wantIDs, ",") {
				t.Fatalf("ids = %v, want %v", ids, tc.wantIDs)
			}
		})
	}

	// A cancelled context must not be reported as an empty list.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if models, err := FetchSiteModels(ctx, &http.Client{Timeout: time.Second}, site.CommandCode, "http://127.0.0.1:1/models"); err == nil || models != nil {
		t.Fatalf("cancelled fetch = %+v, %v", models, err)
	}
}

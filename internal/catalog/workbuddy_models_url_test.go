package catalog

import (
	"testing"

	"github.com/routatic/proxy/internal/site"
)

// The gateway's own panel shows a bare host, the OpenAI convention stops at
// /v1, and a copy-pasted value often carries the full chat path. All three are
// what an operator types, so all three must resolve to the same list URL -
// otherwise the picker is empty for a reason nobody can see.
func TestWorkBuddyModelsURLAcceptsEverySpelling(t *testing.T) {
	const want = "http://127.0.0.1:7863/v1/models"
	for _, endpoint := range []string{
		"http://127.0.0.1:7863",
		"http://127.0.0.1:7863/",
		"http://127.0.0.1:7863/v1",
		"http://127.0.0.1:7863/v1/",
		"http://127.0.0.1:7863/v1/chat/completions",
		"  http://127.0.0.1:7863  ",
	} {
		got, err := SiteModelsURL(site.WorkBuddy, endpoint)
		if err != nil || got != want {
			t.Errorf("SiteModelsURL(%q) = %q, %v; want %q", endpoint, got, err, want)
		}
	}
}

// The gateway need not be local: an operator may reach it through a tunnel or
// on another machine. The derivation keeps whatever origin was configured.
func TestWorkBuddyModelsURLKeepsTheConfiguredOrigin(t *testing.T) {
	if got, err := SiteModelsURL(site.WorkBuddy, "https://wb.example.com/v1/chat/completions"); err != nil || got != "https://wb.example.com/v1/models" {
		t.Errorf("got %q, %v", got, err)
	}
}

// The same refusals the other platforms apply: a URL carrying credentials,
// a query or a fragment is not something to derive from.
func TestWorkBuddyModelsURLRefusesUnsafeEndpoints(t *testing.T) {
	for _, endpoint := range []string{
		"",
		"   ",
		"ftp://127.0.0.1:7863",
		"https://user:pass@127.0.0.1:7863",
		"http://127.0.0.1:7863?key=secret",
		"http://127.0.0.1:7863#frag",
		"not-a-url",
	} {
		if got, err := SiteModelsURL(site.WorkBuddy, endpoint); err == nil || got != "" {
			t.Errorf("unsafe endpoint %q derived %q", endpoint, got)
		}
	}
}

// A gateway the operator runs is the one platform where rewriting the path is
// safe - there is no second host the credential could reach. This pins that the
// rewrite does not leak into the platforms where it would be dangerous.
func TestWorkBuddyDerivationDoesNotLoosenOtherPlatforms(t *testing.T) {
	if _, err := SiteModelsURL(site.CommandCode, "http://127.0.0.1:3456"); err == nil {
		t.Error("CommandCode accepted a bare host; the suffix rule must still apply to it")
	}
	if _, err := SiteModelsURL(site.ClinePass, "http://127.0.0.1:3456"); err == nil {
		t.Error("ClinePass accepted a bare host; its exact-path check must still apply")
	}
}

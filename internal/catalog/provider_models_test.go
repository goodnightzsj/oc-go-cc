package catalog

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// models.dev publishes each provider's roster twice: a top-level "models" map
// keyed by the full provider/model id, and a nested per-provider map keyed by
// the bare name. They are not the same set - a provider added after the
// top-level map was built (cline-pass is one) appears only in the nested view.
// Reading only the top level silently loses that provider's capabilities while
// its models still list, which is how a 384000-token output ceiling became the
// built-in registry's 8192 and clamped real requests.
func TestLoadFoldsProviderNestedModels(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "catalog.json")
	body := `{
	  "providers": {
	    "cline-pass": {
	      "name": "ClinePass",
	      "models": {
	        "deepseek-v4-pro": {"name": "DeepSeek V4 Pro", "limit": {"context": 1000000, "output": 384000}, "tool_call": true}
	      }
	    },
	    "other": {"name": "Other"}
	  },
	  "models": {
	    "other/model": {"name": "Other", "limit": {"context": 1000, "output": 100}}
	  }
	}`
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}

	idx, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	model, ok := idx.Models["cline-pass/deepseek-v4-pro"]
	if !ok {
		t.Fatal("the provider-nested model is unreachable: Load read only the top-level map")
	}
	if got := model.MaxOutputTokens(); got != 384000 {
		t.Errorf("max output = %d, want the published 384000", got)
	}
	if got := model.ContextWindow(); got != 1000000 {
		t.Errorf("context = %d, want 1000000", got)
	}
	// The fixture's own entries are what this asserts. A total count would also
	// count the entries the override table adds, and would break every time that
	// table grows - it is not what this test is about.
	if _, ok := idx.Models["other/model"]; !ok {
		t.Error("the top-level model is missing")
	}
	if got, want := len(idx.Models), 2+overrideEntryCount("cline-pass"); got != want {
		t.Errorf("merged catalog holds %d models, want %d (fixture + overrides)", got, want)
	}
}

// A model the catalog does not carry at all must resolve to zero rather than to
// a guess: zero means "no ceiling recorded" and the caller treats it as such.
func TestMaxOutputTokensIsZeroWhenUnknown(t *testing.T) {
	if got := (Model{}).MaxOutputTokens(); got != 0 {
		t.Errorf("unknown model = %d, want 0", got)
	}
	if got := (Model{Limit: &Limit{Context: 500}}).MaxOutputTokens(); got != 0 {
		t.Errorf("context-only limit = %d, want 0", got)
	}
}

// models.dev qualifies the nested key for some providers and leaves it bare for
// others, and the merge must not prefix a key that already carries its provider.
//
// This is the live shape: all 18 of cline-pass's nested keys are already
// "cline-pass/<name>", so prefixing produced "cline-pass/cline-pass/<name>" as a
// second entry beside the correct one. Both reached GET /v1/models, and the
// doubled id was reachable rather than inert - ResolveShort matched it and
// returned the whole doubled string as the upstream model id.
func TestMergeDoesNotDoublePrefixAnAlreadyQualifiedNestedKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "catalog.json")
	body := `{
	  "providers": {
	    "other": {"name": "Other"},
	    "cline-pass": {
	      "name": "ClinePass",
	      "models": {
	        "cline-pass/deepseek-v4.1-flash": {"name": "DeepSeek V4.1 Flash", "limit": {"context": 1000000, "output": 384000}, "tool_call": true},
	        "glm-5.3": {"name": "GLM-5.3", "limit": {"context": 200000, "output": 8192}, "tool_call": true}
	      }
	    }
	  },
	  "models": {"other/model": {"name": "Other", "limit": {"context": 1000, "output": 100}}}
	}`
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	idx, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if _, ok := idx.Models["cline-pass/cline-pass/deepseek-v4.1-flash"]; ok {
		t.Error("an already-qualified nested key must not be prefixed a second time")
	}
	model, ok := idx.Models["cline-pass/deepseek-v4.1-flash"]
	if !ok {
		t.Fatalf("the qualified nested key is missing; keys = %v", keysOf(idx.Models))
	}
	if got := model.MaxOutputTokens(); got != 384000 {
		t.Errorf("max output = %d, want 384000", got)
	}

	// A bare nested key is still prefixed: both shapes occur in the same feed.
	if _, ok := idx.Models["cline-pass/glm-5.3"]; !ok {
		t.Errorf("a bare nested key must still be qualified; keys = %v", keysOf(idx.Models))
	}
}

func keysOf(m map[string]Model) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

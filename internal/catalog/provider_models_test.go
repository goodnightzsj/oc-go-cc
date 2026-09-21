package catalog

import (
	"os"
	"path/filepath"
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
	// The top-level map is the view other code already resolves against, so a
	// key present in both must keep the top level's entry.
	if len(idx.Models) != 2 {
		t.Errorf("merged catalog holds %d models, want 2", len(idx.Models))
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

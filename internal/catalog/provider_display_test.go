package catalog

import (
	"strings"
	"testing"
)

func TestAmbiguousModelListsProvidersInDisplayOrder(t *testing.T) {
	catalog := &IndexedCatalog{Catalog: Catalog{Providers: map[string]Provider{}, Models: map[string]Model{}}}
	for _, provider := range []string{"aws-bedrock", "openrouter", "commandcode", "opencode-zen", "opencode-go"} {
		catalog.Providers[provider] = Provider{Name: provider}
		id := provider + "/shared"
		catalog.Models[id] = Model{ID: id, Name: "shared"}
	}
	_, err := catalog.ResolveShort("shared")
	want := "[opencode-go, commandcode, opencode-zen, aws-bedrock, openrouter]"
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("ambiguous model error = %v, want provider list %s", err, want)
	}
}

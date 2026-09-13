package site

import "testing"

// DefaultID falls back to the first entry, so a registry with no Default marker
// would silently route every unset provider to whatever happens to be first.
// More than one marker would make the answer depend on iteration order.
func TestDefaultPlatformIsUnique(t *testing.T) {
	var defaults []string
	for _, d := range registry {
		if d.Default {
			defaults = append(defaults, d.ID)
		}
	}
	if len(defaults) != 1 {
		t.Fatalf("descriptors marked Default = %v, want exactly one", defaults)
	}
	if got := DefaultID(); got != defaults[0] {
		t.Fatalf("DefaultID() = %q, want %q", got, defaults[0])
	}
}

func TestOrderIsUniqueAndDense(t *testing.T) {
	seen := make(map[int]string, len(registry))
	for _, d := range registry {
		if other, dup := seen[d.Order]; dup {
			t.Fatalf("%s and %s share display order %d", other, d.ID, d.Order)
		}
		seen[d.Order] = d.ID
	}
	for i := range registry {
		if _, ok := seen[i]; !ok {
			t.Fatalf("display order %d is unused, so the ordering has a gap", i)
		}
	}
}

// The stored contract: these names appear in requests.provider and in existing
// config files, and the client package re-exports them.
func TestNormalizeMatchesStoredContract(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"", OpenCodeGo}, // an unset provider means OpenCode Go
		{"opencode_go", OpenCodeGo},
		{"commandcode", CommandCode},
		{"OpenCode-Go", "OpenCode-Go"}, // case is not ours to fix here
		{"unknown", "unknown"},
	} {
		if got := Normalize(tc.in); got != tc.want {
			t.Errorf("Normalize(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestLookupAndOrderSortUnknownLast(t *testing.T) {
	for _, id := range []string{OpenCodeGo, OpenCodeZen, AWSBedrock, OpenRouter, CommandCode} {
		d, ok := Lookup(id)
		if !ok || d.ID != id {
			t.Fatalf("Lookup(%q) = %+v, %v", id, d, ok)
		}
		if !IsKnown(id) {
			t.Fatalf("IsKnown(%q) = false", id)
		}
	}
	if _, ok := Lookup("not-a-platform"); ok || IsKnown("not-a-platform") {
		t.Fatal("an unknown name was accepted as a platform")
	}
	if got, want := Order("not-a-platform"), len(registry); got != want {
		t.Fatalf("Order(unknown) = %d, want %d", got, want)
	}
	// Underscore spelling must resolve to the same descriptor.
	if d, ok := Lookup("opencode_go"); !ok || d.ID != OpenCodeGo {
		t.Fatalf("legacy spelling did not resolve: %+v %v", d, ok)
	}
}

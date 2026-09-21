package storage

import "testing"

func TestMergePriceEntries(t *testing.T) {
	seed := []priceEntry{{Match: "a", Input: 1}, {Match: "b", Input: 2}, {Match: "c", Input: 3}}
	fetched := []priceEntry{{Match: "b", Input: 20}, {Match: "d", Input: 4}}
	got := mergePriceEntries(seed, fetched)
	byMatch := map[string]priceEntry{}
	for _, e := range got {
		byMatch[e.Match] = e
	}
	if len(got) != 4 {
		t.Fatalf("got %d rules, want 4 (b and d fetched, a and c kept)", len(got))
	}
	if byMatch["b"].Input != 20 {
		t.Errorf("b = %v, want the fetched 20", byMatch["b"].Input)
	}
	for _, keep := range []string{"a", "c"} {
		if _, ok := byMatch[keep]; !ok {
			t.Errorf("seed-only rule %q was dropped", keep)
		}
	}
	if len(mergePriceEntries(nil, fetched)) != 2 {
		t.Error("with no seed the fetched table must pass through")
	}
}

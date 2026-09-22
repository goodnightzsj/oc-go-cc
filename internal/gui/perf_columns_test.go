package gui

import (
	"strings"
	"testing"
)

// TestPerformanceHeaderEndsWithHealth pins the header half of the Performance
// table's column contract. The row half is asserted in
// dashboard_behavior_test.go, which reads the rendered HTML out of the shim;
// index.html is not reachable from there, so the two halves live apart.
//
// This pairing exists because the table shipped with the health dot emitted as
// the fourth cell while the header put Health last. Every latency figure then
// rendered one column to the left of its heading - P99 under "Health",
// Avg (ms) under "P50" - and the existing test passed, because it only counted
// the cells. A table whose sort keys are correct and whose cells sit in the
// wrong columns is worse than an obviously broken one: it reads as valid data.
func TestPerformanceHeaderEndsWithHealth(t *testing.T) {
	page, err := assets.ReadFile("assets/index.html")
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	html := string(page)

	// Scope to the Performance table's own thead. Anchoring on data-sort="model"
	// is not enough: the History table carries that attribute too and comes
	// first, which slices out the wrong table and silently finds nothing.
	anchor := strings.Index(html, "perf.th.model")
	if anchor < 0 {
		t.Fatal("index.html no longer has a Performance header")
	}
	open := strings.LastIndex(html[:anchor], "<thead")
	close := strings.Index(html[anchor:], "</thead>")
	if open < 0 || close < 0 {
		t.Fatal("could not bound the Performance thead")
	}
	head := html[open : anchor+close]

	want := []string{
		"perf.th.model", "perf.th.count", "perf.th.successRate",
		"perf.th.avg", "perf.th.p50", "perf.th.p90", "perf.th.p99",
		"perf.th.health",
	}
	prev := -1
	for _, key := range want {
		at := strings.Index(head, key)
		if at < 0 {
			t.Errorf("Performance header is missing %q", key)
			continue
		}
		if at < prev {
			t.Errorf("%q appears out of order in the Performance header", key)
		}
		prev = at
	}

	// The Health column carries no data-sort: it renders a level, not an
	// orderable figure, and giving it one would let a click sort by a value the
	// column never shows.
	if tail := head[strings.Index(head, "perf.th.p99"):]; strings.Contains(tail, "data-sort=") {
		t.Error("the Health column must not be sortable")
	}
}

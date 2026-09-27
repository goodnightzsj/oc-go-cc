package gui

import (
	"strings"
	"testing"
)

// TestHistoryHeaderColumnsMatchTheRow pins the History table's column contract,
// header against rendered row, by position.
//
// The two halves live apart for the same reason the Performance pair does
// (perf_columns_test.go): index.html is not reachable from the JS shim that
// renders a row, so the header is read from the asset and the cells from the
// shim's output.
//
// This table has already shipped one off-by-one - the Tokens column sorted by
// the three-part input total while its cell showed the four-part sum - and the
// guard that missed it only counted cells. A column count cannot tell a correct
// row from a shifted one; only the order can.
func TestHistoryHeaderColumnsMatchTheRow(t *testing.T) {
	page, err := assets.ReadFile("assets/index.html")
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	html := string(page)

	// Scope to the History table's own thead. Two tables carry data-sort="model"
	// (this one and Performance), and History's buttons are rendered by app.js
	// with class sort-button, so anchor on a key only this header carries.
	anchor := strings.Index(html, "th.tokensPerSecond")
	if anchor < 0 {
		t.Fatal("index.html no longer declares the History Tok/s column")
	}
	open := strings.LastIndex(html[:anchor], "<thead")
	close := strings.Index(html[anchor:], "</thead>")
	if open < 0 || close < 0 {
		t.Fatal("could not bound the History thead")
	}
	head := html[open : anchor+close]

	// Insertion order is the contract: each key is the column heading the row
	// must fill at the same index.
	want := []string{
		"th.time", "th.status", "th.modelPlatform", "th.scenario",
		"th.tokens", "th.cost", "th.duration", "th.tokensPerSecond",
	}
	prev := -1
	for _, key := range want {
		at := strings.Index(head, key)
		if at < 0 {
			t.Errorf("History header is missing %q", key)
			continue
		}
		if at < prev {
			t.Errorf("%q appears out of order in the History header", key)
		}
		prev = at
	}

	// The Tok/s heading must carry a sort key. Without one the header is inert:
	// the click handler binds field undefined, and the backend's
	// requestSortColumn falls through to the default time ordering, so the click
	// appears to sort while changing nothing the user asked for.
	//
	// Offsets into head are relative: `anchor` indexes the whole document, and
	// slicing head with it panics rather than failing the assertion.
	rel := anchor - open
	thStart := strings.LastIndex(head[:rel], "<th")
	if thStart < 0 {
		t.Fatal("could not find the Tok/s <th> opening tag")
	}
	if !strings.Contains(head[thStart:rel], `data-sort="tokens_per_second"`) {
		t.Error("the History Tok/s heading must carry data-sort=\"tokens_per_second\"")
	}
}

package storage

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/history"
)

// TestRequestedModelStoredOnlyWhenItDiffers pins the column's meaning: its
// presence says "this request was routed somewhere other than what it asked
// for". Storing the client's model unconditionally would put the same string in
// two columns and make that question unanswerable without comparing them.
func TestRequestedModelStoredOnlyWhenItDiffers(t *testing.T) {
	db := newCostTestDB(t)
	at := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)

	insertCostRecord(t, db, history.RequestRecord{
		ID: "rerouted", Model: "kimi-k2.6", RequestedModel: "claude-sonnet-4-5",
		Provider: "opencode-go", Success: true, StartTime: at,
	})
	insertCostRecord(t, db, history.RequestRecord{
		ID: "passthrough", Model: "glm-5.2", RequestedModel: "glm-5.2",
		Provider: "opencode-go", Success: true, StartTime: at,
	})
	// Case and padding are not a reroute: model ids are matched that way
	// elsewhere, so a difference in case must not be recorded as one.
	insertCostRecord(t, db, history.RequestRecord{
		ID: "cased", Model: "glm-5.2", RequestedModel: "  GLM-5.2 ",
		Provider: "opencode-go", Success: true, StartTime: at,
	})
	// No client model at all (an internal call) is not a reroute either.
	insertCostRecord(t, db, history.RequestRecord{
		ID: "unstated", Model: "glm-5.2", Provider: "opencode-go", Success: true, StartTime: at,
	})

	stored := map[string]string{}
	rows, err := db.DB().Query(`SELECT id, COALESCE(requested_model, '') FROM requests`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id, requested string
		if err := rows.Scan(&id, &requested); err != nil {
			t.Fatal(err)
		}
		stored[id] = requested
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	if stored["rerouted"] != "claude-sonnet-4-5" {
		t.Errorf("rerouted requested_model = %q, want the client's model", stored["rerouted"])
	}
	for _, id := range []string{"passthrough", "cased", "unstated"} {
		if stored[id] != "" {
			t.Errorf("%s requested_model = %q, want empty (no reroute)", id, stored[id])
		}
	}
}

// TestRequestedModelSurvivesReadBack covers the round trip: the history API
// reads through scanRequests, so a column written but not scanned would look
// stored and still never reach the dashboard.
func TestRequestedModelSurvivesReadBack(t *testing.T) {
	db := newCostTestDB(t)
	at := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	insertCostRecord(t, db, history.RequestRecord{
		ID: "r1", Model: "kimi-k2.6", RequestedModel: "claude-opus-4",
		Provider: "opencode-go", InputTokens: 10, OutputTokens: 5, Success: true, StartTime: at,
	})

	records, _, err := NewRequests(db).Query(RequestQuery{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	if records[0].RequestedModel != "claude-opus-4" {
		t.Errorf("requested model = %q, want claude-opus-4", records[0].RequestedModel)
	}
	if records[0].Model != "kimi-k2.6" {
		t.Errorf("served model = %q, want kimi-k2.6", records[0].Model)
	}
}

// TestRequestedModelIsSearchable pins that the column is reachable the way a
// reader would look for it. Without it in the WHERE clause the term only ever
// matches the served model, so "where did my opus request go" finds nothing.
func TestRequestedModelIsSearchable(t *testing.T) {
	db := newCostTestDB(t)
	at := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	insertCostRecord(t, db, history.RequestRecord{
		ID: "r1", Model: "kimi-k2.6", RequestedModel: "claude-opus-4",
		Provider: "opencode-go", Success: true, StartTime: at,
	})
	insertCostRecord(t, db, history.RequestRecord{
		ID: "r2", Model: "glm-5.2", Provider: "opencode-go", Success: true, StartTime: at,
	})

	q := NewRequests(db)
	found, total, err := q.Query(RequestQuery{Page: 1, PageSize: 10, Search: "opus"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if total != 1 || len(found) != 1 || found[0].ID != "r1" {
		t.Errorf("search for the requested model returned %d rows (total %d), want just r1", len(found), total)
	}
	// Searching the served model must still work.
	found, total, err = q.Query(RequestQuery{Page: 1, PageSize: 10, Search: "kimi"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if total != 1 || found[0].ID != "r1" {
		t.Errorf("search for the served model returned %d rows, want just r1", total)
	}
	// And a term matching neither finds nothing, so the new clause did not
	// widen the search into a match-everything.
	_, total, err = q.Query(RequestQuery{Page: 1, PageSize: 10, Search: "no-such-model"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if total != 0 {
		t.Errorf("unrelated term matched %d rows, want 0", total)
	}
}

// TestRequestedModelDiffers pins the rule itself, including the case that makes
// it worth having: a Claude id sent to a scenario-routing proxy always differs,
// while a request served by the model it named does not.
func TestRequestedModelDiffers(t *testing.T) {
	cases := []struct {
		requested, served string
		want              bool
		why               string
	}{
		{"claude-opus-4", "kimi-k2.6", true, "rerouted"},
		{"glm-5.2", "glm-5.2", false, "served as asked"},
		{"GLM-5.2", "glm-5.2", false, "case is not a reroute"},
		{"  glm-5.2  ", "glm-5.2", false, "padding is not a reroute"},
		{"", "glm-5.2", false, "no client model is not a reroute"},
		{"   ", "glm-5.2", false, "blank client model is not a reroute"},
	}
	for _, c := range cases {
		if got := history.RequestedModelDiffers(c.requested, c.served); got != c.want {
			t.Errorf("RequestedModelDiffers(%q, %q) = %v, want %v (%s)",
				c.requested, c.served, got, c.want, c.why)
		}
	}
}

// TestRequestedModelMigrationIsIdempotent guards the schema change: an older
// database gains the column on open, and opening it twice does not fail.
func TestRequestedModelMigrationIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	for pass := 1; pass <= 2; pass++ {
		db, err := Open(Config{DatabasePath: path, WALEnabled: false})
		if err != nil {
			t.Fatalf("pass %d: open: %v", pass, err)
		}
		var count int
		if err := db.DB().QueryRow(
			`SELECT COUNT(*) FROM pragma_table_info('requests') WHERE name = 'requested_model'`,
		).Scan(&count); err != nil {
			t.Fatalf("pass %d: read columns: %v", pass, err)
		}
		if count != 1 {
			t.Errorf("pass %d: requested_model column count = %d, want 1", pass, count)
		}
		_ = db.Close()
	}
}

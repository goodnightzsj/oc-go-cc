package gui

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The driver's text names tables and columns. Answering with it told whoever
// reached the dashboard what the schema looks like, and told the operator
// nothing they could act on without the journal.
func TestWriteInternalErrorKeepsTheDetailOut(t *testing.T) {
	rec := httptest.NewRecorder()
	writeInternalError(rec, "history query failed", errors.New("SQL logic error: no such table: requests"))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "no such table") || strings.Contains(body, "SQL logic error") {
		t.Fatalf("response echoed the driver: %q", body)
	}
	if !strings.Contains(body, "history query failed") {
		t.Fatalf("response = %q, want the operator-facing sentence", body)
	}
}

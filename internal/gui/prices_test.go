package gui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPricesIncludesStaticReferenceTable(t *testing.T) {
	w := httptest.NewRecorder()
	(&Server{}).handlePrices(w, httptest.NewRequest(http.MethodGet, "/api/prices", nil))
	var result struct {
		Tables map[string]struct {
			Rules         int  `json:"rules"`
			Live          bool `json:"live"`
			ReferenceOnly bool `json:"reference_only"`
		} `json:"tables"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	state, ok := result.Tables["workbuddy"]
	if w.Code != http.StatusOK || !ok || state.Rules == 0 || state.Live || !state.ReferenceOnly {
		t.Fatalf("static reference price response: status=%d body=%s", w.Code, w.Body.String())
	}
}

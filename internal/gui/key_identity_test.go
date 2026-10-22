package gui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestKeyPoolDisplayKeepsStorageIdentity(t *testing.T) {
	t.Setenv("IDENTITY_A", "synthetic-aaaa")
	t.Setenv("IDENTITY_DUP", "synthetic-aaaa")
	t.Setenv("IDENTITY_B", "synthetic-bbbb")
	for _, raw := range []string{
		`{"cline_pass":{"api_keys":["synthetic-aaaa","synthetic-aaaa","synthetic-bbbb"]}}`,
		`{"cline_pass":{"api_keys":["${IDENTITY_A}","${IDENTITY_DUP}","${IDENTITY_B}"]}}`,
	} {
		srv, path := configTestServer(t, raw)
		rec := httptest.NewRecorder()
		srv.handleProxyConfig(rec, httptest.NewRequest(http.MethodGet, "/api/proxy/config", nil))
		var view struct {
			ClinePass struct {
				Keys []string `json:"api_keys"`
			} `json:"cline_pass"`
			Indices map[string][]int `json:"key_indices"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(view.Indices["cline_pass"], []int{0, 2}) {
			t.Fatal("display lost raw slot identity")
		}
		patch, _ := json.Marshal(map[string]any{"cline_pass": map[string]any{"key_pool_edit": true, "api_keys": []any{
			map[string]any{"keep": view.Indices["cline_pass"][1], "hint": view.ClinePass.Keys[1]},
			map[string]any{"keep": view.Indices["cline_pass"][0], "hint": view.ClinePass.Keys[0]},
			map[string]any{"value": "synthetic-cccc"},
		}}})
		rec = httptest.NewRecorder()
		srv.handleProxyConfig(rec, httptest.NewRequest(http.MethodPost, "/api/proxy/config", strings.NewReader(string(patch))))
		if rec.Code != http.StatusNoContent {
			t.Fatalf("save: %d %s", rec.Code, rec.Body)
		}
		if !reflect.DeepEqual(srv.atomicCfg.Get().ClinePass.EffectiveAPIKeys(), []string{"synthetic-bbbb", "synthetic-aaaa", "synthetic-cccc"}) {
			t.Fatal("wrong reordered pool")
		}
		saved, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(raw, "${") && (!strings.Contains(string(saved), "${IDENTITY_B}") || strings.Contains(string(saved), "synthetic-bbbb")) {
			t.Fatal("placeholder identity lost")
		}
		if strings.Contains(raw, "${IDENTITY_DUP}") && !strings.Contains(string(saved), "${IDENTITY_DUP}") {
			t.Fatal("keeping a deduplicated row discarded its hidden placeholder")
		}
		// The old row 2 now points to another key. Never guess from the hint.
		rec = httptest.NewRecorder()
		srv.handleProxyConfig(rec, httptest.NewRequest(http.MethodPost, "/api/proxy/config", strings.NewReader(string(patch))))
		if rec.Code != http.StatusBadRequest {
			t.Fatal("stale row accepted")
		}
	}
}

func TestKeyPoolRowEditsPreserveOverriddenFields(t *testing.T) {
	for _, suffix := range []string{"API_KEY", "API_KEYS"} {
		t.Run(suffix, func(t *testing.T) {
			t.Setenv("ROUTATIC_PROXY_CLINE_PASS_"+suffix, "synthetic-env")
			srv, path := configTestServer(t, `{"cline_pass":{"api_key":"synthetic-single","api_keys":["synthetic-array"]}}`)
			patch := `{"cline_pass":{"key_pool_edit":true,"api_keys":[{"value":"synthetic-new"}]}}`
			rec := httptest.NewRecorder()
			srv.handleProxyConfig(rec, httptest.NewRequest(http.MethodPost, "/api/proxy/config", strings.NewReader(patch)))
			if rec.Code != http.StatusNoContent {
				t.Fatalf("save: %d %s", rec.Code, rec.Body)
			}
			want := []string{"synthetic-env", "synthetic-new"}
			if suffix == "API_KEYS" {
				want = []string{"synthetic-new", "synthetic-env"}
			}
			if !reflect.DeepEqual(srv.atomicCfg.Get().ClinePass.EffectiveAPIKeys(), want) {
				t.Fatal("wrong edited pool")
			}
			saved, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(saved), "synthetic-env") {
				t.Fatal("environment key persisted")
			}
			if suffix == "API_KEYS" && !strings.Contains(string(saved), "synthetic-array") {
				t.Fatal("shadowed array lost")
			}
			if suffix == "API_KEY" && !strings.Contains(string(saved), "synthetic-single") {
				t.Fatal("shadowed single lost")
			}
		})
	}
}

func TestKeyPoolSingleSlotAndRejectedEditAreAtomic(t *testing.T) {
	t.Setenv("ROUTATIC_PROXY_CLINE_PASS_API_KEYS", "synthetic-env")
	t.Setenv("IDENTITY_SINGLE", "synthetic-single")
	const raw = `{"cline_pass":{"api_key":"${IDENTITY_SINGLE}","api_keys":["synthetic-shadowed"]}}`
	srv, path := configTestServer(t, raw)
	indices := srv.atomicCfg.Get().KeyPoolIndices()["cline_pass"]
	if len(indices) != 2 || indices[0] == nil || *indices[0] != -1 || indices[1] != nil {
		t.Fatal("wrong single/env indices")
	}
	keep := `{"cline_pass":{"key_pool_edit":true,"api_keys":[{"keep":-1,"hint":"` + maskKeyHint("synthetic-single") + `"}]}}`
	post := func(patch string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		srv.handleProxyConfig(rec, httptest.NewRequest(http.MethodPost, "/api/proxy/config", strings.NewReader(patch)))
		return rec
	}
	if rec := post(keep); rec.Code != http.StatusNoContent {
		t.Fatalf("keep: %d %s", rec.Code, rec.Body)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(before), "${IDENTITY_SINGLE}") {
		t.Fatal("kept single lost its placeholder")
	}
	live := srv.atomicCfg.Get()
	for _, patch := range []string{
		`{"cline_pass":{"key_pool_edit":true,"api_keys":[{"value":"synthetic-one"},{"value":"synthetic-two"}]}}`,
		`{"cline_pass":{"key_pool_edit":false,"api_keys":[]}}`,
	} {
		if rec := post(patch); rec.Code != http.StatusBadRequest {
			t.Fatalf("invalid edit: %d %s", rec.Code, rec.Body)
		}
		after, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(before) != string(after) || live != srv.atomicCfg.Get() {
			t.Fatal("rejected edit changed disk or runtime")
		}
	}
	if rec := post(`{"cline_pass":{"key_pool_edit":true,"api_keys":[]}}`); rec.Code != http.StatusNoContent {
		t.Fatalf("clear: %d %s", rec.Code, rec.Body)
	}
	if !reflect.DeepEqual(srv.atomicCfg.Get().ClinePass.EffectiveAPIKeys(), []string{"synthetic-env"}) {
		t.Fatal("clear removed environment key or retained single")
	}
}

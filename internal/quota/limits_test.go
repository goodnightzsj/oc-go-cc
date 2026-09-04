package quota

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseModelLimits(t *testing.T) {
	body := []byte(`<table><tr><th>模型</th><th>输入</th><th>使用额度</th></tr>
<tr><td>GLM-5.2</td><td>$1.40</td><td>$60</td></tr>
<tr><td>Grok 4.6 (≤ 200K tokens)</td><td>$2.00</td><td>$15</td></tr>
<tr><td>Kimi &amp; K2</td><td>$0.95</td><td>$0.50</td></tr>
<tr><td>No Allowance</td><td>$1.00</td><td>-</td></tr>
<tr><td>Empty</td><td>$1.00</td><td></td></tr></table>`)
	models, err := ParseModelLimits(body)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(models) != 3 {
		t.Fatalf("models = %d, want 3 (dash/empty rows skipped)", len(models))
	}
	if m := models[0]; m.Model != "GLM-5.2" || m.AllowanceUSD != 60 {
		t.Errorf("models[0] = %+v, want GLM-5.2 $60", m)
	}
	if m := models[1]; m.Model != "Grok 4.6" || m.AllowanceUSD != 15 {
		t.Errorf("models[1] = %+v, want variant suffix stripped, $15", m)
	}
	if m := models[2]; m.Model != "Kimi & K2" || m.AllowanceUSD != 0.5 {
		t.Errorf("models[2] = %+v, want unescaped name + $0.50", m)
	}
}

// TestParseModelLimitsMergesVariants covers pricing variants of one model
// collapsing into a single row under the base name (console lists one row per
// model), keeping the largest allowance.
func TestParseModelLimitsMergesVariants(t *testing.T) {
	body := []byte(`<table><tr><th>Model</th><th>Input</th><th>Usage</th></tr>
<tr><td>DeepSeek V4 Flash (Off-Peak)</td><td>$0.22</td><td>$30</td></tr>
<tr><td>DeepSeek V4 Flash (Peak)</td><td>$0.44</td><td>$30</td></tr>
<tr><td>Grok 4.6 (≤ 200K tokens)</td><td>$2.00</td><td>$15</td></tr>
<tr><td>Omen Alpha</td><td>$0.20</td><td>$100</td></tr></table>`)
	models, err := ParseModelLimits(body)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(models) != 3 {
		t.Fatalf("models = %d, want 3 after merging DeepSeek variants", len(models))
	}
	if m := models[0]; m.Model != "DeepSeek V4 Flash" || m.AllowanceUSD != 30 {
		t.Errorf("merged row = %+v, want DeepSeek V4 Flash $30", m)
	}
	if m := models[2]; m.Model != "Omen Alpha" || m.AllowanceUSD != 100 {
		t.Errorf("models[2] = %+v, want Omen Alpha $100", m)
	}
}

// TestParseModelLimitsEnglishHeader covers the en docs layout, whose last
// column is "Usage" rather than "Allowance"/额度.
func TestParseModelLimitsEnglishHeader(t *testing.T) {
	body := []byte(`<table><tr><th>Model</th><th>Input</th><th>Output</th><th>Cached Read</th><th>Cached Write</th><th>Usage</th></tr>
<tr><td>Omen Alpha</td><td>$0.20</td><td>$0.66</td><td>$0.04</td><td>-</td><td>$100</td></tr>
<tr><td>DeepSeek V4 Flash</td><td>$0.22</td><td>$0.66</td><td>$0.007</td><td>-</td><td>$30</td></tr></table>`)
	models, err := ParseModelLimits(body)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(models) != 2 || models[0].AllowanceUSD != 100 || models[1].AllowanceUSD != 30 {
		t.Fatalf("models = %+v, want 2 rows ($100, $30)", models)
	}
}

func TestParseModelLimitsRejectsUnstructuredHTML(t *testing.T) {
	if _, err := ParseModelLimits([]byte("<html><p>no tables here</p></html>")); err == nil {
		t.Error("expected an error for HTML without a table")
	}
	if _, err := ParseModelLimits([]byte(`<table><tr><th>Name</th></tr><tr><td>x</td></tr></table>`)); err == nil {
		t.Error("expected an error for a table without an allowance column")
	}
}

func TestFetchModelLimitsSkipsUnparsableURLs(t *testing.T) {
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path)
		switch r.URL.Path {
		case "/bad":
			_, _ = w.Write([]byte("<html>layout changed</html>"))
		default:
			_, _ = w.Write([]byte(`<table><tr><th>模型</th><th>使用额度</th></tr><tr><td>GLM</td><td>$60</td></tr></table>`))
		}
	}))
	defer srv.Close()

	lim, err := FetchModelLimits(context.Background(), srv.Client(), srv.URL+"/bad", srv.URL+"/good")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(lim.Models) != 1 || lim.Models[0].AllowanceUSD != 60 {
		t.Fatalf("models = %+v", lim.Models)
	}
	if lim.URL != srv.URL+"/good" {
		t.Errorf("url = %q, want the fallback page", lim.URL)
	}
	if len(calls) != 2 {
		t.Errorf("fetched %d urls, want 2 (first fails, second succeeds)", len(calls))
	}
}

package quota

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseModelLimits(t *testing.T) {
	body := []byte(`<table><tr><th>模型</th><th>输入</th><th>每月限制</th></tr>
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
// column is "Monthly limit" - not "Usage", which an earlier revision of this
// fixture assumed.
func TestParseModelLimitsEnglishHeader(t *testing.T) {
	body := []byte(`<table><tr><th>Model</th><th>Input</th><th>Output</th><th>Cached Read</th><th>Cached Write</th><th>Monthly limit</th></tr>
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

// TestParseModelLimitsAcceptsTheLiveHeaders is the regression guard for a real
// defect: the zh page renamed its column from 使用额度 to 每月限制, the parser
// matched 额度 only, and every zh parse failed silently into the en fallback.
// The output stayed correct, so nothing surfaced it - the fetch simply stopped
// reading the page it names as primary.
//
// Both headers here are transcribed from the live pages rather than composed,
// because a hand-written fixture only ever states what the author expected.
func TestParseModelLimitsAcceptsTheLiveHeaders(t *testing.T) {
	cases := []struct {
		name   string
		header []string
	}{
		{"zh", []string{"模型", "输入", "输出", "缓存读取", "缓存写入", "每月限制"}},
		{"en", []string{"Model", "Input", "Output", "Cached Read", "Cached Write", "Monthly limit"}},
		// The spelling this page used before the rename, kept so an older
		// mirror or a cached page still parses.
		{"zh-legacy", []string{"模型", "输入", "输出", "缓存读取", "缓存写入", "使用额度"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			header := "<tr>" + strings.Join(mapCells("th", tc.header), "") + "</tr>"
			row := "<tr>" + strings.Join(mapCells("td",
				[]string{"DeepSeek V4 Flash", "$0.22", "$0.66", "$0.007", "-", "$30"}), "") + "</tr>"
			models, err := ParseModelLimits([]byte("<table>" + header + row + "</table>"))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if len(models) != 1 || models[0].AllowanceUSD != 30 {
				t.Fatalf("models = %+v, want one row at $30", models)
			}
		})
	}
}

// TestParseModelLimitsSkipsAPromotedRow. The live page renders a promotion into
// the same cell ("$15 $604x · 9 月 27 日结束"), which is two amounts and a
// multiplier in one field. Reading it leniently would take whichever number the
// parser reached first and publish it as the allowance. Skipping the row costs
// that model a breakdown; misreading it would put a wrong figure next to a real
// spend number.
func TestParseModelLimitsSkipsAPromotedRow(t *testing.T) {
	body := []byte(`<table><tr><th>模型</th><th>输入</th><th>每月限制</th></tr>
<tr><td>DeepSeek V4.1 Flash (Peak)</td><td>$0.30</td><td>$15 $604x · 9 月 27 日结束</td></tr>
<tr><td>DeepSeek V4 Pro</td><td>$1.32</td><td>$15</td></tr></table>`)
	models, err := ParseModelLimits(body)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(models) != 1 || models[0].Model != "DeepSeek V4 Pro" {
		t.Fatalf("models = %+v, want only the parseable row", models)
	}
}

func mapCells(tag string, values []string) []string {
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = "<" + tag + ">" + v + "</" + tag + ">"
	}
	return out
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

// TestServedButUnparsablePageIsWarnedAbout is the guard for how the zh page
// silently stopped being read: it renamed its allowance column, every zh parse
// failed, the en page answered instead, and because the fallback is designed to
// absorb exactly that, nothing anywhere reported it. The output was correct and
// the only symptom was which URL the snapshot came from.
//
// The warning is the whole detection path, so it is asserted here rather than
// left to review. Note the distinction being pinned: a page that is *served*
// but unreadable warns, while a page that cannot be *fetched* does not - the
// second is what the fallback exists for.
func TestServedButUnparsablePageIsWarnedAbout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/unreadable":
			// Served fine; no allowance table in the body.
			_, _ = w.Write([]byte("<html><body><table><tr><th>Name</th></tr><tr><td>x</td></tr></table></body></html>"))
		case "/missing":
			http.NotFound(w, r)
		default:
			_, _ = w.Write([]byte(`<table><tr><th>模型</th><th>每月限制</th></tr><tr><td>GLM</td><td>$60</td></tr></table>`))
		}
	}))
	defer srv.Close()

	capture := func(urls ...string) (string, error) {
		var buf strings.Builder
		old := slog.Default()
		h := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})
		slog.SetDefault(slog.New(h))
		defer slog.SetDefault(old)
		_, err := FetchModelLimits(context.Background(), srv.Client(), urls...)
		return buf.String(), err
	}

	// Served but unreadable: warns, and names the page so the culprit is known.
	logs, err := capture(srv.URL+"/unreadable", srv.URL+"/good")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if !strings.Contains(logs, "could not be read") {
		t.Errorf("no warning for a served-but-unreadable page; logs = %q", logs)
	}
	if !strings.Contains(logs, "/unreadable") {
		t.Errorf("warning does not name the page that failed; logs = %q", logs)
	}

	// Not fetchable: the fallback's intended case, so no warning.
	logs, err = capture(srv.URL+"/missing", srv.URL+"/good")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if strings.Contains(logs, "could not be read") {
		t.Errorf("a 404 must not warn as a layout change; logs = %q", logs)
	}

	// A parse failure on every source still returns the last error, so a caller
	// can report something rather than being handed a silent nil.
	if _, err := capture(srv.URL + "/unreadable"); err == nil {
		t.Error("all sources unreadable must return an error, not a nil snapshot")
	}
}

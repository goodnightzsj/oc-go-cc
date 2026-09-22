package quota

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Docs pages that publish the per-model monthly usage allowance table. The
// zh page is primary; the en page is the fallback if the zh layout changes.
// The base plan limits ($12 / $30 / $60) are constant, but each model carries
// its own allowance multiplier (e.g. $15 for Grok 4.6, $100 for Omen Alpha),
// so the table is refreshed daily rather than hard-coded.
const (
	DocsURL         = "https://opencode.ai/docs/zh-cn/go"
	DocsFallbackURL = "https://opencode.ai/docs/go"
)

// ModelLimit is the monthly usage allowance a Go plan key may spend on one
// model, straight from the docs table.
type ModelLimit struct {
	Model        string  `json:"model"`
	AllowanceUSD float64 `json:"allowance_usd"`
}

// ModelLimits is one snapshot of the per-model allowance table.
type ModelLimits struct {
	URL       string       `json:"url"`
	FetchedAt time.Time    `json:"fetched_at"`
	Models    []ModelLimit `json:"models"`
}

// FetchModelLimits pulls the per-model allowance table from the Go docs and
// returns the parsed snapshot. With no URLs given, the zh page is tried first
// and the en page as fallback; the first page that parses to at least one
// model wins.
func FetchModelLimits(ctx context.Context, client *http.Client, urls ...string) (*ModelLimits, error) {
	if len(urls) == 0 {
		urls = []string{DocsURL, DocsFallbackURL}
	}
	var lastErr error
	for _, url := range urls {
		lim, err := fetchModelLimitsFrom(ctx, client, url)
		if err == nil {
			return lim, nil
		}
		// A page that was served but not understood is a different event from
		// one that could not be fetched, and only the first is worth a warning.
		// A fetch failure is what the fallback exists for. A parse failure means
		// the fallback is hiding a layout change - which is exactly how the zh
		// page stopped being read: it renamed its allowance column, every zh
		// parse failed, the en page answered instead, and the output stayed
		// correct and identical-looking, so nothing ever reported it.
		//
		// This warning is the whole guard. It is logged rather than surfaced
		// because there is nothing for a caller to do about it: the remaining
		// sources are still tried, and the result is either right or absent.
		var unparsable *unparsablePageError
		if errors.As(err, &unparsable) {
			slog.Warn("docs page was served but its allowance table could not be read; trying the next source",
				"url", url, "error", unparsable.err)
		}
		lastErr = err
	}
	return nil, lastErr
}

// unparsablePageError marks a page that answered successfully but whose
// allowance table could not be extracted from the body. It exists so the
// caller can tell that apart from a transport or status failure; see the
// warning in FetchModelLimits for why the distinction matters.
type unparsablePageError struct {
	url string
	err error
}

func (e *unparsablePageError) Error() string { return fmt.Sprintf("docs %s: %v", e.url, e.err) }
func (e *unparsablePageError) Unwrap() error { return e.err }

func fetchModelLimitsFrom(ctx context.Context, client *http.Client, url string) (*ModelLimits, error) {
	// Fall back to a default client rather than dereferencing nil. Every caller
	// passes one today; this is here so the next one cannot panic, matching
	// history's fetchers.
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("docs %s: HTTP %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	models, err := ParseModelLimits(body)
	if err != nil {
		// The body arrived; only the table could not be read out of it.
		return nil, &unparsablePageError{url: url, err: err}
	}
	return &ModelLimits{URL: url, FetchedAt: time.Now().UTC(), Models: models}, nil
}

var (
	htmlTableRE = regexp.MustCompile(`(?s)<table[^>]*>(.*?)</table>`)
	htmlRowRE   = regexp.MustCompile(`(?s)<tr[^>]*>(.*?)</tr>`)
	htmlCellRE  = regexp.MustCompile(`(?s)<t[dh][^>]*>(.*?)</t[dh]>`)
)

// ParseModelLimits extracts the per-model allowance table from a Go docs page.
// The table is located by a header cell containing a limit keyword (限制 /
// 额度 / Allowance / Limit / Usage); the model name is the first column, the
// allowance is the matching cell of each row, formatted like "$60" (a "-" or
// missing value skips the row).
//
// This table is a cross-check, not the source of truth for what a plan has
// left. The account's own 5-hour / weekly / monthly usage is - see the quota
// handler - and it is what the dashboard treats as authoritative. The table
// only supplies the per-model share used to break that total down, so a row
// this parser cannot read costs a breakdown, never a spend figure.
//
// The Chinese header is 每月限制 (Monthly limit). An earlier revision matched
// 额度 ("allowance") only, which the page stopped using, so every zh parse
// failed silently and the fetch fell through to the English page - correct
// output, wrong page, and invisible because the fallback exists for a genuine
// layout change. Both spellings are matched now. A promotion can also render
// into this cell ("$15 $604x · ..."), in which case the row is skipped rather
// than misread: leniency here would round a multiplier up into an allowance.
func ParseModelLimits(body []byte) ([]ModelLimit, error) {
	for _, table := range htmlTableRE.FindAllStringSubmatch(string(body), -1) {
		rows := htmlRowRE.FindAllStringSubmatch(table[1], -1)
		if len(rows) < 2 {
			continue
		}
		header := cellTexts(rows[0][1])
		allowIdx := -1
		for i, cell := range header {
			if strings.Contains(strings.ToLower(cell), "allowance") ||
				strings.Contains(cell, "限制") ||
				strings.Contains(cell, "额度") ||
				strings.Contains(strings.ToLower(cell), "usage") ||
				strings.Contains(strings.ToLower(cell), "limit") {
				allowIdx = i
			}
		}
		if allowIdx < 0 {
			continue
		}
		var models []ModelLimit
		for _, row := range rows[1:] {
			cells := cellTexts(row[1])
			if len(cells) <= allowIdx {
				continue
			}
			allow, err := parseDollars(cells[allowIdx])
			if err != nil || allow == 0 {
				continue
			}
			models = append(models, ModelLimit{Model: cells[0], AllowanceUSD: allow})
		}
		if len(models) > 0 {
			return mergeVariants(models), nil
		}
	}
	return nil, fmt.Errorf("no usage-allowance table found")
}

// mergeVariants collapses pricing variants of one model ("DeepSeek V4 Flash
// (Peak)" / "(Off-Peak)", "Grok 4.6 (≤ 200K tokens)" / "(> 200K tokens)")
// into a single row under the base name, keeping the largest allowance — the
// console lists one row per model, and a variant never lowers the plan.
func mergeVariants(models []ModelLimit) []ModelLimit {
	merged := make([]ModelLimit, 0, len(models))
	idx := make(map[string]int, len(models))
	for _, m := range models {
		base := baseModelName(m.Model)
		if i, ok := idx[base]; ok {
			if m.AllowanceUSD > merged[i].AllowanceUSD {
				merged[i].AllowanceUSD = m.AllowanceUSD
			}
			continue
		}
		idx[base] = len(merged)
		merged = append(merged, ModelLimit{Model: base, AllowanceUSD: m.AllowanceUSD})
	}
	return merged
}

// baseModelName strips the parenthetical pricing-variant suffix, e.g.
// "DeepSeek V4 Flash (Off-Peak)" → "DeepSeek V4 Flash".
func baseModelName(name string) string {
	if i := strings.IndexByte(name, '('); i >= 0 {
		name = name[:i]
	}
	return strings.TrimSpace(name)
}

func cellTexts(row string) []string {
	cells := htmlCellRE.FindAllStringSubmatch(row, -1)
	out := make([]string, 0, len(cells))
	for _, c := range cells {
		text := html.UnescapeString(regexp.MustCompile(`<[^>]+>`).ReplaceAllString(c[1], ""))
		out = append(out, strings.TrimSpace(text))
	}
	return out
}

func parseDollars(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "-" {
		return 0, fmt.Errorf("no allowance")
	}
	return strconv.ParseFloat(strings.TrimPrefix(s, "$"), 64)
}

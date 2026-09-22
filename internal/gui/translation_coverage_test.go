package gui

import (
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestEveryTranslationKeyIsDefined guards a defect that renders as raw text in
// both languages and is invisible to every existing test: a `data-i18n` key used
// in the markup but never defined resolves to its own name, so the page shows
// "clinepass.quotaHint" where a sentence belongs.
//
// It shipped that way for seven ClinePass keys across the Quota and Settings
// tabs. Nothing caught it because the markup is valid, the lookup returns a
// string, and only a human reading the rendered page would notice.
func TestEveryTranslationKeyIsDefined(t *testing.T) {
	app, err := assets.ReadFile("assets/app.js")
	if err != nil {
		t.Fatal(err)
	}
	page, err := assets.ReadFile("assets/index.html")
	if err != nil {
		t.Fatal(err)
	}

	// Keys defined in the EN table. The two language tables are checked for
	// parity separately, so reading one is enough to decide "defined".
	defined := translationKeys(t, string(app), "en")

	// Every key referenced from the markup, in any of the data-i18n forms.
	used := map[string]bool{}
	for _, match := range regexp.MustCompile(`data-i18n(?:-placeholder|-option|-aria-label)?="([^"]+)"`).
		FindAllStringSubmatch(string(page), -1) {
		used[match[1]] = true
	}
	// Keys referenced from script-generated markup are just as real: a row built
	// in JS carries the same attribute and leaks the same way.
	//
	// A call whose argument continues with `+` is building a key at runtime
	// (`t('quota.status.' + status)`), so what the pattern captures is a prefix
	// and not a key. Those are checked by the parity test instead, which sees
	// the full set of defined keys.
	for _, match := range regexp.MustCompile(`\bt\('([a-zA-Z]+\.[a-zA-Z0-9_.]+)'\)`).
		FindAllStringSubmatch(string(app), -1) {
		used[match[1]] = true
	}

	var missing []string
	for key := range used {
		if !defined[key] {
			missing = append(missing, key)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("these keys are used but defined in no translation table, so they render as their own name:\n  %s",
			strings.Join(missing, "\n  "))
	}
}

// TestNoTranslationKeyIsDefinedTwice guards a silent-failure mode that no other
// check here can see: two entries with the same key in one table. The later
// entry wins at runtime, so the earlier one is dead - and because both keys are
// "defined", the missing-key and parity tests both pass.
//
// It shipped that way. `analytics.throughput` meant "Last-minute throughput" (a
// card on the Overview) and "Tok/s" (a column header on Usage Analytics); the
// second definition silently overrode the first, so the Overview card was
// labelled "Tok/s" above a value reading "0 RPM" in both languages.
func TestNoTranslationKeyIsDefinedTwice(t *testing.T) {
	app, err := assets.ReadFile("assets/app.js")
	if err != nil {
		t.Fatal(err)
	}
	source := string(app)

	for _, lang := range []string{"en", "zh"} {
		body := translationBlock(t, source, lang)
		seen := map[string]bool{}
		var dups []string
		for _, match := range regexp.MustCompile(`(?m)^\s*'([a-zA-Z][a-zA-Z0-9_.]*)':`).
			FindAllStringSubmatch(body, -1) {
			if seen[match[1]] {
				dups = append(dups, match[1])
			}
			seen[match[1]] = true
		}
		sort.Strings(dups)
		if len(dups) > 0 {
			t.Errorf("the %s table defines these keys more than once, so the earlier value is dead:\n  %s",
				lang, strings.Join(dups, "\n  "))
		}
	}
}

// TestTranslationTablesAreInParity pins that every key exists in both languages.
// A key present only in English falls back to the key name for a Chinese reader,
// which is the same visible failure as an undefined key but harder to spot in
// review because the English page looks correct.
func TestTranslationTablesAreInParity(t *testing.T) {
	app, err := assets.ReadFile("assets/app.js")
	if err != nil {
		t.Fatal(err)
	}
	source := string(app)
	en := translationKeys(t, source, "en")
	zh := translationKeys(t, source, "zh")

	var onlyEN, onlyZH []string
	for key := range en {
		if !zh[key] {
			onlyEN = append(onlyEN, key)
		}
	}
	for key := range zh {
		if !en[key] {
			onlyZH = append(onlyZH, key)
		}
	}
	sort.Strings(onlyEN)
	sort.Strings(onlyZH)
	if len(onlyEN) > 0 {
		t.Errorf("defined in English but missing from Chinese (renders as the key name):\n  %s",
			strings.Join(onlyEN, "\n  "))
	}
	if len(onlyZH) > 0 {
		t.Errorf("defined in Chinese but missing from English:\n  %s", strings.Join(onlyZH, "\n  "))
	}
}

// translationKeys parses one language block out of the TRANSLATIONS object.
func translationKeys(t *testing.T, source, lang string) map[string]bool {
	t.Helper()
	keys := map[string]bool{}
	for _, match := range regexp.MustCompile(`(?m)^\s*'([a-zA-Z][a-zA-Z0-9_.]*)':`).
		FindAllStringSubmatch(translationBlock(t, source, lang), -1) {
		keys[match[1]] = true
	}
	if len(keys) == 0 {
		t.Fatalf("parsed 0 keys from the %s table, so the assertion would be vacuous", lang)
	}
	return keys
}

// translationBlock returns the raw text of one language block.
//
// The tables are found by their opening marker rather than by brace counting,
// because the values contain braces ("{n} days") that would unbalance a naive
// scan. Each block ends at the first line that closes it at the same indent.
func translationBlock(t *testing.T, source, lang string) string {
	t.Helper()
	marker := "\n  en: {"
	if lang == "zh" {
		marker = "\n  zh: {"
	}
	at := strings.Index(source, marker)
	if at < 0 {
		t.Fatalf("could not locate the %s translation table", lang)
	}
	rest := source[at+len(marker):]
	end := strings.Index(rest, "\n  },")
	if end < 0 {
		t.Fatalf("could not locate the end of the %s translation table", lang)
	}
	block := rest[:end]
	if !strings.Contains(block, ":") {
		t.Fatalf("the %s block looks empty, so the assertion would be vacuous", lang)
	}
	return block
}

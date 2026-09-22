package history

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// weekdayPeak is a Monday 02:00 UTC instant: weekday, inside the 01-04 window.
// Kept beside the holiday tests because the whole point of the exemption is
// that this instant stops billing at peak on a holiday.
var weekdayPeak = time.Date(2026, 2, 16, 2, 0, 0, 0, time.UTC)

// TestHolidayIsOffPeakOnlyWhereThePlatformExemptsIt pins the per-platform
// split. It is the assertion that fails if the exemption is applied to every
// platform (CommandCode would under-bill) or to none (OpenCode Go and ClinePass
// over-bill, which is the defect this table was added to fix).
func TestHolidayIsOffPeakOnlyWhereThePlatformExemptsIt(t *testing.T) {
	// 2026-02-16 is a Monday inside the Spring Festival holiday, per the
	// State Council notice and the embedded seed.
	if !IsChineseHoliday(weekdayPeak) {
		t.Fatal("the embedded calendar does not carry 2026-02-16; the seed or its date range regressed")
	}
	if got := weekdayPeak.Weekday(); got != time.Monday {
		t.Fatalf("fixture day is %v, want Monday", got)
	}

	for _, tc := range []struct {
		provider string
		model    string
		want     float64
	}{
		{"opencode-go", "deepseek-v4-pro", 1},           // inherits DeepSeek's exemption
		{"cline-pass", "cline-pass/deepseek-v4-pro", 1}, // footnotes that page
		{"commandcode", "deepseek/deepseek-v4-pro", 2},  // states no exemption
	} {
		if got := ProviderPeakMultiplier(tc.provider, tc.model, weekdayPeak); got != tc.want {
			t.Errorf("%s on a Chinese public holiday = %v, want %v", tc.provider, got, tc.want)
		}
	}

	// A non-holiday Monday in the same window must still bill at peak, so the
	// exemption is not simply switching peak off for these platforms.
	ordinary := time.Date(2026, 3, 2, 2, 0, 0, 0, time.UTC)
	if IsChineseHoliday(ordinary) {
		t.Fatal("2026-03-02 is in the calendar; pick another control day")
	}
	for _, provider := range []string{"opencode-go", "cline-pass", "commandcode"} {
		if got := ProviderPeakMultiplier(provider, "deepseek-v4-pro", ordinary); got != 2 {
			t.Errorf("%s on an ordinary Monday = %v, want 2", provider, got)
		}
	}
}

// TestHolidayExemptsTheWholeDayNotJustTheWindows guards the reading of "All
// other hours are off-peak, including weekends and Chinese public holidays in
// full": a holiday hour outside the peak windows was already off-peak, so an
// implementation that only checked the windows would pass a narrower test.
func TestHolidayExemptsTheWholeDayNotJustTheWindows(t *testing.T) {
	// 12:00 UTC on 2026-02-16 sits between the 01-04 and 06-10 windows.
	between := time.Date(2026, 2, 16, 12, 0, 0, 0, time.UTC)
	if got := ProviderPeakMultiplier("opencode-go", "deepseek-v4-pro", between); got != 1 {
		t.Errorf("off-peak holiday hour = %v, want 1", got)
	}
}

// TestHolidayDatesAreReadInUTC is the off-by-one guard, and it has to sit on a
// holiday boundary to mean anything. The peak windows are 01:00-04:00 and
// 06:00-10:00 UTC, which is 09:00-18:00 Beijing on the same calendar day - so a
// UTC instant must be checked against its own UTC date, and both fixtures below
// are the first or last hours of a holiday *run*, where shifting by Beijing's
// eight hours lands on the wrong side.
//
// 2026-09-25 (Fri) to 09-27 (Sun) is 中秋, with Monday 09-28 a working day.
// 02:00Z on the 28th is inside the peak window; its Beijing time is the 28th at
// 10:00, and its UTC date is the 28th too - but a converter that subtracted
// eight hours instead of shifting the zone makes it the 27th, which is a
// holiday, and the peak charge would silently vanish from the first working
// hours of the week.
func TestHolidayDatesAreReadInUTC(t *testing.T) {
	afterHoliday := time.Date(2026, 9, 28, 2, 0, 0, 0, time.UTC)
	if got := afterHoliday.Weekday(); got != time.Monday {
		t.Fatalf("fixture day is %v, want Monday", got)
	}
	if IsChineseHoliday(afterHoliday) {
		t.Fatal("2026-09-28 is in the calendar; the fixture is meant to be the day after the run")
	}
	if !IsChineseHoliday(time.Date(2026, 9, 27, 2, 0, 0, 0, time.UTC)) {
		t.Fatal("2026-09-27 should be a holiday; the boundary fixture is wrong")
	}
	if got := ProviderPeakMultiplier("opencode-go", "deepseek-v4-pro", afterHoliday); got != 2 {
		t.Errorf("first working hours after a holiday = %v, want 2 (a Beijing-shifted lookup loses this)", got)
	}

	// And the mirror case: 04:00Z on the last holiday day is 12:00 Beijing, still
	// the holiday, and must stay off-peak.
	lastHolidayHours := time.Date(2026, 9, 27, 7, 0, 0, 0, time.UTC)
	if !IsChineseHoliday(lastHolidayHours) {
		t.Error("an instant inside the holiday run must read as a holiday")
	}

	// The windows themselves are same-day in both zones, so the holiday that
	// governs a peak hour is the UTC date's. Asserting that directly: every hour
	// of the three holiday days is off-peak, and every hour of the working day
	// that follows is peak inside its windows.
	for h := 0; h < 24; h++ {
		onHoliday := time.Date(2026, 9, 26, h, 30, 0, 0, time.UTC) // Saturday, but a holiday regardless
		if got := ProviderPeakMultiplier("opencode-go", "deepseek-v4-pro", onHoliday); got != 1 {
			t.Fatalf("holiday hour %02d:30Z = %v, want 1", h, got)
		}
	}
}

// TestEmbeddedSeedCoversTheYearsThatMatter. The seed is what answers when the
// network never does, so a gap in it is a silent over-bill. The upstream year
// file is keyed by the State Council document, so the newest year may hold no
// dates until the notice is published - hence the check is on a year that has
// already been announced, not on "this year".
func TestEmbeddedSeedCoversTheYearsThatMatter(t *testing.T) {
	var file holidayFile
	if err := json.Unmarshal(holidaySeed, &file); err != nil {
		t.Fatalf("embedded seed does not parse: %v", err)
	}
	years := map[string]int{}
	for _, day := range file.Days {
		if !day.IsOffDay {
			t.Errorf("seed carries a non-off day %q; make-up workdays must be dropped at generation", day.Date)
		}
		years[day.Date[:4]]++
	}
	// 2026 was announced 2025-11-04, so its holidays must be present. A year of
	// ~7 holidays plus weekends joined in is the shape to expect.
	if years["2026"] < 25 {
		t.Errorf("seed holds %d days for 2026, want the announced schedule (>=25)", years["2026"])
	}
	// 2007 is the upstream series' start; if that regressed the seed generator
	// is not reading the source it names.
	if years["2007"] == 0 {
		t.Error("seed does not reach back to 2007, the start of the upstream series")
	}
	if ChineseHolidayCount() != len(file.Days) {
		t.Errorf("loaded calendar holds %d days, seed holds %d", ChineseHolidayCount(), len(file.Days))
	}
}

// TestFetchHolidaysReadsBothYearsAndToleratesAnAbsentOne. The following year's
// file does not exist until the government publishes, and demanding it would
// make every refresh fail for most of the year. A year that exists but is
// malformed is a different thing and must be an error.
func TestFetchHolidaysReadsBothYearsAndToleratesAnAbsentOne(t *testing.T) {
	var requested []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = append(requested, r.URL.Path)
		switch {
		case strings.HasSuffix(r.URL.Path, "/2026.json"):
			_, _ = w.Write([]byte(`{"days":[{"name":"x","date":"2026-01-01","isOffDay":true},` +
				`{"name":"x","date":"2026-01-04","isOffDay":false}]}`))
		case strings.HasSuffix(r.URL.Path, "/2027.json"):
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	set, err := fetchHolidays(t, srv.URL, time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("an absent next-year file must not fail the refresh: %v", err)
	}
	if _, ok := set["2026-01-01"]; !ok {
		t.Error("the published year's off day was dropped")
	}
	if _, ok := set["2026-01-04"]; ok {
		t.Error("a make-up workday (isOffDay:false) was installed as a holiday")
	}
	if len(requested) != 2 {
		t.Errorf("requested %v, want both the current and the following year", requested)
	}
}

// fetchHolidays is the test's entry to the fetch, pointed at a local server.
// Only the host changes; the URL pattern is the package's own.
func fetchHolidays(t *testing.T, base string, now time.Time) (map[string]struct{}, error) {
	t.Helper()
	return fetchHolidaysFrom(context.Background(), nil, now, base+"/%d.json")
}

// TestFetchHolidaysRejectsAMalformedPublishedYear. A year the source publishes
// but that cannot be parsed is a change in its shape and must surface, not be
// treated like a year that is not there yet.
func TestFetchHolidaysRejectsAMalformedPublishedYear(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/2026.json") {
			_, _ = w.Write([]byte(`{"days":[{"name":"x","date":"01/01/2026","isOffDay":true}]}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	if _, err := fetchHolidays(t, srv.URL, time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)); err == nil {
		t.Fatal("a published year with an unparseable date must fail, not be skipped silently")
	}
}

// TestFailedRefreshKeepsThePreviousCalendar. Failing open - emptying the
// calendar - would turn every holiday back into a working day and restore the
// over-billing, and it would do it exactly when the network is already
// unhealthy. The guard is that the loaded set is only replaced on success.
func TestFailedRefreshKeepsThePreviousCalendar(t *testing.T) {
	before := ChineseHolidayCount()
	if before == 0 {
		t.Fatal("no calendar loaded to preserve")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	n, err := refreshHolidaysFrom(context.Background(), srv.Client(), time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), srv.URL+"/%d.json")
	if err == nil {
		t.Fatal("a refresh against a failing source must report an error")
	}
	if n != before {
		t.Errorf("calendar holds %d days after a failed refresh, want the previous %d", n, before)
	}
	if !IsChineseHoliday(weekdayPeak) {
		t.Error("a failed refresh dropped the calendar's contents")
	}
}

// TestFetchHolidaysRejectsAnEmptyPayload. An empty `days` array on the *current*
// year is the shape a not-yet-announced file has, but a fetch that installs
// nothing must not be reported as success.
func TestFetchHolidaysRejectsAnEmptyPayload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"days":[]}`))
	}))
	defer srv.Close()

	if _, err := fetchHolidays(t, srv.URL, time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)); err == nil {
		t.Fatal("an empty payload must not be installed as a calendar")
	}
}

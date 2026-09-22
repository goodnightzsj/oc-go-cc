// Chinese public holiday calendar, for the one rule that needs it: DeepSeek's
// peak pricing exempts Chinese public holidays, and two of the platforms this
// proxy resells inherit that window.
//
// The calendar is not published as an API. It is a State Council notice issued
// once a year (the 2026 one is 国办发明电〔2025〕7号) and republished as data by
// third parties. NateScarlet/holiday-cn is the one used here because it states
// what billing needs - a date and whether it is an off day - and cites the
// notice it scraped, so a wrong date can be traced to its source. It is fetched
// daily by its own CI, so the fetch below is of a file that moves when the
// government publishes, not a copy someone has to remember to update.
//
// This file is the only part of the package that talks to the network, and it
// fails closed in both directions: a fetch that fails leaves the previous
// calendar in place, and if none has ever succeeded the embedded seed answers.
// Pricing must never silently degrade to "no holidays known" - that is exactly
// the bug this table exists to fix.
package history

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
	"time"

	_ "embed"
)

// HolidaySourceURL is holiday-cn's master copy, one file per year. The year in
// the name is the State Council document's year, not the year of every date it
// covers - December days can belong to the next document - so the current and
// the following year are both read.
const HolidaySourceURL = "https://raw.githubusercontent.com/NateScarlet/holiday-cn/master/%d.json"

// DefaultHolidayRefreshInterval is daily. The upstream file only changes when
// the State Council publishes, which is once a year for the next year's
// schedule, but the notice is what creates next year's file and there is no
// cheaper way to notice it appearing than to ask.
const DefaultHolidayRefreshInterval = 24 * time.Hour

const (
	holidayFetchTimeout = 20 * time.Second
	maxHolidayBytes     = 1 << 20
)

//go:embed seed_holidays_cn.json
var holidaySeed []byte

// holidayCalendar holds the off days as YYYY-MM-DD keys in UTC. The peak
// windows run 01:00-10:00 UTC, which is 09:00-18:00 in Beijing on the same
// calendar day, so a holiday checked against the request's UTC date is the
// holiday the platform is billing in.
var holidayCalendar atomic.Pointer[map[string]struct{}]

// holidayDay is one row of the upstream file. Only off days are read; the
// make-up workdays it also carries cannot change a peak verdict, because the
// window is stated as Monday through Friday and a make-up day is always a
// weekend day made working - weekday() still reports Saturday or Sunday.
type holidayDay struct {
	Name     string `json:"name"`
	Date     string `json:"date"`
	IsOffDay bool   `json:"isOffDay"`
}

type holidayFile struct {
	Days []holidayDay `json:"days"`
}

// IsChineseHoliday reports whether the given date is a Chinese public holiday.
// The date is taken in UTC, matching the peak windows.
func IsChineseHoliday(t time.Time) bool {
	set := holidayCalendar.Load()
	if set == nil || t.IsZero() {
		return false
	}
	_, ok := (*set)[t.UTC().Format("2006-01-02")]
	return ok
}

// ChineseHolidayCount reports how many off days the loaded calendar holds, so a
// refresh can report what it installed. Zero means no calendar is loaded at
// all - the seed is installed during init, so this should never be observed.
func ChineseHolidayCount() int {
	set := holidayCalendar.Load()
	if set == nil {
		return 0
	}
	return len(*set)
}

// LoadHolidaySeed installs the embedded calendar. It is called from init so the
// calendar is never empty, including on the very first request after a cold
// start and on a host that cannot reach the source at all.
func LoadHolidaySeed() error {
	set, err := parseHolidayDays(holidaySeed)
	if err != nil {
		return fmt.Errorf("parse embedded holiday seed: %w", err)
	}
	if len(set) == 0 {
		return fmt.Errorf("embedded holiday seed holds no off days")
	}
	holidayCalendar.Store(&set)
	return nil
}

func init() {
	// A failure here is a build error, not a runtime condition: the seed is
	// embedded above. Panicking keeps it visible instead of shipping a binary
	// that prices every holiday as a working day.
	if err := LoadHolidaySeed(); err != nil {
		panic(err)
	}
}

func parseHolidayDays(body []byte) (map[string]struct{}, error) {
	var file holidayFile
	if err := json.Unmarshal(body, &file); err != nil {
		return nil, err
	}
	set := make(map[string]struct{}, len(file.Days))
	for _, day := range file.Days {
		// Non-off days are make-up workdays and are deliberately skipped.
		if !day.IsOffDay || day.Date == "" {
			continue
		}
		// Catch a malformed date here rather than storing a key that can never
		// match a request.
		if _, err := time.Parse("2006-01-02", day.Date); err != nil {
			return nil, fmt.Errorf("bad holiday date %q: %w", day.Date, err)
		}
		set[day.Date] = struct{}{}
	}
	if len(set) == 0 {
		return nil, fmt.Errorf("no off days in payload")
	}
	return set, nil
}

// FetchHolidays reads the current and following year from the source. Reading
// both is not belt-and-braces: the upstream files are keyed by the State
// Council document's year, so a December date can live in the next year's file,
// and the following year's file appears, empty, as soon as the notice does.
//
// A year that is absent is not an error - the next year's file does not exist
// until the government publishes, and demanding it would make every refresh
// fail for eleven months of the year. A year that exists but cannot be parsed
// is an error, because that is a change in the source's shape.
func FetchHolidays(ctx context.Context, client *http.Client, now time.Time) (map[string]struct{}, error) {
	return fetchHolidaysFrom(ctx, client, now, HolidaySourceURL)
}

// fetchHolidaysFrom is FetchHolidays against an arbitrary URL pattern (%d is
// the year), so the shape of the source - which years exist, what a malformed
// one does - can be tested without the network.
func fetchHolidaysFrom(ctx context.Context, client *http.Client, now time.Time, pattern string) (map[string]struct{}, error) {
	if client == nil {
		client = &http.Client{Timeout: holidayFetchTimeout}
	}
	utc := now.UTC()
	merged := make(map[string]struct{})
	read := 0
	var lastErr error
	for year := utc.Year(); year <= utc.Year()+1; year++ {
		set, found, err := fetchHolidayYear(ctx, client, year, pattern)
		if err != nil {
			lastErr = err
			continue
		}
		if !found {
			continue
		}
		read++
		for date := range set {
			merged[date] = struct{}{}
		}
	}
	if read == 0 {
		if lastErr != nil {
			return nil, lastErr
		}
		return nil, fmt.Errorf("no holiday years available from %s", pattern)
	}
	if len(merged) == 0 {
		return nil, fmt.Errorf("source returned no off days")
	}
	return merged, nil
}

// fetchHolidayYear returns one year's off days, and whether the source has that
// year at all (a published year with an empty `days` array is "not yet").
func fetchHolidayYear(ctx context.Context, client *http.Client, year int, pattern string) (map[string]struct{}, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, holidayFetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf(pattern, year), nil)
	if err != nil {
		return nil, false, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return nil, false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("holiday source %d: %s", year, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxHolidayBytes))
	if err != nil {
		return nil, false, err
	}
	set, err := parseHolidayDays(body)
	if err != nil {
		return nil, false, fmt.Errorf("holiday source %d: %w", year, err)
	}
	return set, true, nil
}

// RefreshHolidays fetches the calendar and installs it. On failure the previous
// calendar is left in place - never emptied - so a network outage cannot turn
// holidays into working days and re-introduce the over-billing this fixes.
func RefreshHolidays(ctx context.Context, client *http.Client, now time.Time) (int, error) {
	return refreshHolidaysFrom(ctx, client, now, HolidaySourceURL)
}

func refreshHolidaysFrom(ctx context.Context, client *http.Client, now time.Time, pattern string) (int, error) {
	set, err := fetchHolidaysFrom(ctx, client, now, pattern)
	if err != nil {
		return ChineseHolidayCount(), err
	}
	holidayCalendar.Store(&set)
	return len(set), nil
}

// HolidayRefreshLoop refreshes on the configured interval until ctx is done,
// then waits for an in-flight refresh before returning.
func HolidayRefreshLoop(ctx context.Context, interval time.Duration, client *http.Client, onResult func(int, error)) {
	if interval <= 0 {
		interval = DefaultHolidayRefreshInterval
	}
	for {
		n, err := RefreshHolidays(ctx, client, time.Now())
		if onResult != nil {
			onResult(n, err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		}
	}
}

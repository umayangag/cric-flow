package availability_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/umayangag/cric-flow/go-app/internal/availability"
	"github.com/umayangag/cric-flow/go-app/internal/config"
)

// TestCalendarDay_TakesTheDayFromTheValuesOwnZone is what `Truncate(24 * time.Hour)`
// could not do (GO-09). Truncate rounds the instant down to midnight UTC and leaves the
// value in its own zone, so a date written east or west of UTC came out as a neighbouring
// day once pgx encoded it from that value's own year/month/day.
func TestCalendarDay_TakesTheDayFromTheValuesOwnZone(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		when time.Time
		want time.Time
	}{
		{
			name: "a small hour five behind UTC",
			when: time.Date(2025, 3, 1, 1, 0, 0, 0, time.FixedZone("EST", -5*3600)),
			want: time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "a late evening five behind UTC",
			when: time.Date(2025, 3, 1, 22, 0, 0, 0, time.FixedZone("EST", -5*3600)),
			want: time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "a morning nine ahead of UTC",
			when: time.Date(2025, 3, 2, 5, 0, 0, 0, time.FixedZone("JST", 9*3600)),
			want: time.Date(2025, 3, 2, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "a half-hour offset",
			when: time.Date(2025, 6, 15, 23, 45, 0, 0, time.FixedZone("IST", 5*3600+1800)),
			want: time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "a midday in UTC",
			when: time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC),
			want: time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "a day that is already midnight UTC is unchanged",
			when: time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC),
			want: time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "the zero time stays zero",
			when: time.Time{},
			want: time.Time{},
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := availability.CalendarDay(tc.when)

			assert.Equal(t, tc.want, got)
			assert.Equal(t, time.UTC, got.Location())
		})
	}
}

// TestWindowStart_ClampsMonthArithmeticAtTheCutoffBoundary pins the arithmetic the whole
// default pool hangs on.
//
// Go's AddDate normalises rather than clamps: 31 March less one month is 3 March, which
// would quietly widen a window by three days at every month end and be invisible in the
// answer. A window is a span of months, so the boundary is the same day of the month, or
// that month's last day where there is no such day.
func TestWindowStart_ClampsMonthArithmeticAtTheCutoffBoundary(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		cutoff time.Time
		months int
		want   time.Time
	}{
		{
			name:   "twelve months back is the same day a year earlier",
			cutoff: date(2026, 9, 3),
			months: 12,
			want:   date(2025, 9, 3),
		},
		{
			name:   "nine months back crosses the year boundary",
			cutoff: date(2026, 9, 3),
			months: 9,
			want:   date(2025, 12, 3),
		},
		{
			name:   "a day of month the target month does not have clamps to its last day",
			cutoff: date(2026, 3, 31),
			months: 1,
			want:   date(2026, 2, 28),
		},
		{
			name:   "the clamp knows about leap years",
			cutoff: date(2024, 3, 31),
			months: 1,
			want:   date(2024, 2, 29),
		},
		{
			name:   "29 February a year on from a leap day clamps to 28",
			cutoff: date(2025, 2, 28),
			months: 12,
			want:   date(2024, 2, 28),
		},
		{
			name:   "a non-positive window is no window at all",
			cutoff: date(2026, 9, 3),
			months: 0,
			want:   time.Time{},
		},
	}

	for i := range testCases {
		testCase := testCases[i]
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := availability.WindowStart(testCase.cutoff, testCase.months)

			assert.Equal(t, testCase.want, got)
		})
	}
}

// TestWindowStart_IncludesItsOwnFirstDay states the half-open interval the pool query
// implements: [start, cutoff). A player whose last match was on the boundary day is in
// the window; one day earlier is out.
func TestWindowStart_IncludesItsOwnFirstDay(t *testing.T) {
	t.Parallel()
	cutoff := date(2026, 9, 3)

	start := availability.WindowStart(cutoff, 12)

	assert.False(t, start.After(date(2025, 9, 3)), "the boundary day is inside the window")
	assert.True(t, start.After(date(2025, 9, 2)), "the day before the boundary is outside it")
	assert.True(t, start.Before(cutoff))
}

// TestWindowMonths_UsesTheMeasuredPerFormatDefaults pins the measurement D-12 recorded:
// twelve months everywhere except T20I, where nine months already covers 95%.
func TestWindowMonths_UsesTheMeasuredPerFormatDefaults(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		format string
		want   int
	}{
		{name: "TEST", format: "TEST", want: 12},
		{name: "ODI", format: "ODI", want: 12},
		{name: "T20", format: "T20", want: 12},
		{name: "T20I is the one format nine months covers", format: "T20I", want: 9},
		{name: "an alias resolves to its canonical format", format: "IT20", want: 9},
		{name: "lower case is the same format", format: "t20i", want: 9},
		{
			name:   "a format nobody measured still gets a bound, never all-time",
			format: "HUNDRED",
			want:   config.DefaultPoolRecencyMonthsFallback,
		},
	}

	for i := range testCases {
		testCase := testCases[i]
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := availability.WindowMonths(nil, testCase.format)

			assert.Equal(t, testCase.want, got)
		})
	}
}

// TestWindowMonths_ConfigOverridesTheMeasuredDefault keeps the window configurable: the
// numbers are measurements of one dataset, and a deployment on another must be able to
// move them without a rebuild.
func TestWindowMonths_ConfigOverridesTheMeasuredDefault(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{}
	cfg.Pool.RecencyMonths = map[string]int{"ODI": 24}

	assert.Equal(t, 24, availability.WindowMonths(cfg, "ODI"))
	assert.Equal(t, 12, availability.WindowMonths(cfg, "TEST"), "an unnamed format keeps its default")
}

// TestPoolVocabularyIsStable guards the literals the console renders and the contract
// publishes (H-24): renaming one here without regenerating the contract fails this test
// before it reaches a surface.
func TestPoolVocabularyIsStable(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []string{"recency_window", "all_time", "manual"}, availability.PoolSources())
	assert.Equal(t, []string{"user_flagged", "retired", "both_sides"}, availability.ExclusionReasons())
	assert.Equal(t, "default", availability.DefaultActor)
}

// date builds a UTC midnight date, which is what a match date is.
func date(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

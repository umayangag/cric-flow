package biography_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/biography"
)

// TestShare_ReturnsZeroForAnEmptyDenominator keeps a format with no appearances from
// dividing by zero and rendering NaN as a coverage figure.
func TestShare_ReturnsZeroForAnEmptyDenominator(t *testing.T) {
	t.Parallel()

	assert.InDelta(t, 0.0, biography.Share(3, 0), 1e-9)
	assert.InDelta(t, 50.0, biography.Share(1, 2), 1e-9)
}

// TestTotal_SumsTheAppearanceCountsButNotThePlayerCounts: a player appears in several
// formats, so summing matched players across rows would count him several times.
func TestTotal_SumsTheAppearanceCountsButNotThePlayerCounts(t *testing.T) {
	t.Parallel()

	total := biography.Total([]biography.CoverageRow{
		{Format: "ODI", Appearances: 100, Matched: 90, BirthDate: 88, Players: 40, MatchedPlayers: 30},
		{Format: "T20", Appearances: 200, Matched: 100, BirthDate: 95, Players: 60, MatchedPlayers: 35},
	})

	assert.EqualValues(t, 300, total.Appearances)
	assert.EqualValues(t, 190, total.Matched)
	assert.EqualValues(t, 183, total.BirthDate)
	assert.Zero(t, total.Players, "the store fills the player counts in from its own query")
	assert.Zero(t, total.MatchedPlayers)
}

// TestSortRows_OrdersByCanonicalFormatThenGender so two runs render the same table.
func TestSortRows_OrdersByCanonicalFormatThenGender(t *testing.T) {
	t.Parallel()
	rows := []biography.CoverageRow{
		{Format: "T20I", Gender: "male"},
		{Format: "ODI", Gender: "male"},
		{Format: "ODI", Gender: "female"},
		{Format: "TEST", Gender: "male"},
		{Format: "HUNDRED", Gender: "male"},
	}

	biography.SortRows(rows)

	assert.Equal(t, []string{"TEST", "ODI", "ODI", "T20I", "HUNDRED"},
		[]string{rows[0].Format, rows[1].Format, rows[2].Format, rows[3].Format, rows[4].Format},
		"an unknown format sorts last rather than displacing the canonical order")
	assert.Equal(t, "female", rows[1].Gender)
}

// TestRenderMarkdown_ShowsTheFiguresAndTheGaps is the report the X-1b gate reads.
func TestRenderMarkdown_ShowsTheFiguresAndTheGaps(t *testing.T) {
	t.Parallel()
	fetched := time.Date(2026, 9, 4, 7, 52, 26, 0, time.UTC)
	coverage := biography.Coverage{
		GeneratedAt:   fetched,
		LastFetchedAt: &fetched,
		SourceLicense: biography.SourceLicense,
		Rows: []biography.CoverageRow{
			{
				Format: "ODI", Gender: "male", Appearances: 1000, Matched: 940, BirthDate: 931,
				BowlingStyle: 67, Players: 100, MatchedPlayers: 75,
			},
		},
		Unmatched: []biography.UnmatchedPlayer{
			{
				PlayerID: 9, Name: "CJA Amini", CricsheetID: "4d84ad05", CricinfoID: "332980",
				Appearances: 127, Attempted: true,
			},
		},
	}
	coverage.Total = biography.Total(coverage.Rows)

	report := biography.RenderMarkdown(coverage)

	assert.Contains(t, report, "CC0-1.0", "the licence is stated with the figures")
	assert.Contains(t, report, "93.1 %", "the DOB share the gate reads")
	assert.Contains(t, report, "CJA Amini")
	assert.Contains(t, report, "332980")
	assert.Contains(t, report, "P2697", "the report says how the join was made")
}

// TestRenderMarkdown_SaysSoWhenNothingIsUnmatched avoids an empty table that reads as a
// missing measurement.
func TestRenderMarkdown_SaysSoWhenNothingIsUnmatched(t *testing.T) {
	t.Parallel()

	report := biography.RenderMarkdown(biography.Coverage{
		SourceLicense: biography.SourceLicense,
		Rows:          []biography.CoverageRow{{Format: "ODI", Gender: "male", Appearances: 10, Matched: 10}},
	})

	require.Contains(t, report, "No player with an appearance is unmatched")
}

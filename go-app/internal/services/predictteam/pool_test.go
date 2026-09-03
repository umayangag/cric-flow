package predictteam

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/availability"
	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// matchDate is the fixture every pool test is built around.
var matchDate = time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)

// TestPoolQueryFor_DefaultsToTheMeasuredRecencyWindow is the defect's fix stated as a
// test: a caller who asks for nothing gets a bounded pool, not the all-time one that
// offered players who retired a decade ago.
func TestPoolQueryFor_DefaultsToTheMeasuredRecencyWindow(t *testing.T) {
	t.Parallel()

	query, summary := poolQueryFor("T20I", 43, matchDate, PoolRequest{}, nil, true)

	assert.Equal(t, availability.SourceRecencyWindow, summary.Source)
	assert.Equal(t, 9, summary.WindowMonths, "T20I's measured window is nine months")
	assert.Equal(t, "2025-12-10", summary.Since)
	assert.Equal(t, time.Date(2025, 12, 10, 0, 0, 0, 0, time.UTC), query.Since)
	assert.Equal(t, matchDate, query.Cutoff)
	assert.True(t, query.ApplyLedger)
}

// TestPoolQueryFor_AllTimeIsAskedForExplicitly keeps the widening a decision. There is no
// way to reach the unbounded pool by leaving a parameter out.
func TestPoolQueryFor_AllTimeIsAskedForExplicitly(t *testing.T) {
	t.Parallel()

	query, summary := poolQueryFor("ODI", 43, matchDate, PoolRequest{AllTime: true}, nil, true)

	assert.Equal(t, availability.SourceAllTime, summary.Source)
	assert.Zero(t, summary.WindowMonths)
	assert.Empty(t, summary.Since)
	assert.True(t, query.Since.IsZero(), "a zero Since is what the query reads as all-time")
}

// TestPoolQueryFor_HonoursARequestedWindow keeps the window overridable per request, as
// the plan asks: the per-format number is a default, not a rule.
func TestPoolQueryFor_HonoursARequestedWindow(t *testing.T) {
	t.Parallel()

	query, summary := poolQueryFor("ODI", 43, matchDate, PoolRequest{WindowMonths: 24}, nil, true)

	assert.Equal(t, 24, summary.WindowMonths)
	assert.Equal(t, "2024-09-10", summary.Since)
	assert.Equal(t, time.Date(2024, 9, 10, 0, 0, 0, 0, time.UTC), query.Since)
}

// TestPoolQueryFor_BacktestsCarryNoLedger is H-19 in this corner: a retirement flagged in
// 2026 says nothing about who was available in 2019, so a backtest sees no exclusions —
// while still getting the window, relative to its own as-of date.
func TestPoolQueryFor_BacktestsCarryNoLedger(t *testing.T) {
	t.Parallel()
	asOf := time.Date(2019, 6, 1, 0, 0, 0, 0, time.UTC)
	flags := map[int64]availability.Flag{7: {PlayerID: 7}}

	query, summary := poolQueryFor("ODI", 43, asOf, PoolRequest{}, flags, false)

	assert.False(t, query.ApplyLedger)
	assert.Nil(t, query.Flags, "a backtest is not shown a ledger it must then ignore")
	assert.Equal(t, "2018-06-01", summary.Since, "the window is relative to the as-of date")
}

// TestInsufficientPoolError_NamesTheWindowAndTheWayOut keeps the new failure mode
// actionable. After D-12 a short pool is usually the window, and both fixes — widen it,
// or pick by hand — are the user's to choose.
func TestInsufficientPoolError_NamesTheWindowAndTheWayOut(t *testing.T) {
	t.Parallel()

	windowed := &InsufficientPoolError{
		Team: "India (men)", Size: 8, Need: 11,
		Summary: PoolSummary{Source: availability.SourceRecencyWindow, WindowMonths: 12},
	}
	allTime := &InsufficientPoolError{
		Team: "India (men)", Size: 8, Need: 11,
		Summary: PoolSummary{Source: availability.SourceAllTime},
	}

	assert.Contains(t, windowed.Error(), "last 12 months")
	assert.Contains(t, windowed.Error(), "all-time")
	assert.NotContains(t, allTime.Error(), "months",
		"an all-time pool that is too small is not the window's doing")
}

// TestCheckPoolSize_RefusesOnlyAPoolThatCannotFieldAnXI keeps the bound where it was: a
// pool of exactly eleven is a side, and the window is allowed to leave one.
func TestCheckPoolSize_RefusesOnlyAPoolThatCannotFieldAnXI(t *testing.T) {
	t.Parallel()
	summary := PoolSummary{Source: availability.SourceRecencyWindow, WindowMonths: 12}

	testCases := []struct {
		name        string
		size        int
		wantRefused bool
	}{
		{name: "a full side is enough", size: 11},
		{name: "more than a side is enough", size: 25},
		{name: "one short cannot field an XI", size: 10, wantRefused: true},
		{name: "an empty pool cannot", size: 0, wantRefused: true},
	}

	for i := range testCases {
		testCase := testCases[i]
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := checkPoolSize("India (men)", testCase.size, 11, summary)

			if !testCase.wantRefused {
				assert.NoError(t, err)
				return
			}
			var insufficient *InsufficientPoolError
			require.ErrorAs(t, err, &insufficient)
			assert.Equal(t, testCase.size, insufficient.Size)
			assert.Equal(t, 11, insufficient.Need)
			assert.Equal(t, summary, insufficient.Summary)
		})
	}
}

// TestCandidates_RefusesARequestThatNamesNoSide stops the candidate list before it reaches
// the database: a list is for one side, and there is no guessing which (D-10).
func TestCandidates_RefusesARequestThatNamesNoSide(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		input CandidatesInput
	}{
		{name: "no format", input: CandidatesInput{Team: db.TeamRef{ClubID: 43}}},
		{name: "no side", input: CandidatesInput{Format: "T20I"}},
	}

	for i := range testCases {
		testCase := testCases[i]
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			result, err := Candidates(context.Background(), testCase.input)

			require.Error(t, err)
			assert.Nil(t, result)
		})
	}
}

// TestNewExcludedCandidates_CarriesTheReasonOntoTheWire is §8.7 for a filter: a player
// the ledger removes travels with his reason so the surface can show it and the user can
// undo it.
func TestNewExcludedCandidates_CarriesTheReasonOntoTheWire(t *testing.T) {
	t.Parallel()

	got := newExcludedCandidates([]db.ExcludedPlayer{
		{
			PlayerID:   7,
			PlayerName: "MS Dhoni",
			LastPlayed: time.Date(2019, 7, 9, 0, 0, 0, 0, time.UTC),
			Reason:     availability.ReasonRetired,
			Detail:     "no appearance in any format since 2019-07-09 (5-year bound)",
		},
		{PlayerID: 8, PlayerName: "A Player", Reason: availability.ReasonUserFlagged},
	})

	assert.Equal(t, []ExcludedCandidate{
		{
			PlayerID:   7,
			PlayerName: "MS Dhoni",
			LastPlayed: "2019-07-09",
			Reason:     availability.ReasonRetired,
			Detail:     "no appearance in any format since 2019-07-09 (5-year bound)",
		},
		{PlayerID: 8, PlayerName: "A Player", Reason: availability.ReasonUserFlagged},
	}, got)
	assert.Nil(t, newExcludedCandidates(nil), "no exclusions is an absent field, not an empty list")
}

// TestNewCandidates_ShowsExcludedPlayersInTheSameList states the surface rule: an
// exclusion a user cannot see is one he cannot undo, so the excluded players are on the
// list, marked, in name order beside everyone else.
func TestNewCandidates_ShowsExcludedPlayersInTheSameList(t *testing.T) {
	t.Parallel()

	got := newCandidates(db.PlayerPool{
		Players: []db.PlayerPoolRow{
			{
				PlayerID: 2, PlayerName: "B Player", IsWicketKeeper: 1,
				LastPlayed: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
			},
			{PlayerID: 3, PlayerName: "C Player"},
		},
		Excluded: []db.ExcludedPlayer{
			{PlayerID: 1, PlayerName: "A Player", Reason: availability.ReasonRetired},
		},
	})

	assert.Equal(t, []Candidate{
		{PlayerID: 1, PlayerName: "A Player", Excluded: true, Reason: availability.ReasonRetired},
		{PlayerID: 2, PlayerName: "B Player", IsWicketKeeper: true, LastPlayed: "2026-08-01"},
		{PlayerID: 3, PlayerName: "C Player"},
	}, got)
}

// TestFormatDate_LeavesAnUnknownDateOut keeps "we do not know when he last played" out of
// the response as an absent field rather than as a zero date pretending to be one.
func TestFormatDate_LeavesAnUnknownDateOut(t *testing.T) {
	t.Parallel()

	assert.Empty(t, formatDate(time.Time{}))
	assert.Equal(t, "2026-09-10", formatDate(matchDate))
}

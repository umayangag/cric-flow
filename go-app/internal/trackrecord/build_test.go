package trackrecord_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/predictions"
	"github.com/umayangag/cric-flow/go-app/internal/trackrecord"
)

const (
	testland  int64 = 4
	otherland int64 = 54
	elsewhere int64 = 59
)

var (
	matchDay = time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	dayAfter = time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC)
	today    = time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
)

// fakeLookup answers a fixture from a map keyed by the exact fixture, and records what it
// was asked so a test can assert the record resolved by the exact date and nothing else.
type fakeLookup struct {
	matches map[trackrecord.Fixture][]trackrecord.PlayedMatch
	asked   []trackrecord.Fixture
	err     error
}

func (f *fakeLookup) FindMatches(_ context.Context, fixture trackrecord.Fixture) ([]trackrecord.PlayedMatch, error) {
	f.asked = append(f.asked, fixture)
	if f.err != nil {
		return nil, f.err
	}
	return f.matches[fixture], nil
}

type storedOptions struct {
	id          string
	objective   string
	issued      time.Time
	matchDate   time.Time
	team1       int64
	team2       int64
	probability float64
	// ranges is nil for an answer with no scorecard (a format with no innings length).
	ranges       *[2][2]float64
	sharedFactor *bool
	players1     []int64
	players2     []int64
	payload      string
}

func stored(opts storedOptions) predictions.Prediction {
	if opts.id == "" {
		opts.id = "p-" + opts.issued.Format("0102T1504")
	}
	if opts.objective == "" {
		opts.objective = "win"
	}
	if opts.matchDate.IsZero() {
		opts.matchDate = matchDay
	}
	if opts.team1 == 0 {
		opts.team1, opts.team2 = testland, otherland
	}
	if opts.players1 == nil {
		opts.players1 = []int64{101, 102, 103}
		opts.players2 = []int64{201, 202, 203}
	}
	payload := opts.payload
	if payload == "" {
		payload = payloadFor(opts)
	}
	return predictions.Prediction{
		ID:                    opts.id,
		IssuedAt:              opts.issued,
		RunID:                 "run-1",
		RatingsThrough:        time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC),
		FormatCode:            "T20I",
		Team1OppositionID:     opts.team1,
		Team2OppositionID:     opts.team2,
		Gender:                "male",
		MatchDate:             opts.matchDate,
		SelectionObjective:    opts.objective,
		WinProbabilityTeam1:   opts.probability,
		WinProbabilitySource:  "display",
		SimulatorSharedFactor: opts.sharedFactor,
		Payload:               json.RawMessage(payload),
	}
}

func payloadFor(opts storedOptions) string {
	players := func(ids []int64) string {
		out, _ := json.Marshal(func() []map[string]int64 {
			rows := make([]map[string]int64, 0, len(ids))
			for _, id := range ids {
				rows = append(rows, map[string]int64{"player_id": id})
			}
			return rows
		}())
		return string(out)
	}
	scorecard := ""
	if opts.ranges != nil {
		scorecard = fmt.Sprintf(`, "scorecard": {"samples": 2000, "toss_marginalised": true,
			"team1_innings": {"p10": %g, "median": 0, "p90": %g}, "team2_innings": {"p10": %g, "median": 0, "p90": %g}}`,
			opts.ranges[0][0], opts.ranges[0][1], opts.ranges[1][0], opts.ranges[1][1])
	}
	return fmt.Sprintf(`{"team1_side": {"club_id": %d, "display_name": "Testland"},
		"team2_side": {"club_id": %d, "display_name": "Otherland"},
		"team1": %s, "team2": %s%s}`, opts.team1, opts.team2, players(opts.players1), players(opts.players2), scorecard)
}

func fixtureFor(team1, team2 int64, date time.Time) trackrecord.Fixture {
	return trackrecord.Fixture{Format: "T20I", Gender: "male", MatchDate: date, Team1: team1, Team2: team2}
}

func played(winner *int64, firstBats int64, firstRuns int, secondBats int64, secondRuns int, fielded map[int64]int64) trackrecord.PlayedMatch {
	return trackrecord.PlayedMatch{
		MatchID:            9001,
		WinnerOppositionID: winner,
		Innings: []trackrecord.PlayedInnings{
			{Number: 1, BattingOppositionID: firstBats, Runs: firstRuns},
			{Number: 2, BattingOppositionID: secondBats, Runs: secondRuns},
		},
		FieldedPlayers: fielded,
	}
}

func ptr[T any](v T) *T { return &v }

func allFielded() map[int64]int64 {
	return map[int64]int64{101: testland, 102: testland, 103: testland, 201: otherland, 202: otherland, 203: otherland}
}

func build(t *testing.T, lookup *fakeLookup, rows ...predictions.Prediction) *trackrecord.Record {
	t.Helper()
	record, err := trackrecord.Build(context.Background(), rows, lookup, today)
	require.NoError(t, err)
	return record
}

func entryByID(t *testing.T, record *trackrecord.Record, id string) trackrecord.Entry {
	t.Helper()
	for i := range record.Predictions {
		if record.Predictions[i].ID == id {
			return record.Predictions[i]
		}
	}
	require.Failf(t, "entry missing", "no entry %s on the record", id)
	return trackrecord.Entry{}
}

// Every state from a fixture, one row each, and the counts on the wire with every state
// present.
func TestBuild_EachStoredPredictionLandsInExactlyOneState(t *testing.T) {
	t.Parallel()
	issued := time.Date(2026, 9, 5, 10, 0, 0, 0, time.UTC)
	testCases := []struct {
		name      string
		row       predictions.Prediction
		matches   []trackrecord.PlayedMatch
		wantState string
	}{
		{
			name:      "a hand-built eleven is a scenario",
			row:       stored(storedOptions{id: "scenario", objective: "fixed", issued: issued, probability: 0.4}),
			matches:   []trackrecord.PlayedMatch{played(ptr(testland), testland, 150, otherland, 140, allFielded())},
			wantState: trackrecord.StateScenario,
		},
		{
			name:      "no match in the database is unresolved",
			row:       stored(storedOptions{id: "waiting", issued: issued, probability: 0.4}),
			matches:   nil,
			wantState: trackrecord.StateUnresolved,
		},
		{
			name:      "a match with no winner is a no-result",
			row:       stored(storedOptions{id: "washout", issued: issued, probability: 0.4}),
			matches:   []trackrecord.PlayedMatch{played(nil, testland, 40, otherland, 0, allFielded())},
			wantState: trackrecord.StateNoResult,
		},
		{
			name:      "a match with a winner is scored",
			row:       stored(storedOptions{id: "played", issued: issued, probability: 0.4}),
			matches:   []trackrecord.PlayedMatch{played(ptr(otherland), testland, 150, otherland, 151, allFielded())},
			wantState: trackrecord.StateScored,
		},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			lookup := &fakeLookup{matches: map[trackrecord.Fixture][]trackrecord.PlayedMatch{
				fixtureFor(testland, otherland, matchDay): tc.matches,
			}}

			record := build(t, lookup, tc.row)

			require.Len(t, record.Predictions, 1)
			assert.Equal(t, tc.wantState, record.Predictions[0].State)
			assert.Equal(t, 1, record.States[tc.wantState])
			assert.Len(t, record.States, len(trackrecord.States()), "every state is counted, zero included")
			total := 0
			for _, n := range record.States {
				total += n
			}
			assert.Equal(t, 1, total, "exactly one state")
		})
	}
}

// The superseding rule: of two Optimise forecasts of one fixture issued before the match,
// the later one is scored and the earlier one is superseded by it -- and a scenario of the
// same fixture, and a forecast issued after the match day, take no part.
func TestBuild_ALaterForecastOfTheSameFixtureSupersedesTheEarlierOne(t *testing.T) {
	t.Parallel()
	first := stored(storedOptions{id: "first", issued: time.Date(2026, 9, 3, 9, 0, 0, 0, time.UTC), probability: 0.3})
	// The same fixture with the sides named the other way round is the same fixture.
	second := stored(storedOptions{id: "second", issued: time.Date(2026, 9, 8, 9, 0, 0, 0, time.UTC),
		team1: otherland, team2: testland, probability: 0.7})
	scenario := stored(storedOptions{id: "scenario", objective: "fixed", issued: time.Date(2026, 9, 9, 9, 0, 0, 0, time.UTC), probability: 0.5})
	hindsight := stored(storedOptions{id: "hindsight", issued: time.Date(2026, 9, 12, 9, 0, 0, 0, time.UTC), probability: 0.9})
	lookup := &fakeLookup{matches: map[trackrecord.Fixture][]trackrecord.PlayedMatch{
		fixtureFor(testland, otherland, matchDay):  {played(ptr(testland), testland, 150, otherland, 140, allFielded())},
		fixtureFor(otherland, testland, matchDay): {played(ptr(testland), testland, 150, otherland, 140, allFielded())},
	}}

	record := build(t, lookup, first, second, scenario, hindsight)

	assert.Equal(t, trackrecord.StateSuperseded, entryByID(t, record, "first").State)
	assert.Equal(t, "second", entryByID(t, record, "first").SupersededBy)
	assert.Equal(t, trackrecord.StateScored, entryByID(t, record, "second").State)
	assert.Equal(t, trackrecord.StateScenario, entryByID(t, record, "scenario").State)
	afterTheFact := entryByID(t, record, "hindsight")
	assert.Equal(t, trackrecord.StateScored, afterTheFact.State, "issued after the match day: not superseding, not superseded")
	assert.True(t, afterTheFact.IssuedAfterMatchDate, "and flagged on the wire")
	assert.Equal(t, 2, record.Win.Overall.N)
	assert.Equal(t, []string{"hindsight", "scenario", "second", "first"}, func() []string {
		ids := make([]string, 0, 4)
		for _, e := range record.Predictions {
			ids = append(ids, e.ID)
		}
		return ids
	}(), "newest first")
}

// A forecast issued on the match day itself still counts as issued before the match.
func TestBuild_AForecastIssuedOnTheMatchDaySupersedesAnEarlierOne(t *testing.T) {
	t.Parallel()
	earlier := stored(storedOptions{id: "earlier", issued: time.Date(2026, 9, 9, 9, 0, 0, 0, time.UTC), probability: 0.3})
	matchDayForecast := stored(storedOptions{id: "match-day", issued: time.Date(2026, 9, 10, 23, 59, 0, 0, time.UTC), probability: 0.35})
	lookup := &fakeLookup{}

	record := build(t, lookup, earlier, matchDayForecast)

	assert.Equal(t, trackrecord.StateSuperseded, entryByID(t, record, "earlier").State)
	assert.Equal(t, trackrecord.StateUnresolved, entryByID(t, record, "match-day").State)
	assert.False(t, entryByID(t, record, "match-day").IssuedAfterMatchDate)
}

// The record asks for the exact fixture -- date, both sides, format, gender -- and a match
// the lookup holds on the neighbouring date is not found, because it is never asked for.
func TestBuild_ResolvesByTheExactMatchDateAndNotANeighbouringOne(t *testing.T) {
	t.Parallel()
	row := stored(storedOptions{id: "series-game-1", issued: time.Date(2026, 9, 5, 9, 0, 0, 0, time.UTC), probability: 0.4})
	lookup := &fakeLookup{matches: map[trackrecord.Fixture][]trackrecord.PlayedMatch{
		fixtureFor(testland, otherland, dayAfter): {played(ptr(testland), testland, 150, otherland, 140, allFielded())},
	}}

	record := build(t, lookup, row)

	entry := entryByID(t, record, "series-game-1")
	assert.Equal(t, trackrecord.StateUnresolved, entry.State)
	require.NotNil(t, entry.DaysPastMatchDate)
	assert.Equal(t, 10, *entry.DaysPastMatchDate, "20 September against a 10 September fixture")
	require.Len(t, lookup.asked, 1)
	assert.Equal(t, fixtureFor(testland, otherland, matchDay), lookup.asked[0])
}

// Two matches between the same sides on the same day: the record cannot say which was
// meant, and says so rather than scoring against one of them.
func TestBuild_ADoubleHeaderIsUnresolvedWithTheReasonOnTheWire(t *testing.T) {
	t.Parallel()
	row := stored(storedOptions{id: "double", issued: time.Date(2026, 9, 5, 9, 0, 0, 0, time.UTC), probability: 0.4})
	lookup := &fakeLookup{matches: map[trackrecord.Fixture][]trackrecord.PlayedMatch{
		fixtureFor(testland, otherland, matchDay): {
			played(ptr(testland), testland, 150, otherland, 140, allFielded()),
			played(ptr(otherland), otherland, 150, testland, 140, allFielded()),
		},
	}}

	record := build(t, lookup, row)

	entry := entryByID(t, record, "double")
	assert.Equal(t, trackrecord.StateUnresolved, entry.State)
	assert.Contains(t, entry.StateNote, "2 matches")
}

// The scores against hand-computed values: Brier, the base rate from the same rows, the
// coverage of both served ranges by the innings actually played, and the eleven overlap.
func TestBuild_ScoresTheScoredPredictionsAgainstHandComputedValues(t *testing.T) {
	t.Parallel()
	// Testland were given 0.8 and lost: Brier 0.64. Otherland-first fixture on another day:
	// team1 given 0.3 and won: Brier 0.49. Mean 0.565; base rate 0.5, base-rate Brier 0.25.
	lost := stored(storedOptions{id: "lost", issued: time.Date(2026, 9, 5, 9, 0, 0, 0, time.UTC), probability: 0.8,
		ranges: &[2][2]float64{{140, 180}, {130, 170}}, sharedFactor: ptr(true)})
	won := stored(storedOptions{id: "won", issued: time.Date(2026, 9, 5, 9, 0, 0, 0, time.UTC), matchDate: dayAfter,
		team1: elsewhere, team2: testland, probability: 0.3,
		ranges: &[2][2]float64{{140, 180}, {130, 170}}, sharedFactor: ptr(true),
		players1: []int64{301, 302}, players2: []int64{101, 999}})
	lookup := &fakeLookup{matches: map[trackrecord.Fixture][]trackrecord.PlayedMatch{
		// Testland batted first for 150 (inside 140-180), Otherland chased 151 (inside 130-170).
		fixtureFor(testland, otherland, matchDay): {played(ptr(otherland), testland, 150, otherland, 151, allFielded())},
		// Testland batted first for 200 (outside 130-170), Elsewhere chased 120 (outside 140-180).
		fixtureFor(elsewhere, testland, dayAfter): {played(ptr(elsewhere), testland, 200, elsewhere, 120,
			map[int64]int64{301: elsewhere, 101: testland, 999: otherland})},
	}}

	record := build(t, lookup, lost, won)

	require.Equal(t, 2, record.Win.Overall.N)
	assert.InDelta(t, 0.565, *record.Win.Overall.Brier, 1e-9)
	assert.InDelta(t, 0.5, *record.Win.Overall.BaseRate, 1e-9)
	assert.InDelta(t, 0.25, *record.Win.Overall.BaseRateBrier, 1e-9)
	assert.Equal(t, 2, record.Win.ByFormat["T20I"].N)

	lostEntry := entryByID(t, record, "lost")
	require.NotNil(t, lostEntry.Score)
	assert.InDelta(t, 0.64, lostEntry.Score.Brier, 1e-9)
	assert.False(t, lostEntry.Score.Team1Won)
	assert.True(t, *lostEntry.Score.Team1Covered)
	assert.True(t, *lostEntry.Score.Team2Covered)
	assert.Equal(t, trackrecord.ElevenOverlap{Matched: 6, Of: 6, Team1Matched: 3, Team2Matched: 3}, lostEntry.Score.ElevenOverlap)
	require.NotNil(t, lostEntry.Happened)
	assert.Equal(t, 150, *lostEntry.Happened.Team1Total)
	assert.Equal(t, 151, *lostEntry.Happened.Team2Total)
	assert.True(t, *lostEntry.Happened.Team1BattedFirst)

	wonEntry := entryByID(t, record, "won")
	require.NotNil(t, wonEntry.Score)
	assert.InDelta(t, 0.49, wonEntry.Score.Brier, 1e-9)
	assert.False(t, *wonEntry.Score.Team1Covered, "Elsewhere chased 120 against a served 140-180")
	assert.False(t, *wonEntry.Score.Team2Covered, "Testland made 200 against a served 130-170")
	// 301 played for Elsewhere; 302 did not play; 101 played for Testland; 999 played, but for
	// a third side, which is not the side it was named for.
	assert.Equal(t, trackrecord.ElevenOverlap{Matched: 2, Of: 4, Team1Matched: 1, Team2Matched: 1}, wonEntry.Score.ElevenOverlap)
	assert.False(t, *wonEntry.Happened.Team1BattedFirst)

	// Coverage by the innings actually played: two first innings (one in, one out), two
	// chases (one in, one out) -- all in the one population.
	require.Len(t, record.Coverage.Rows, 1)
	row := record.Coverage.Rows[0]
	assert.Equal(t, trackrecord.PopulationWithSharedFactor, row.Population)
	assert.Equal(t, trackrecord.CoverageScore{N: 2, Covered: 1, Coverage: ptr(0.5)}, row.FirstInnings)
	assert.Equal(t, trackrecord.CoverageScore{N: 2, Covered: 1, Coverage: ptr(0.5)}, row.Chase)

	assert.Equal(t, 2, record.Elevens.N)
	assert.InDelta(t, 4.0, *record.Elevens.Mean, 1e-9)
	assert.Equal(t, 2, *record.Elevens.Min)
	assert.Equal(t, 6, *record.Elevens.Max)
	assert.Equal(t, 1, record.Elevens.Complete)
}

// One bin, the Brier and the base-rate Brier against the harness's own functions: the
// fixture was generated by ml/xi/sim_harness.reliability and _brier, and ml-service's
// tests hold the harness to reproducing it.
func TestBuild_ReliabilityAndBrierMatchTheHarnessFixture(t *testing.T) {
	t.Parallel()
	_, file, _, _ := runtime.Caller(0)
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(file), "../../../ml-service/tests/fixtures/track_record_reliability.json"))
	require.NoError(t, err)
	var pinned struct {
		Bins          int       `json:"bins"`
		P             []float64 `json:"p"`
		Y             []float64 `json:"y"`
		Brier         float64   `json:"brier"`
		BaseRate      float64   `json:"base_rate"`
		BaseRateBrier float64   `json:"base_rate_brier"`
		Reliability   []trackrecord.ReliabilityBin
	}
	require.NoError(t, json.Unmarshal(raw, &pinned))
	require.Equal(t, trackrecord.ReliabilityBins, pinned.Bins)
	rows := make([]predictions.Prediction, 0, len(pinned.P))
	lookup := &fakeLookup{matches: map[trackrecord.Fixture][]trackrecord.PlayedMatch{}}
	for i := range pinned.P {
		// One fixture per row so none supersedes another; the side named first wins when y is 1.
		date := matchDay.AddDate(0, 0, i)
		rows = append(rows, stored(storedOptions{id: fmt.Sprintf("row-%d", i), issued: time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC),
			matchDate: date, probability: pinned.P[i]}))
		winner := otherland
		if pinned.Y[i] == 1 {
			winner = testland
		}
		lookup.matches[fixtureFor(testland, otherland, date)] = []trackrecord.PlayedMatch{played(ptr(winner), testland, 150, otherland, 140, allFielded())}
	}

	record := build(t, lookup, rows...)

	require.Equal(t, len(pinned.P), record.Win.Overall.N)
	assert.InDelta(t, pinned.Brier, *record.Win.Overall.Brier, 1e-12)
	assert.InDelta(t, pinned.BaseRate, *record.Win.Overall.BaseRate, 1e-12)
	assert.InDelta(t, pinned.BaseRateBrier, *record.Win.Overall.BaseRateBrier, 1e-12)
	require.Len(t, record.Win.Reliability, len(pinned.Reliability))
	for i := range pinned.Reliability {
		want, got := pinned.Reliability[i], record.Win.Reliability[i]
		assert.Equal(t, want.N, got.N, "bin %d", i)
		assert.InDelta(t, want.Lo, got.Lo, 1e-12, "bin %d", i)
		assert.InDelta(t, want.Hi, got.Hi, 1e-12, "bin %d", i)
		assert.InDelta(t, want.Predicted, got.Predicted, 1e-12, "bin %d", i)
		assert.InDelta(t, want.Observed, got.Observed, 1e-12, "bin %d", i)
	}
	// The pinned bin: both 0.3s fall in [0.2, 0.3), just below numpy's third edge.
	assert.Equal(t, trackrecord.ReliabilityBin{Lo: 0.2, Hi: 0.30000000000000004, N: 2, Predicted: 0.3, Observed: 0.5}, record.Win.Reliability[1])
}

// The two simulator populations are never pooled: a factored and a factorless prediction
// of the same format land in separate rows with their own denominators, an answer stored
// before the column existed is "unknown", and one with no scorecard is "not simulated".
func TestBuild_KeepsTheSimulatorPopulationsApart(t *testing.T) {
	t.Parallel()
	issued := time.Date(2026, 9, 5, 9, 0, 0, 0, time.UTC)
	ranges := &[2][2]float64{{140, 180}, {130, 170}}
	rows := []predictions.Prediction{
		stored(storedOptions{id: "factored", issued: issued, matchDate: matchDay, probability: 0.5, ranges: ranges, sharedFactor: ptr(true)}),
		stored(storedOptions{id: "factorless", issued: issued, matchDate: matchDay.AddDate(0, 0, 1), probability: 0.5, ranges: ranges, sharedFactor: ptr(false)}),
		stored(storedOptions{id: "before-the-column", issued: issued, matchDate: matchDay.AddDate(0, 0, 2), probability: 0.5, ranges: ranges}),
		stored(storedOptions{id: "no-innings-length", issued: issued, matchDate: matchDay.AddDate(0, 0, 3), probability: 0.5}),
	}
	lookup := &fakeLookup{matches: map[trackrecord.Fixture][]trackrecord.PlayedMatch{}}
	for i := range rows {
		// Testland 150 in 140-180; Otherland 200, outside 130-170.
		lookup.matches[fixtureFor(testland, otherland, rows[i].MatchDate)] = []trackrecord.PlayedMatch{played(ptr(testland), testland, 150, otherland, 200, allFielded())}
	}

	record := build(t, lookup, rows...)

	assert.Equal(t, map[string]int{
		trackrecord.PopulationWithSharedFactor:    1,
		trackrecord.PopulationWithoutSharedFactor: 1,
		trackrecord.PopulationUnknown:             1,
		trackrecord.PopulationNotSimulated:        1,
	}, record.Coverage.Populations)
	require.Len(t, record.Coverage.Rows, 4, "one row per population, no pooled row")
	byPopulation := map[string]trackrecord.CoverageRow{}
	for _, row := range record.Coverage.Rows {
		byPopulation[row.Population] = row
	}
	for _, simulated := range []string{trackrecord.PopulationWithSharedFactor, trackrecord.PopulationWithoutSharedFactor, trackrecord.PopulationUnknown} {
		assert.Equal(t, trackrecord.CoverageScore{N: 1, Covered: 1, Coverage: ptr(1.0)}, byPopulation[simulated].FirstInnings, simulated)
		assert.Equal(t, trackrecord.CoverageScore{N: 1, Covered: 0, Coverage: ptr(0.0)}, byPopulation[simulated].Chase, simulated)
	}
	notSimulated := byPopulation[trackrecord.PopulationNotSimulated]
	assert.Equal(t, 1, notSimulated.NPredictions)
	assert.Equal(t, trackrecord.CoverageScore{N: 0, Covered: 0, Coverage: nil}, notSimulated.FirstInnings)
	assert.Nil(t, entryByID(t, record, "no-innings-length").Claimed.Team1Range)
	assert.Equal(t, 4, record.Win.Overall.N, "the win probability is scored for all four")
}

// An empty record is an empty record: every count zero, no curve invented.
func TestBuild_AnEmptyRecordInventsNothing(t *testing.T) {
	t.Parallel()
	record := build(t, &fakeLookup{})

	assert.Equal(t, 0, record.Total)
	assert.Equal(t, 0, record.Win.Overall.N)
	assert.Nil(t, record.Win.Overall.Brier)
	assert.Nil(t, record.Win.Overall.BaseRateBrier)
	assert.Empty(t, record.Win.Reliability)
	assert.Empty(t, record.Coverage.Rows)
	assert.Equal(t, 0, record.Elevens.N)
	assert.Empty(t, record.Predictions)
	assert.Equal(t, len(trackrecord.States()), len(record.States))
}

// A payload the record cannot read keeps its row, in its state, with the error on it.
func TestBuild_AnUnreadablePayloadStaysOnTheRecordWithTheErrorNamed(t *testing.T) {
	t.Parallel()
	row := stored(storedOptions{id: "corrupt", issued: time.Date(2026, 9, 5, 9, 0, 0, 0, time.UTC), probability: 0.4, payload: "{not json"})
	lookup := &fakeLookup{}

	record := build(t, lookup, row)

	entry := entryByID(t, record, "corrupt")
	assert.Equal(t, trackrecord.StateUnresolved, entry.State)
	assert.Contains(t, entry.PayloadError, "read stored payload")
	assert.Equal(t, trackrecord.PopulationNotSimulated, entry.Population)
}

// A lookup failure is an error, never a record that quietly reads "unresolved".
func TestBuild_ALookupFailureIsReturned(t *testing.T) {
	t.Parallel()
	row := stored(storedOptions{id: "any", issued: time.Date(2026, 9, 5, 9, 0, 0, 0, time.UTC), probability: 0.4})

	_, err := trackrecord.Build(context.Background(), []predictions.Prediction{row}, &fakeLookup{err: fmt.Errorf("connection refused")}, today)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection refused")
}

func TestVocabularies_AreDeclaredOnce(t *testing.T) {
	t.Parallel()
	assert.Equal(t, []string{"scenario", "superseded", "unresolved", "no_result", "scored"}, trackrecord.States())
	assert.Equal(t, []string{"with_shared_factor", "without_shared_factor", "unknown", "not_simulated"}, trackrecord.Populations())
	assert.Equal(t, []string{"brier", "record_base_rate_brier", "reliability", "coverage_80", "eleven_overlap"}, trackrecord.MetricKeys())
}

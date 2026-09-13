package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/db/dbtest"
	"github.com/umayangag/cric-flow/go-app/internal/trackrecord"
)

// The track record end to end against a real database, through the real handlers (P2-4):
// a prediction in each state, and an import moving one to scored with no operator step.
//
// These truncate. Run them against a scratch database, never one holding an import:
// `make -C go-app test-db`; dbtest.SkipUnlessScratchDatabase refuses the working one.

// issuePrediction files one answer through the real prediction handler and returns its id.
func issuePrediction(t *testing.T, app *App, body string) string {
	t.Helper()
	rec := httptest.NewRecorder()
	app.predictTeamSelectionHandler(rec, jsonPredictRequest(body))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var served servedPredictionBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &served))
	require.True(t, served.Record.Stored)
	return served.Record.ID
}

// backdate moves a stored prediction's issued_at, so a later forecast of the same fixture
// can be issued in the same test run.
func backdate(t *testing.T, id string, issuedAt time.Time) {
	t.Helper()
	require.NoError(t, db.Exec(context.Background(),
		`UPDATE issued_prediction SET issued_at = $2 WHERE id = $1`, id, issuedAt))
}

// importMatch is what an import leaves behind for a played match: the match row, its
// innings and the fielded elevens.
func importMatch(t *testing.T, matchID int64, date string, team1, team2 int64, winner *int64, playerIDs []int64) {
	t.Helper()
	ctx := context.Background()
	formatID, err := db.GetOrCreateMatchFormat(ctx, "TEST")
	require.NoError(t, err)
	require.NoError(t, db.Exec(ctx,
		`INSERT INTO match (match_id, format_id, match_date, original_match_type, gender, outcome_winner_opposition_id)
		 VALUES ($1, $2, $3, 'TEST', 'male', $4)`, matchID, formatID, date, winner))
	require.NoError(t, db.Exec(
		ctx,
		`INSERT INTO match_inning (match_id, inning_number, batting_team_opposition_id, bowling_team_opposition_id, runs_scored)
		 VALUES ($1, 1, $2, $3, 250), ($1, 2, $3, $2, 240)`,
		matchID,
		team1,
		team2,
	))
	for i, playerID := range playerIDs {
		side := team1
		if i >= len(playerIDs)/2 {
			side = team2
		}
		require.NoError(t, db.Exec(
			ctx,
			`INSERT INTO match_player (match_id, player_id, opposition_id) VALUES ($1, $2, $3)`,
			matchID,
			playerID,
			side,
		))
	}
}

func readTrackRecord(t *testing.T, app *App) trackrecord.Record {
	t.Helper()
	rec := httptest.NewRecorder()
	app.trackRecordHandler(rec, httptest.NewRequest(http.MethodGet, "/api/track-record", nil))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var record trackrecord.Record
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &record))
	return record
}

func stateOf(record trackrecord.Record, id string) string {
	for _, entry := range record.Predictions {
		if entry.ID == id {
			return entry.State
		}
	}
	return "missing from the record"
}

// Every state at once, on the same record: a scenario, two forecasts of one fixture (the
// earlier superseded), a no-result, an unresolved fixture, and a scored one -- then the
// import that resolves the unresolved one, with the record read again and nothing else done.
func TestTrackRecord_EveryStateFromTheDatabase_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	fixture := seedPredictFixture(t)
	app := &App{mlClient: scriptedMLService(t, nil)}
	optimise := func(date string) string {
		return fmt.Sprintf(
			`{"format":"TEST","team1_id":%d,"team2_id":%d,"match_date":%q}`,
			fixture.team1ID,
			fixture.team2ID,
			date,
		)
	}

	// A fixture predicted twice: the earlier forecast is superseded by the later one.
	earlier := issuePrediction(t, app, optimise("2026-09-10"))
	backdate(t, earlier, time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC))
	later := issuePrediction(t, app, optimise("2026-09-10"))
	backdate(t, later, time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC))
	// A hand-built eleven of the same fixture: a scenario, never scored.
	scenarioBody := fmt.Sprintf(`{"format":"TEST","team1_id":%d,"team2_id":%d,"match_date":"2026-09-10",
		"team1_xi":[1,2,3,4,5,6,7,8,9,10,11],"team2_xi":[12,13,14,15,16,17,18,19,20,21,22]}`, fixture.team1ID, fixture.team2ID)
	scenario := issuePrediction(t, app, scenarioBody)
	// A fixture the database will hold with no winner.
	washout := issuePrediction(t, app, optimise("2026-09-12"))
	backdate(t, washout, time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC))
	// A fixture nothing has imported yet.
	pending := issuePrediction(t, app, optimise("2026-09-14"))
	backdate(t, pending, time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC))

	team1Wins := fixture.team1ID
	importMatch(t, 9010, "2026-09-10", fixture.team1ID, fixture.team2ID, &team1Wins, []int64{1, 2, 3, 12, 13, 14})
	importMatch(t, 9012, "2026-09-12", fixture.team1ID, fixture.team2ID, nil, nil)

	record := readTrackRecord(t, app)

	assert.Equal(t, 5, record.Total)
	assert.Equal(t, trackrecord.StateSuperseded, stateOf(record, earlier))
	assert.Equal(t, trackrecord.StateScored, stateOf(record, later))
	assert.Equal(t, trackrecord.StateScenario, stateOf(record, scenario))
	assert.Equal(t, trackrecord.StateNoResult, stateOf(record, washout))
	assert.Equal(t, trackrecord.StateUnresolved, stateOf(record, pending))
	assert.Equal(t, map[string]int{
		trackrecord.StateScenario: 1, trackrecord.StateSuperseded: 1, trackrecord.StateUnresolved: 1,
		trackrecord.StateNoResult: 1, trackrecord.StatePostHoc: 0, trackrecord.StateScored: 1,
	}, record.States)
	require.Equal(t, 1, record.Win.Overall.N)
	// The scripted ml-service gives team1 0.6 and team1 won: Brier 0.16, base rate 1.0.
	assert.InDelta(t, 0.16, *record.Win.Overall.Brier, 1e-9)
	assert.InDelta(t, 0.0, *record.Win.Overall.BaseRateBrier, 1e-9)
	assert.Equal(t, 1, record.Coverage.Populations[trackrecord.PopulationNotSimulated], "TEST has no simulator")
	assert.Equal(t, 1, record.Elevens.N)

	// Clause 4: the import that brings the pending match moves it to scored on the next
	// read. Nothing is run between the insert and the read.
	team2Wins := fixture.team2ID
	importMatch(t, 9014, "2026-09-14", fixture.team1ID, fixture.team2ID, &team2Wins, nil)

	after := readTrackRecord(t, app)

	assert.Equal(t, trackrecord.StateScored, stateOf(after, pending))
	assert.Equal(t, 2, after.Win.Overall.N)
	assert.Equal(t, 0, after.States[trackrecord.StateUnresolved])
	// The miss is on the record: 0.6 for a side that lost, Brier 0.36, mean (0.16 + 0.36) / 2.
	assert.InDelta(t, 0.26, *after.Win.Overall.Brier, 1e-9)
}

// The resolution is by the exact date: a match between the same sides the day after the
// predicted date leaves the prediction unresolved, with the days since the match date.
func TestTrackRecord_ANeighbouringDateIsNotTheFixture_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	fixture := seedPredictFixture(t)
	app := &App{mlClient: scriptedMLService(t, nil)}
	id := issuePrediction(
		t,
		app,
		fmt.Sprintf(
			`{"format":"TEST","team1_id":%d,"team2_id":%d,"match_date":"2026-09-10"}`,
			fixture.team1ID,
			fixture.team2ID,
		),
	)
	backdate(t, id, time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC))
	winner := fixture.team1ID
	importMatch(t, 9011, "2026-09-11", fixture.team1ID, fixture.team2ID, &winner, nil)

	record := readTrackRecord(t, app)

	assert.Equal(t, trackrecord.StateUnresolved, stateOf(record, id))
	require.NotNil(t, record.Predictions[0].DaysPastMatchDate)
}

// recordRename points a superseded opposition row at the club's current row, the way the
// importer writes opposition.canonical_id at the end of a run from configs/team_lineage.json.
func recordRename(t *testing.T, superseded, current int64) {
	t.Helper()
	require.NoError(t, db.Exec(context.Background(),
		`UPDATE opposition SET canonical_id = $2 WHERE id = $1`, superseded, current))
}

// leaveClubUnrenamed is the other half of the pair: the phase at which this case does not
// rename anything.
func leaveClubUnrenamed(*testing.T, int64, int64) {}

// entryOf is the record's row for one prediction id.
func entryOf(t *testing.T, record trackrecord.Record, id string) trackrecord.Entry {
	t.Helper()
	for _, entry := range record.Predictions {
		if entry.ID == id {
			return entry
		}
	}
	require.FailNowf(t, "prediction missing from the record", "id %s", id)
	return trackrecord.Entry{}
}

// everyFieldedPlayer is the twenty-two players seedPredictFixture inserts, eleven a side
// in the order the two innings put them in, so that every player an eleven can name is
// also a player who took the field.
func everyFieldedPlayer() []int64 {
	players := make([]int64, 0, 22)
	for id := int64(1); id <= 22; id++ {
		players = append(players, id)
	}
	return players
}

// A club that renames is one club to the record (GO-02).
//
// Cricsheet names a team by whatever it was called on the day, so a rebrand splits a club
// into two opposition rows and a match keeps whichever row played; a prediction is filed
// under the club id, COALESCE(canonical_id, id). Matching the two spaces against each
// other leaves such a fixture unresolved forever, and where one side happens to match it
// scores the other side's half the wrong way round: team1_won inverted, no team1_total,
// team1_batted_first reversed, an eleven overlap of zero. Both orderings are here --
// the lineage written before the forecast was issued, and written after the match was
// imported -- because the record must not depend on when somebody reviewed the rename.
func TestTrackRecord_ARenamedClubFixtureResolvesAndScores_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	testCases := []struct {
		name string
		// beforeTheForecast and afterTheImport are the two phases the lineage row can be
		// written at; each case renames at one of them and does nothing at the other.
		beforeTheForecast func(t *testing.T, superseded, current int64)
		afterTheImport    func(t *testing.T, superseded, current int64)
		// forecastTeam1 is the row the forecast names team1 by: the club id as it stood
		// when the forecast was issued.
		forecastTeam1 func(superseded, current int64) int64
	}{
		{
			name:              "the club renamed before the forecast was issued",
			beforeTheForecast: recordRename,
			afterTheImport:    leaveClubUnrenamed,
			forecastTeam1:     func(_, current int64) int64 { return current },
		},
		{
			name:              "the club renamed after the match was imported",
			beforeTheForecast: leaveClubUnrenamed,
			afterTheImport:    recordRename,
			forecastTeam1:     func(superseded, _ int64) int64 { return superseded },
		},
	}
	for i := range testCases {
		t.Run(testCases[i].name, func(t *testing.T) {
			ctx := context.Background()
			fixture := seedPredictFixture(t)
			currentRow := insertClub(ctx, t, "Testland United")
			app := &App{mlClient: scriptedMLService(t, nil)}
			testCases[i].beforeTheForecast(t, fixture.team1ID, currentRow)

			id := issuePrediction(t, app, fmt.Sprintf(
				`{"format":"TEST","team1_id":%d,"team2_id":%d,"match_date":"2026-09-10"}`,
				testCases[i].forecastTeam1(fixture.team1ID, currentRow), fixture.team2ID))
			backdate(t, id, time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC))
			// The import writes the row that played on the day -- the pre-rename one --
			// with team1 batting first, making 250, and winning.
			winner := fixture.team1ID
			importMatch(t, 9010, "2026-09-10",
				fixture.team1ID, fixture.team2ID, &winner, everyFieldedPlayer())
			testCases[i].afterTheImport(t, fixture.team1ID, currentRow)

			record := readTrackRecord(t, app)

			require.Equal(t, trackrecord.StateScored, stateOf(record, id))
			entry := entryOf(t, record, id)
			assert.Equal(t, currentRow, entry.Team1.ID, "the record names team1 by its club id")
			require.NotNil(t, entry.Score)
			assert.True(t, entry.Score.Team1Won, "team1 won, and the record has to say so")
			// The scripted ml-service gives team1 0.6 and team1 won: Brier 0.16, not 0.36.
			assert.InDelta(t, 0.16, entry.Score.Brier, 1e-9)
			assert.Positive(t, entry.Score.ElevenOverlap.Team1Matched)
			assert.Positive(t, entry.Score.ElevenOverlap.Team2Matched)
			assert.Equal(t, entry.Score.ElevenOverlap.Of, entry.Score.ElevenOverlap.Matched,
				"every named player took the field for the side they were named for")
			require.NotNil(t, entry.Happened)
			require.NotNil(t, entry.Happened.WinnerOppositionID)
			assert.Equal(t, currentRow, *entry.Happened.WinnerOppositionID)
			require.NotNil(t, entry.Happened.Team1Total)
			assert.Equal(t, 250, *entry.Happened.Team1Total)
			require.NotNil(t, entry.Happened.Team2Total)
			assert.Equal(t, 240, *entry.Happened.Team2Total)
			require.NotNil(t, entry.Happened.Team1BattedFirst)
			assert.True(t, *entry.Happened.Team1BattedFirst)
		})
	}
}

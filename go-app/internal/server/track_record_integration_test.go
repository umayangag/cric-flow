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
		trackrecord.StateNoResult: 1, trackrecord.StateScored: 1,
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

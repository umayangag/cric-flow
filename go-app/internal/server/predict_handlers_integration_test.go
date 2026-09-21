package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/db/dbtest"
)

// The prediction handler end to end against a real database and a scripted ml-service
// (P1-5): a served prediction carries the run and the date it was served from, and a
// stale registry's refusal reaches the caller as the 503 ml-service wrote, code intact.
//
// These truncate. Run them against a scratch database, never one holding an import:
// `make -C go-app test-db`; dbtest.SkipUnlessScratchDatabase refuses the working one.

// predictFixture is one TEST fixture the handler can predict: two clubs, eleven players a
// side, one match between them inside the recency window of the match date.
type predictFixture struct {
	team1ID   int64
	team2ID   int64
	matchDate string
}

func seedPredictFixture(t *testing.T) predictFixture {
	t.Helper()
	ctx := context.Background()
	pool, err := db.Connect(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })
	_, file, _, _ := runtime.Caller(0)
	require.NoError(t, db.RunMigrations(ctx, filepath.Clean(filepath.Join(filepath.Dir(file), "../../migrations"))))
	require.NoError(t, db.Exec(ctx, `TRUNCATE TABLE
		auction_player, auction_venue, auction_likely_xi,
		auction_opposition_player, auction_opposition, auction,
		issued_prediction,
		player_status_event, player_status, player_biography,
		ball_event_wicket, ball_event, match_player, batting_data, bowling_data,
		fielding_data, fielding_event, match_inning, match, player, opposition,
		venue_weather, venue RESTART IDENTITY`))

	formatID, err := db.GetOrCreateMatchFormat(ctx, "TEST")
	require.NoError(t, err)
	fixture := predictFixture{matchDate: "2026-09-10"}
	fixture.team1ID = insertClub(ctx, t, "Testland")
	fixture.team2ID = insertClub(ctx, t, "Otherland")

	const matchID = 9001
	require.NoError(t, db.Exec(ctx,
		`INSERT INTO match (match_id, format_id, match_date, original_match_type)
		 VALUES ($1, $2, '2026-06-01', 'TEST')`, matchID, formatID))
	for inning, side := range []struct{ bats, bowls int64 }{
		{fixture.team1ID, fixture.team2ID},
		{fixture.team2ID, fixture.team1ID},
	} {
		require.NoError(t, db.Exec(ctx,
			`INSERT INTO match_inning
			   (match_id, inning_number, batting_team_opposition_id, bowling_team_opposition_id)
			 VALUES ($1, $2, $3, $4)`, matchID, inning+1, side.bats, side.bowls))
		for i := 0; i < 11; i++ {
			playerID := insertPlayerRow(
				ctx,
				t,
				fmt.Sprintf("%d%02x", inning+1, i),
				fmt.Sprintf("Player %d-%d", inning+1, i),
			)
			require.NoError(t, db.Exec(ctx,
				`INSERT INTO batting_data (match_id, inning_number, player_id) VALUES ($1, $2, $3)`,
				matchID, inning+1, playerID))
		}
	}
	return fixture
}

func insertClub(ctx context.Context, t *testing.T, name string) int64 {
	t.Helper()
	var id int64
	require.NoError(t, db.Pool.QueryRow(ctx,
		`INSERT INTO opposition (opposition_name, gender) VALUES ($1, 'male') RETURNING id`, name).Scan(&id))
	return id
}

func insertPlayerRow(ctx context.Context, t *testing.T, externalID, name string) int64 {
	t.Helper()
	var id int64
	require.NoError(t, db.Pool.QueryRow(ctx,
		`INSERT INTO player (external_id, player_name, is_wicket_keeper, is_retired)
		 VALUES ($1, $2, 0, 0) RETURNING id`, externalID, name).Scan(&id))
	return id
}

// asOfRecorder collects the `as_of` every call to the scripted ml-service carried, so a
// test can say what go-app sent rather than what it meant to send.
type asOfRecorder struct {
	mu   sync.Mutex
	seen []string
}

func (r *asOfRecorder) record(asOf string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seen = append(r.seen, asOf)
}

func (r *asOfRecorder) calls() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string{}, r.seen...)
}

// scriptedMLService answers the calls a TEST prediction makes -- /xi/optimize for each side,
// /xi/predict-win and /performance/predict -- every answer stamped with the same run and date. With
// `refusal` set, /xi/optimize answers that instead, which is where H-11 refuses first.
func scriptedMLService(t *testing.T, refusal *mlServiceError) *MLClient {
	t.Helper()
	return scriptedMLServiceRecording(t, refusal, nil)
}

// scriptedMLServiceRecording is scriptedMLService with every call's `as_of` written to
// `asOf` -- an empty string where the call carried none.
func scriptedMLServiceRecording(t *testing.T, refusal *mlServiceError, asOf *asOfRecorder) *MLClient {
	t.Helper()
	const stamp = `"served_ratings": {"run_id": "20260906T083819Z-36689f80", "ratings_through": "2026-09-02"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if refusal != nil {
			w.WriteHeader(refusal.Status)
			_ = json.NewEncoder(w).Encode(map[string]any{"detail": map[string]any{
				"code": refusal.Code, "message": refusal.Message, "hint": refusal.Hint,
			}})
			return
		}
		var body struct {
			AsOf           string   `json:"as_of"`
			PoolPlayerIDs  []string `json:"pool_player_ids"`
			Team1PlayerIDs []string `json:"team1_player_ids"`
			Team2PlayerIDs []string `json:"team2_player_ids"`
			// Team1BatsFirst is echoed back through `toss_marginalised` and
			// `innings_marginalised`, because that is what ml-service does: a fake that
			// always claimed to have marginalised would be answering a shape the real
			// service never sends, and go-app refuses the pair when they disagree (GO-07).
			Team1BatsFirst   *bool `json:"team1_bats_first"`
			Team1Constraints *struct {
				MinBowlers    int      `json:"min_bowlers"`
				RequireKeeper bool     `json:"require_keeper"`
				MustInclude   []string `json:"must_include"`
			} `json:"team1_constraints"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		if asOf != nil {
			asOf.record(body.AsOf)
		}
		marginalised := body.Team1BatsFirst == nil
		switch r.URL.Path {
		case "/xi/optimize":
			selected, err := json.Marshal(body.PoolPlayerIDs[:11])
			require.NoError(t, err)
			reasons, err := json.Marshal(ratingOrderedReasons(body.PoolPlayerIDs))
			require.NoError(t, err)
			_, _ = fmt.Fprintf(w, `{"selected_player_ids": %s, "objective": "ratings", "optimised": false,
				"evaluations": 0, "improved_over_seed": 0, "unknown_player_ids": [], "marginal_values": {},
				"selection_reasons": %s, %s}`,
				selected, reasons, stamp)
		case "/xi/predict-win":
			// Play mode: the eleven that was sent is checked against the constraints
			// that came with it, and the check rides back on the same answer (P1-2).
			checks := ""
			if body.Team1Constraints != nil {
				missing, err := json.Marshal(body.Team1Constraints.MustInclude)
				require.NoError(t, err)
				checks = fmt.Sprintf(`"team1_constraint_check": {"team_size": %d, "bowlers": 4, "min_bowlers": %d,
					"has_keeper": false, "require_keeper": %t, "missing_must_include": %s, "met": false},
					"team2_constraint_check": {"team_size": %d, "bowlers": 6, "min_bowlers": %d,
					"has_keeper": true, "require_keeper": %t, "missing_must_include": [], "met": true},`,
					len(body.Team1PlayerIDs), body.Team1Constraints.MinBowlers, body.Team1Constraints.RequireKeeper,
					missing, len(body.Team2PlayerIDs), body.Team1Constraints.MinBowlers,
					body.Team1Constraints.RequireKeeper)
			}
			_, _ = fmt.Fprintf(w,
				`{"team1_win_probability": 0.6, "objective_probability": 0.55, "toss_marginalised": %t, %s %s}`,
				marginalised, checks, stamp)
		case "/performance/predict":
			lines := make([]string, 0, 22)
			// Both elevens come back in one flat list, and each row says which side it is
			// for: go-app reads it by (side, player_id) since GO-04, so a fake that put
			// every row on side 1 would be answering a shape ml-service never sends.
			sides := []struct {
				number int
				ids    []string
			}{{number: 1, ids: body.Team1PlayerIDs}, {number: 2, ids: body.Team2PlayerIDs}}
			for _, side := range sides {
				for _, id := range side.ids {
					lines = append(lines, fmt.Sprintf(`{"player_id": %q, "side": %d, "p_bats": 1, "p_bowls": 0.5,
					"runs": {"q10": 3, "median": 26, "q90": 71}, "balls_faced": {"q10": 8, "median": 44, "q90": 110},
					"runs_conceded": {"q10": 10, "median": 33, "q90": 60},
					"wickets": {"expected": 1.4, "p0": 0.3, "p1": 0.4, "p2_plus": 0.3}, "catches_expected": 0.5}`,
						id, side.number))
				}
			}
			_, _ = fmt.Fprintf(w, `{"players": [%s], "innings_marginalised": %t, "unknown_player_ids": [], %s}`,
				strings.Join(lines, ","), marginalised, stamp)
		default:
			t.Errorf("unexpected ml-service call %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return &MLClient{BaseURL: srv.URL, HTTP: srv.Client()}
}

// ratingOrderedReasons is what ml-service sends back for the rating-ordered pick a TEST
// fixture gets (P1-3): a reason per selected player, and no best alternative anywhere,
// because nothing was maximised.
func ratingOrderedReasons(poolIDs []string) map[string]map[string]any {
	reasons := map[string]map[string]any{}
	for i, id := range poolIDs[:11] {
		reasons[id] = map[string]any{
			"roles":             []string{"bowling_option"},
			"selection_rating":  1.5 - float64(i)*0.1,
			"rating_percentile": 100 - float64(i)*5,
			"pool_size":         len(poolIDs),
		}
	}
	return reasons
}

func predictRequestFor(fixture predictFixture) *http.Request {
	return jsonPredictRequest(fmt.Sprintf(
		`{"format":"TEST","team1_id":%d,"team2_id":%d,"match_date":%q}`,
		fixture.team1ID, fixture.team2ID, fixture.matchDate))
}

// GO-01 end to end: a request for a played match reaches ml-service with `as_of` naming the
// match date on every call it makes, so the ratings it is answered from stop before the
// match; a request for an upcoming match names nothing and is answered through today.
func TestPredictTeamSelectionHandler_SendsAsOfForAPlayedMatchOnly_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	fixture := seedPredictFixture(t)
	upcoming := time.Now().UTC().AddDate(0, 0, 7).Format(time.DateOnly)

	testCases := []struct {
		name      string
		matchDate string
		wantAsOf  string
	}{
		{name: "a played match names its date", matchDate: "2026-08-01", wantAsOf: "2026-08-01"},
		{name: "an upcoming match names nothing", matchDate: upcoming, wantAsOf: ""},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			recorder := &asOfRecorder{}
			app := &App{mlClient: scriptedMLServiceRecording(t, nil, recorder)}
			rec := httptest.NewRecorder()
			// A twelve-month window keeps the seeded 2026-06-01 match in the pool from
			// either date, so both requests are answered and the only difference is as_of.
			request := jsonPredictRequest(fmt.Sprintf(
				`{"format":"TEST","team1_id":%d,"team2_id":%d,"match_date":%q,
				  "team1_pool":{"window_months":12},"team2_pool":{"window_months":12}}`,
				fixture.team1ID, fixture.team2ID, tc.matchDate))

			app.predictTeamSelectionHandler(rec, request)

			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			calls := recorder.calls()
			require.NotEmpty(t, calls, "the prediction made no ml-service call")
			for _, asOf := range calls {
				assert.Equal(t, tc.wantAsOf, asOf)
			}
		})
	}
}

// GO-08 end to end: a venue the caller named and this database does not hold is refused,
// and nothing is written. The lookup used to be a get-or-create, so this request inserted
// a venue row and then predicted at a ground with no history behind it, answering 200 as
// though the venue had been found.
func TestPredictTeamSelectionHandler_AnUnknownVenueIsRefusedAndCreatesNothing_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	fixture := seedPredictFixture(t)
	app := &App{mlClient: scriptedMLService(t, nil)}
	rec := httptest.NewRecorder()
	request := jsonPredictRequest(fmt.Sprintf(
		`{"format":"TEST","team1_id":%d,"team2_id":%d,"match_date":%q,"venue":"No Such Ground"}`,
		fixture.team1ID, fixture.team2ID, fixture.matchDate))

	app.predictTeamSelectionHandler(rec, request)

	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	var body struct {
		Code string `json:"code"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "VENUE_NOT_FOUND", body.Code)
	assert.Zero(t, countVenuesNamed(t, "No Such Ground"),
		"a request that names a venue must never create one")
}

// The other two outcomes, on the wire: a venue that is held is named in the answer, and a
// request that names none says the fixture was read without one. Before GO-08 the response
// carried no venue at all, so the two were indistinguishable.
func TestPredictTeamSelectionHandler_TheAnswerNamesTheVenueItRead_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	fixture := seedPredictFixture(t)
	venueID := insertVenue(t, "Testland Oval")

	testCases := []struct {
		name         string
		venue        string
		wantResolved bool
		wantVenueID  int64
		wantName     string
		wantNote     string
	}{
		{
			name:         "a venue that is held",
			venue:        `,"venue":"Testland Oval"`,
			wantResolved: true,
			wantVenueID:  venueID,
			wantName:     "Testland Oval",
		},
		{
			name:     "no venue named",
			venue:    "",
			wantNote: "no venue was named; every model read this fixture without one",
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			app := &App{mlClient: scriptedMLService(t, nil)}
			rec := httptest.NewRecorder()
			request := jsonPredictRequest(fmt.Sprintf(
				`{"format":"TEST","team1_id":%d,"team2_id":%d,"match_date":%q%s}`,
				fixture.team1ID, fixture.team2ID, fixture.matchDate, tc.venue))

			app.predictTeamSelectionHandler(rec, request)

			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			var payload struct {
				Venue struct {
					Resolved bool   `json:"resolved"`
					VenueID  int64  `json:"venue_id"`
					Name     string `json:"name"`
					Note     string `json:"note"`
				} `json:"venue"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
			assert.Equal(t, tc.wantResolved, payload.Venue.Resolved)
			assert.Equal(t, tc.wantVenueID, payload.Venue.VenueID)
			assert.Equal(t, tc.wantName, payload.Venue.Name)
			assert.Equal(t, tc.wantNote, payload.Venue.Note)
		})
	}
}

// GO-09 end to end: the same day written two ways is answered the same way. The seeded
// match is played on 2026-06-01 and is the only history either side has, so a fixture on
// the 2nd holds it in the pool -- unless the cutoff slips a day, which is what truncating
// an offset-bearing instant did. The offset spelling was then answered with an empty pool.
func TestPredictTeamSelectionHandler_AnOffsetBearingMatchDateKeepsThePreviousDay_Integration(
	t *testing.T,
) {
	dbtest.SkipUnlessScratchDatabase(t)
	fixture := seedPredictFixture(t)

	testCases := []struct {
		name      string
		matchDate string
	}{
		{name: "the bare day", matchDate: "2026-06-02"},
		{name: "the same day written five hours behind UTC", matchDate: "2026-06-02T01:00:00-05:00"},
		{name: "the same day written nine hours ahead of UTC", matchDate: "2026-06-02T22:00:00+09:00"},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			app := &App{mlClient: scriptedMLService(t, nil)}
			rec := httptest.NewRecorder()
			request := jsonPredictRequest(fmt.Sprintf(
				`{"format":"TEST","team1_id":%d,"team2_id":%d,"match_date":%q,
				  "team1_pool":{"window_months":12},"team2_pool":{"window_months":12}}`,
				fixture.team1ID, fixture.team2ID, tc.matchDate))

			app.predictTeamSelectionHandler(rec, request)

			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			var payload struct {
				Team1Pool struct {
					Size  int    `json:"size"`
					Since string `json:"since"`
				} `json:"team1_pool"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
			assert.Equal(t, 11, payload.Team1Pool.Size, "the previous day's match is in the window")
			assert.Equal(t, "2025-06-02", payload.Team1Pool.Since)
		})
	}
}

// insertVenue adds one venue the prediction path can resolve, and returns its id.
func insertVenue(t *testing.T, name string) int64 {
	t.Helper()
	var id int64
	require.NoError(t, db.Pool.QueryRow(context.Background(),
		`INSERT INTO venue (venue_name) VALUES ($1) RETURNING id`, name).Scan(&id))
	return id
}

// countVenuesNamed reports how many venue rows carry this name, so a test can say that a
// refused request wrote nothing.
func countVenuesNamed(t *testing.T, name string) int {
	t.Helper()
	var count int
	require.NoError(t, db.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM venue WHERE venue_name = $1`, name).Scan(&count))
	return count
}

func TestPredictTeamSelectionHandler_AServedPredictionCarriesItsDateAndRun_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	fixture := seedPredictFixture(t)
	app := &App{mlClient: scriptedMLService(t, nil)}
	rec := httptest.NewRecorder()

	app.predictTeamSelectionHandler(rec, predictRequestFor(fixture))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var payload struct {
		RatingsThrough string `json:"ratings_through"`
		RunID          string `json:"run_id"`
		Team1          []struct {
			PlayerName string `json:"player_name"`
		} `json:"team1"`
		WinProbability struct {
			Team1 float64 `json:"team1"`
		} `json:"win_probability"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	assert.Equal(t, "2026-09-02", payload.RatingsThrough)
	assert.Equal(t, "20260906T083819Z-36689f80", payload.RunID)
	assert.Len(t, payload.Team1, 11)
	assert.InDelta(t, 0.6, payload.WinProbability.Team1, 1e-9)
}

// P1-3 end to end: a rating-ordered prediction carries a "why this player" block for every
// selected player, naming the pool the percentile is taken over, and no best alternative —
// nothing was maximised, so the card gets no win-model comparison to print.
func TestPredictTeamSelectionHandler_ARatingOrderedPredictionCarriesItsSelectionReasons_Integration(
	t *testing.T,
) {
	dbtest.SkipUnlessScratchDatabase(t)
	fixture := seedPredictFixture(t)
	app := &App{mlClient: scriptedMLService(t, nil)}
	rec := httptest.NewRecorder()

	app.predictTeamSelectionHandler(rec, predictRequestFor(fixture))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var payload struct {
		Selection struct {
			Optimised bool `json:"optimised"`
		} `json:"selection"`
		Team1 []struct {
			MarginalValue   *float64 `json:"marginal_value"`
			SelectionReason *struct {
				Roles            []string `json:"roles"`
				SelectionRating  float64  `json:"selection_rating"`
				RatingPercentile float64  `json:"rating_percentile"`
				PoolSize         int      `json:"pool_size"`
				BestAlternative  *struct {
					PlayerName string `json:"player_name"`
				} `json:"best_alternative"`
			} `json:"selection_reason"`
		} `json:"team1"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	assert.False(t, payload.Selection.Optimised)
	require.Len(t, payload.Team1, 11)
	for i := range payload.Team1 {
		player := payload.Team1[i]
		require.NotNil(t, player.SelectionReason, "every selected player carries his card's state")
		assert.Equal(t, []string{"bowling_option"}, player.SelectionReason.Roles)
		assert.Equal(t, 11, player.SelectionReason.PoolSize)
		assert.GreaterOrEqual(t, player.SelectionReason.RatingPercentile, 0.0)
		assert.Nil(t, player.SelectionReason.BestAlternative, "nothing was maximised, so nothing was compared")
		assert.Nil(t, player.MarginalValue)
	}
}

// seededPlayerIDs returns one side's player ids in a stable order: the fixture gives each
// side's eleven external ids beginning with its inning number.
func seededPlayerIDs(t *testing.T, externalIDPrefix string) []int64 {
	t.Helper()
	rows, err := db.Pool.Query(context.Background(),
		`SELECT id FROM player WHERE external_id LIKE $1 ORDER BY id`, externalIDPrefix+"%")
	require.NoError(t, err)
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		require.NoError(t, rows.Scan(&id))
		ids = append(ids, id)
	}
	require.NoError(t, rows.Err())
	return ids
}

func playPredictRequest(fixture predictFixture, xi1, xi2 []int64, extraTeam1 []int64) *http.Request {
	pinned1, _ := json.Marshal(xi1)
	pinned2, _ := json.Marshal(xi2)
	extras, _ := json.Marshal(extraTeam1)
	return jsonPredictRequest(fmt.Sprintf(
		`{"format":"TEST","team1_id":%d,"team2_id":%d,"match_date":%q,
		  "team1_xi":%s,"team2_xi":%s,"extra_team1":%s}`,
		fixture.team1ID, fixture.team2ID, fixture.matchDate, pinned1, pinned2, extras))
}

// Play mode end to end (P1-2): the eleven the caller built is the eleven that is scored,
// nothing is searched for, and the constraints it breaks are on the answer.
func TestPredictTeamSelectionHandler_APinnedElevenIsScoredAsSent_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	fixture := seedPredictFixture(t)
	xi1, xi2 := seededPlayerIDs(t, "1"), seededPlayerIDs(t, "2")
	require.Len(t, xi1, 11)
	app := &App{mlClient: scriptedMLService(t, nil)}
	rec := httptest.NewRecorder()

	// The eleven is sent in the caller's own order, with the last two swapped, and one
	// must-include id the eleven does not hold.
	pinned := append([]int64{}, xi1...)
	pinned[9], pinned[10] = pinned[10], pinned[9]
	app.predictTeamSelectionHandler(rec, playPredictRequest(fixture, pinned, xi2, []int64{xi1[0]}))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var payload struct {
		RunID     string `json:"run_id"`
		Selection struct {
			Objective string `json:"objective"`
			Optimised bool   `json:"optimised"`
			Note      string `json:"note"`
		} `json:"selection"`
		Team1 []struct {
			PlayerID        int64    `json:"player_id"`
			MarginalValue   *float64 `json:"marginal_value"`
			SelectionReason *struct {
				PoolSize int `json:"pool_size"`
			} `json:"selection_reason"`
		} `json:"team1"`
		Constraints *struct {
			MinBowlers int `json:"min_bowlers"`
			Team1      struct {
				Bowlers            int  `json:"bowlers"`
				Met                bool `json:"met"`
				MissingMustInclude []struct {
					PlayerID int64 `json:"player_id"`
				} `json:"missing_must_include"`
			} `json:"team1"`
			Team2 struct {
				Met bool `json:"met"`
			} `json:"team2"`
		} `json:"constraints"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	assert.Equal(t, "fixed", payload.Selection.Objective)
	assert.False(t, payload.Selection.Optimised, "nothing was searched for")
	assert.NotEmpty(t, payload.Selection.Note)
	assert.Equal(t, "20260906T083819Z-36689f80", payload.RunID, "a re-score carries its date like any prediction")
	require.Len(t, payload.Team1, 11)
	scored := make([]int64, 0, 11)
	for _, player := range payload.Team1 {
		scored = append(scored, player.PlayerID)
		assert.Nil(t, player.MarginalValue, "nothing was maximised, so no player has a margin")
		assert.Nil(t, player.SelectionReason, "the caller built this eleven, so there is no selection to explain")
	}
	assert.Equal(t, pinned, scored, "the eleven that was sent is the eleven that came back, in order")
	require.NotNil(t, payload.Constraints)
	assert.False(t, payload.Constraints.Team1.Met, "a broken constraint is reported, never repaired")
	assert.Equal(t, 4, payload.Constraints.Team1.Bowlers)
	require.Len(t, payload.Constraints.Team1.MissingMustInclude, 1)
	assert.Equal(t, xi1[0], payload.Constraints.Team1.MissingMustInclude[0].PlayerID)
	assert.True(t, payload.Constraints.Team2.Met)
}

func TestPredictTeamSelectionHandler_APinnedElevenShortOfPlayersIsRefused_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	fixture := seedPredictFixture(t)
	xi1, xi2 := seededPlayerIDs(t, "1"), seededPlayerIDs(t, "2")
	app := &App{mlClient: scriptedMLService(t, nil)}
	rec := httptest.NewRecorder()

	app.predictTeamSelectionHandler(rec, playPredictRequest(fixture, xi1[:10], xi2, nil))

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	body := decodeAPIError(t, rec)
	assert.Equal(t, "XI_INCOMPLETE", body.Code)
	assert.NotContains(t, rec.Body.String(), "win_probability", "a refusal carries no number")
}

// A pinned id that names nobody is refused rather than dropped: the ten who resolved are
// not the eleven that was sent, and an answer for them would say nothing about it.
//
// A pinned id that names a player of *another* club is honoured, deliberately: a pinned
// player joins the pool the way a must-include id does, and a caller naming a signing the
// database has not seen play for this club yet is the case that field exists for (D-12).
func TestPredictTeamSelectionHandler_APinnedIdThatNamesNobodyIsRefused_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	fixture := seedPredictFixture(t)
	xi1, xi2 := seededPlayerIDs(t, "1"), seededPlayerIDs(t, "2")
	app := &App{mlClient: scriptedMLService(t, nil)}
	rec := httptest.NewRecorder()

	const noSuchPlayer int64 = 999999
	pinned := append(append([]int64{}, xi1[:10]...), noSuchPlayer)
	app.predictTeamSelectionHandler(rec, playPredictRequest(fixture, pinned, xi2, nil))

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	body := decodeAPIError(t, rec)
	assert.Equal(t, "XI_PLAYER_UNKNOWN", body.Code)
	assert.Contains(t, body.Message, "999999")
}

func TestPredictTeamSelectionHandler_StaleRatingsAreRefusedWith503RatingsStale_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	fixture := seedPredictFixture(t)
	app := &App{mlClient: scriptedMLService(t, staleRatingsRefusal)}
	rec := httptest.NewRecorder()

	app.predictTeamSelectionHandler(rec, predictRequestFor(fixture))

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	body := decodeAPIError(t, rec)
	assert.Equal(t, "RATINGS_STALE", body.Code)
	assert.Contains(t, body.Message, "2026-09-02")
	assert.Contains(t, body.Message, "limit 14")
	assert.Contains(t, body.Hint, "retrain")
	assert.NotContains(t, rec.Body.String(), "win_probability", "a refusal carries no number")
}

// GO-07 end to end: a TEST request that names the toss is answered at that batting order,
// and the answer says so.
//
// TEST has no innings length, so this path never reaches the simulator -- the headline is
// the display model's and the per-player numbers are L2-B's quantiles, and both read the
// batting order. Until this fix the toss stopped at go-app's request structs, so the
// prediction was the reading averaged over both orders and the response said the format was
// to blame. On the served run the two readings are 0.04 apart in TEST, and up to 0.14.
func TestPredictTeamSelectionHandler_ANamedTossIsReadAndTheAnswerSaysSo_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)

	testCases := []struct {
		name        string
		tossField   string
		wantReading string
		wantNote    bool
	}{
		{
			name:        "no toss is the marginalised reading",
			tossField:   "",
			wantReading: "marginalised",
			wantNote:    false,
		},
		{
			name:        "team1 bats first is the toss-aware reading",
			tossField:   `,"team1_bats_first":true`,
			wantReading: "toss_aware",
			wantNote:    true,
		},
		{
			name:        "team2 bats first is the toss-aware reading too",
			tossField:   `,"team1_bats_first":false`,
			wantReading: "toss_aware",
			wantNote:    true,
		},
	}

	for i := range testCases {
		testCase := testCases[i]
		t.Run(testCase.name, func(t *testing.T) {
			fixture := seedPredictFixture(t)
			app := &App{mlClient: scriptedMLService(t, nil)}
			rec := httptest.NewRecorder()
			request := jsonPredictRequest(fmt.Sprintf(
				`{"format":"TEST","team1_id":%d,"team2_id":%d,"match_date":%q%s}`,
				fixture.team1ID, fixture.team2ID, fixture.matchDate, testCase.tossField))

			app.predictTeamSelectionHandler(rec, request)

			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			var payload struct {
				Toss struct {
					Team1BatsFirst *bool  `json:"team1_bats_first"`
					Reading        string `json:"reading"`
					Note           string `json:"note"`
				} `json:"toss"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
			assert.Equal(t, testCase.wantReading, payload.Toss.Reading)
			assert.Equal(t, testCase.wantNote, payload.Toss.Note != "")
			assert.NotContains(t, payload.Toss.Note, "no innings length",
				"the format is not why anything here is toss-blind")
		})
	}
}

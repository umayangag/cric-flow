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
	"testing"

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
		player_status_event, player_status, player_biography,
		ball_event, match_player, batting_data, bowling_data,
		fielding_data, fielding_event, match_inning, match, player, opposition RESTART IDENTITY`))

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
			playerID := insertPlayerRow(ctx, t, fmt.Sprintf("%d%02x", inning+1, i), fmt.Sprintf("Player %d-%d", inning+1, i))
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

// scriptedMLService answers the calls a TEST prediction makes -- /xi/optimize for each side,
// /xi/predict-win and /performance/predict -- every answer stamped with the same run and date. With
// `refusal` set, /xi/optimize answers that instead, which is where H-11 refuses first.
func scriptedMLService(t *testing.T, refusal *mlServiceError) *MLClient {
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
			PoolPlayerIDs  []string `json:"pool_player_ids"`
			Team1PlayerIDs []string `json:"team1_player_ids"`
			Team2PlayerIDs []string `json:"team2_player_ids"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		switch r.URL.Path {
		case "/xi/optimize":
			selected, err := json.Marshal(body.PoolPlayerIDs[:11])
			require.NoError(t, err)
			_, _ = fmt.Fprintf(w, `{"selected_player_ids": %s, "objective": "ratings", "optimised": false,
				"evaluations": 0, "improved_over_seed": 0, "unknown_player_ids": [], "marginal_values": {}, %s}`,
				selected, stamp)
		case "/xi/predict-win":
			_, _ = fmt.Fprintf(w, `{"team1_win_probability": 0.6, "objective_probability": 0.55, %s}`, stamp)
		case "/performance/predict":
			lines := make([]string, 0, 22)
			for _, id := range append(body.Team1PlayerIDs, body.Team2PlayerIDs...) {
				lines = append(lines, fmt.Sprintf(`{"player_id": %q, "side": 1, "p_bats": 1, "p_bowls": 0.5,
					"runs": {"q10": 3, "median": 26, "q90": 71}, "balls_faced": {"q10": 8, "median": 44, "q90": 110},
					"runs_conceded": {"q10": 10, "median": 33, "q90": 60},
					"wickets": {"expected": 1.4, "p0": 0.3, "p1": 0.4, "p2_plus": 0.3}, "catches_expected": 0.5}`, id))
			}
			_, _ = fmt.Fprintf(w, `{"players": [%s], "innings_marginalised": true, "unknown_player_ids": [], %s}`,
				strings.Join(lines, ","), stamp)
		default:
			t.Errorf("unexpected ml-service call %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return &MLClient{BaseURL: srv.URL, HTTP: srv.Client()}
}

func predictRequestFor(fixture predictFixture) *http.Request {
	return jsonPredictRequest(fmt.Sprintf(
		`{"format":"TEST","team1_id":%d,"team2_id":%d,"match_date":%q}`,
		fixture.team1ID, fixture.team2ID, fixture.matchDate))
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

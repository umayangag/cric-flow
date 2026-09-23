package db

import (
	"context"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/db/connection"
	"github.com/umayangag/cric-flow/go-app/internal/db/dbtest"
	"github.com/umayangag/cric-flow/go-app/internal/trackrecord"
)

// queryCountingTracer counts every Query, QueryRow and Exec pgx sends to the server on
// connections built from it, so a test can assert on round trips directly rather than
// trust that a refactor did what it claims (GO-14).
type queryCountingTracer struct {
	count atomic.Int64
}

func (tr *queryCountingTracer) TraceQueryStart(
	ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData,
) context.Context {
	tr.count.Add(1)
	return ctx
}

func (tr *queryCountingTracer) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// countingPool opens a second connection to the same database Pool already points at,
// with a query-counting tracer attached. pgxpool only accepts a tracer at construction, so
// counting round trips against the process's own Pool means building a second pool for
// exactly that and swapping it in for the test.
func countingPool(t *testing.T) (*pgxpool.Pool, *queryCountingTracer) {
	t.Helper()
	dsn := connection.BuildDSN(
		envOr("POSTGRES_USER", "postgres"), envOr("POSTGRES_PASSWORD", "postgres"),
		envOr("POSTGRES_HOST", "localhost"), envOr("POSTGRES_PORT", "5432"),
		envOr("POSTGRES_DB", "cricket_flow_test"), envOr("POSTGRES_SSLMODE", "disable"),
	)
	cfg, err := pgxpool.ParseConfig(dsn)
	require.NoError(t, err)
	tracer := &queryCountingTracer{}
	cfg.ConnConfig.Tracer = tracer
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool, tracer
}

// GO-14: fill used to run two queries per match it had just found, inside a loop over
// every match FindMatches returned. FindMatches' own doc comment already names the case
// that multiplies: "a double-header it cannot tell apart" -- two matches for one fixture,
// which paid two round trips for the second match beyond what the first one cost.
//
// This proves both halves of the fix. The round-trip count for a two-match fixture is a
// fixed 3 -- the find, plus fillAll's two batched queries -- not the 5 a per-match loop
// would still cost. And the batching does not cross-attribute: each match keeps its own
// innings totals and its own fielded eleven, which is the failure mode a bug in fillAll's
// match_id keying would produce.
func TestMatchLookup_FindMatches_ADoubleHeaderIsOneBatchedRoundTrip_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	ctx := context.Background()
	pool, err := Connect(ctx)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, RunMigrations(ctx, migrationsDir()))
	require.NoError(t, Exec(ctx, `TRUNCATE TABLE
		auction_player, auction_venue, auction_likely_xi,
		auction_opposition_player, auction_opposition, auction,
		issued_prediction,
		player_status_event, player_status, player_biography,
		ball_event_wicket, ball_event, match_player, batting_data, bowling_data,
		fielding_data, fielding_event, match_inning, match, player, opposition RESTART IDENTITY`))

	formatID, err := GetOrCreateMatchFormat(ctx, "TEST")
	require.NoError(t, err)
	var team1, team2 int64
	require.NoError(t, Pool.QueryRow(ctx,
		`INSERT INTO opposition (opposition_name, gender) VALUES ('Testland', 'male') RETURNING id`).
		Scan(&team1))
	require.NoError(t, Pool.QueryRow(ctx,
		`INSERT INTO opposition (opposition_name, gender) VALUES ('Otherland', 'male') RETURNING id`).
		Scan(&team2))
	matchDate := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	p101 := insertPlayer(ctx, t, "p-101", "First Match Team1 Player")
	p201 := insertPlayer(ctx, t, "p-201", "First Match Team2 Player")
	p301 := insertPlayer(ctx, t, "p-301", "Second Match Team1 Player")
	p401 := insertPlayer(ctx, t, "p-401", "Second Match Team2 Player")

	insertMatch := func(matchID int64, team1Runs, team2Runs int, fielders map[int64]int64) {
		require.NoError(t, Exec(ctx,
			`INSERT INTO match (match_id, format_id, match_date, original_match_type, gender)
			 VALUES ($1, $2, $3, 'TEST', 'male')`, matchID, formatID, matchDate))
		require.NoError(t, Exec(ctx,
			`INSERT INTO match_inning
			   (match_id, inning_number, batting_team_opposition_id, bowling_team_opposition_id, runs_scored)
			 VALUES ($1, 1, $2, $3, $4), ($1, 2, $3, $2, $5)`,
			matchID, team1, team2, team1Runs, team2Runs))
		for playerID, side := range fielders {
			require.NoError(t, Exec(ctx,
				`INSERT INTO match_player (match_id, player_id, opposition_id) VALUES ($1, $2, $3)`,
				matchID, playerID, side))
		}
	}
	// Two matches, same day, same two sides: a double-header.
	insertMatch(9101, 300, 200, map[int64]int64{p101: team1, p201: team2})
	insertMatch(9102, 150, 140, map[int64]int64{p301: team1, p401: team2})

	counting, tracer := countingPool(t)
	originalPool := Pool
	Pool = counting
	t.Cleanup(func() { Pool = originalPool })

	lookup := NewMatchLookup()
	matches, err := lookup.FindMatches(ctx, trackrecord.Fixture{
		Format: "TEST", Gender: "male", MatchDate: matchDate, Team1: team1, Team2: team2,
	})

	require.NoError(t, err)
	require.Len(t, matches, 2, "both matches on the day are a double-header, both returned")
	assert.EqualValues(t, 3, tracer.count.Load(),
		"the find, plus fillAll's two batched queries -- not two more round trips per extra match")

	byID := map[int64]trackrecord.PlayedMatch{}
	for _, m := range matches {
		byID[m.MatchID] = m
	}
	first, second := byID[9101], byID[9102]
	require.Len(t, first.Innings, 2)
	assert.Equal(t, 300, first.Innings[0].Runs, "9101's own runs, not 9102's")
	assert.Equal(t, map[int64]int64{p101: team1, p201: team2}, first.FieldedPlayers,
		"9101's own eleven, not mixed with the other match")
	require.Len(t, second.Innings, 2)
	assert.Equal(t, 150, second.Innings[0].Runs, "9102's own runs, not 9101's")
	assert.Equal(t, map[int64]int64{p301: team1, p401: team2}, second.FieldedPlayers,
		"9102's own eleven, not mixed with the other match")
}

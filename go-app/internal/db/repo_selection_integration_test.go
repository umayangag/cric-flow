package db

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/availability"
	"github.com/umayangag/cric-flow/go-app/internal/db/dbtest"
)

// The recency window and the retirement ledger against a real database (D-12).
//
// These are integration tests because the two things worth pinning here are SQL: the
// half-open window `[since, cutoff)`, and a promotion that reaches `player.is_retired`,
// `player_status` and `player_status_event` together or not at all. A mock over the
// connection pool would assert that this file's own strings were sent, which is what the
// dead `is_retired = 0` predicate would have passed for years.
//
// They truncate. Run them against a scratch database, never one holding an import:
// `make -C go-app test-db`, which points POSTGRES_DB at one. dbtest.SkipUnlessScratchDatabase
// refuses to run them against the working database for that reason.

// poolFixture builds one club, one format and four players with known last-played dates:
// recent (inside any window), stale (three years ago, plus one appearance exactly on the
// window's first day), ancient (a decade ago) and onCutoff, whose only appearance is the
// cutoff match itself.
type poolFixture struct {
	ctx      context.Context
	clubID   int64
	formatID int64
	recent   int64
	stale    int64
	ancient  int64
	onCutoff int64
	cutoff   time.Time
}

func setUpPoolFixture(t *testing.T) poolFixture {
	t.Helper()
	ctx := context.Background()
	pool, err := Connect(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })
	require.NoError(t, RunMigrations(ctx, migrationsDir()))
	require.NoError(t, Exec(ctx, `TRUNCATE TABLE
		player_status_event, player_status, player_biography,
		ball_event, match_player, batting_data, bowling_data,
		fielding_data, fielding_event, match_inning, match, player, opposition RESTART IDENTITY`))

	formatID, err := GetOrCreateMatchFormat(ctx, "ODI")
	require.NoError(t, err)

	fixture := poolFixture{
		ctx:      ctx,
		formatID: formatID,
		cutoff:   time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC),
	}
	require.NoError(t, Pool.QueryRow(ctx,
		`INSERT INTO opposition (opposition_name, gender) VALUES ('Testland', 'male') RETURNING id`).
		Scan(&fixture.clubID))

	fixture.recent = insertPlayer(ctx, t, "rec-1", "Recent Player")
	fixture.stale = insertPlayer(ctx, t, "sta-1", "Stale Player")
	fixture.ancient = insertPlayer(ctx, t, "anc-1", "Ancient Player")
	fixture.onCutoff = insertPlayer(ctx, t, "cut-1", "Cutoff Day Player")

	insertAppearance(ctx, t, fixture, appearance{matchID: 1001, playerID: fixture.recent, on: "2026-06-01"})
	insertAppearance(ctx, t, fixture, appearance{matchID: 1002, playerID: fixture.stale, on: "2023-09-10"})
	insertAppearance(ctx, t, fixture, appearance{matchID: 1003, playerID: fixture.ancient, on: "2016-01-01"})
	// Exactly on the twelve-month boundary: the window is closed at its start, so this
	// player is in.
	insertAppearance(ctx, t, fixture, appearance{matchID: 1004, playerID: fixture.stale, on: "2025-09-10"})
	// Exactly on the cutoff day: the window is half-open there, because the cutoff is the
	// match being predicted and a prediction may not read its own team sheet.
	insertAppearance(ctx, t, fixture, appearance{matchID: 1005, playerID: fixture.onCutoff, on: "2026-09-10"})
	return fixture
}

func insertPlayer(ctx context.Context, t *testing.T, externalID, name string) int64 {
	t.Helper()
	var id int64
	require.NoError(t, Pool.QueryRow(ctx,
		`INSERT INTO player (external_id, player_name, is_wicket_keeper, is_retired)
		 VALUES ($1, $2, 0, 0) RETURNING id`, externalID, name).Scan(&id))
	return id
}

// appearance is one player's batting appearance for the club in one match.
//
// The match id is given rather than generated because that is how the schema works:
// `match.match_id` is the id Cricsheet names the file with, assigned by the importer, and
// the column has neither a default nor a sequence behind it. A fixture that omits it is
// rejected by the not-null constraint, which is what kept this file from ever running.
type appearance struct {
	matchID  int64
	playerID int64
	on       string
}

// insertAppearance records one batting appearance for the club on that date.
func insertAppearance(ctx context.Context, t *testing.T, fixture poolFixture, a appearance) {
	t.Helper()
	require.NoError(t, Exec(ctx,
		`INSERT INTO match (match_id, format_id, match_date, original_match_type)
		 VALUES ($1, $2, $3, 'ODI')`, a.matchID, fixture.formatID, a.on))
	require.NoError(t, Exec(ctx,
		`INSERT INTO match_inning
		   (match_id, inning_number, batting_team_opposition_id, bowling_team_opposition_id)
		 VALUES ($1, 1, $2, $2)`, a.matchID, fixture.clubID))
	require.NoError(t, Exec(ctx,
		`INSERT INTO batting_data (match_id, inning_number, player_id) VALUES ($1, 1, $2)`,
		a.matchID, a.playerID))
}

// TestListPlayerPoolByOpposition_WindowIsHalfOpenAtTheCutoff_Integration pins the window
// arithmetic where it matters: against the database, at the boundary day.
func TestListPlayerPoolByOpposition_WindowIsHalfOpenAtTheCutoff_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	fixture := setUpPoolFixture(t)

	windowed, err := ListPlayerPoolByOpposition(fixture.ctx, PoolQuery{
		FormatCode:   "ODI",
		OppositionID: fixture.clubID,
		Cutoff:       fixture.cutoff,
		Since:        availability.WindowStart(fixture.cutoff, 12),
		ApplyLedger:  true,
	})
	require.NoError(t, err)
	allTime, err := ListPlayerPoolByOpposition(fixture.ctx, PoolQuery{
		FormatCode:   "ODI",
		OppositionID: fixture.clubID,
		Cutoff:       fixture.cutoff,
		ApplyLedger:  true,
	})
	require.NoError(t, err)

	assert.ElementsMatch(t,
		[]int64{fixture.recent, fixture.stale}, playerIDsOf(windowed.Players),
		"the window's first day is inside it; a decade ago is not, and the cutoff day is not")
	assert.ElementsMatch(t,
		[]int64{fixture.recent, fixture.stale, fixture.ancient}, playerIDsOf(allTime.Players),
		"an all-time pool is the one D-12 replaced, and is still available on request; "+
			"it is unbounded at the start and still half-open at the cutoff")
	assert.NotContains(t, playerIDsOf(allTime.Players), fixture.onCutoff,
		"the cutoff match's own team sheet is not evidence about who was available for it")
}

// TestListPlayerPoolByOpposition_LastPlayedIsTheMostRecentAppearance_Integration keeps the
// date the candidate list shows honest: it is the latest appearance before the cutoff, not
// the first one the union happened to return.
func TestListPlayerPoolByOpposition_LastPlayedIsTheMostRecentAppearance_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	fixture := setUpPoolFixture(t)

	pool, err := ListPlayerPoolByOpposition(fixture.ctx, PoolQuery{
		FormatCode:   "ODI",
		OppositionID: fixture.clubID,
		Cutoff:       fixture.cutoff,
		ApplyLedger:  true,
	})
	require.NoError(t, err)

	assert.Equal(t, "2025-09-10",
		rowFor(t, pool.Players, fixture.stale).LastPlayed.Format(time.DateOnly),
		"two appearances, and the later one is the one shown")
}

// TestListPlayerPoolByOpposition_ExtraIdsBypassEveryFilter_Integration keeps the escape
// hatch open: a caller naming a player by id is better evidence about availability than
// any window or ledger this repository holds.
func TestListPlayerPoolByOpposition_ExtraIdsBypassEveryFilter_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	fixture := setUpPoolFixture(t)
	require.NoError(t, Exec(fixture.ctx,
		`UPDATE player SET is_retired = 1 WHERE id = $1`, fixture.ancient))

	pool, err := ListPlayerPoolByOpposition(fixture.ctx, PoolQuery{
		FormatCode:     "ODI",
		OppositionID:   fixture.clubID,
		Cutoff:         fixture.cutoff,
		Since:          availability.WindowStart(fixture.cutoff, 12),
		ExtraPlayerIDs: []int64{fixture.ancient},
		ApplyLedger:    true,
	})
	require.NoError(t, err)

	assert.Contains(t, playerIDsOf(pool.Players), fixture.ancient)
	assert.Empty(t, pool.Excluded, "a player the caller insisted on is not an exclusion")
}

// TestPlayerStatusStore_PromotionAndDemotionAreOneStateChange_Integration is the ledger's
// whole rule against the database: a corroborated claim raises the stored fact and records
// which criterion allowed it, and withdrawing the claim lowers the fact and records that.
func TestPlayerStatusStore_PromotionAndDemotionAreOneStateChange_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	fixture := setUpPoolFixture(t)
	store := NewPlayerStatusStore()
	ledger := availability.NewLedger(store, availability.Criteria(nil))
	now := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)

	promoted, err := ledger.Flag(fixture.ctx, availability.DefaultActor, fixture.ancient, "ODI", now)
	require.NoError(t, err)
	claimOnly, err := ledger.Flag(fixture.ctx, availability.DefaultActor, fixture.recent, "ODI", now)
	require.NoError(t, err)

	assert.True(t, promoted.Flag.Promoted(), "a decade away is corroborated by inactivity")
	assert.Equal(t, availability.CriterionInactivity, promoted.Flag.Criterion)
	assert.False(t, claimOnly.Flag.Promoted(), "a player who played in June has not retired")
	assert.Equal(t, 1, retiredFlagOf(fixture.ctx, t, fixture.ancient))
	assert.Equal(t, 0, retiredFlagOf(fixture.ctx, t, fixture.recent),
		"a claim nobody corroborated is not a fact about the player")

	pool, err := ListPlayerPoolByOpposition(fixture.ctx, PoolQuery{
		FormatCode:   "ODI",
		OppositionID: fixture.clubID,
		Cutoff:       fixture.cutoff,
		ApplyLedger:  true,
		Flags:        mustFlags(fixture.ctx, t, ledger),
	})
	require.NoError(t, err)
	assert.ElementsMatch(t, []int64{fixture.stale}, playerIDsOf(pool.Players))
	assert.Len(t, pool.Excluded, 2, "both are excluded, and both are visible with a reason")

	_, existed, err := ledger.Unflag(fixture.ctx, availability.DefaultActor, fixture.ancient)
	require.NoError(t, err)
	assert.True(t, existed)
	assert.Equal(t, 0, retiredFlagOf(fixture.ctx, t, fixture.ancient), "un-flagging demotes the fact")
	assert.Equal(t, []string{"flagged", "promoted", "unflagged", "demoted"},
		statusEventsOf(fixture.ctx, t, fixture.ancient),
		"every change to the claim and to the fact is recorded, in order")
}

// TestListPlayerPoolByOpposition_ABacktestSeesNoLedger_Integration is H-19 in SQL: a
// retirement recorded today is not evidence about who was available at a past cutoff.
func TestListPlayerPoolByOpposition_ABacktestSeesNoLedger_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	fixture := setUpPoolFixture(t)
	require.NoError(t, Exec(fixture.ctx,
		`UPDATE player SET is_retired = 1 WHERE id = $1`, fixture.ancient))

	pool, err := ListPlayerPoolByOpposition(fixture.ctx, PoolQuery{
		FormatCode:   "ODI",
		OppositionID: fixture.clubID,
		Cutoff:       fixture.cutoff,
		ApplyLedger:  false,
	})
	require.NoError(t, err)

	assert.Contains(t, playerIDsOf(pool.Players), fixture.ancient)
	assert.Empty(t, pool.Excluded)
}

// rowFor returns the pool row for one player, failing the test when the pool does not
// hold him. Looking the row up rather than scanning past it is what stops an assertion
// about a row from passing because the row was never there.
func rowFor(t *testing.T, rows []PlayerPoolRow, playerID int64) PlayerPoolRow {
	t.Helper()
	for i := range rows {
		if rows[i].PlayerID == playerID {
			return rows[i]
		}
	}
	require.FailNowf(t, "player not in pool", "player %d", playerID)
	return PlayerPoolRow{}
}

func playerIDsOf(rows []PlayerPoolRow) []int64 {
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.PlayerID)
	}
	return ids
}

func retiredFlagOf(ctx context.Context, t *testing.T, playerID int64) int {
	t.Helper()
	var retired int
	require.NoError(t, Pool.QueryRow(ctx,
		`SELECT is_retired FROM player WHERE id = $1`, playerID).Scan(&retired))
	return retired
}

func statusEventsOf(ctx context.Context, t *testing.T, playerID int64) []string {
	t.Helper()
	rows, err := Pool.Query(ctx,
		`SELECT event FROM player_status_event WHERE player_id = $1 ORDER BY id`, playerID)
	require.NoError(t, err)
	defer rows.Close()
	var events []string
	for rows.Next() {
		var event string
		require.NoError(t, rows.Scan(&event))
		events = append(events, event)
	}
	require.NoError(t, rows.Err())
	return events
}

func mustFlags(ctx context.Context, t *testing.T, ledger *availability.Ledger) map[int64]availability.Flag {
	t.Helper()
	flags, err := ledger.Flags(ctx, availability.DefaultActor)
	require.NoError(t, err)
	return flags
}

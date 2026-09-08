package db

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/biography"
	"github.com/umayangag/cric-flow/go-app/internal/db/dbtest"
)

// connectAndMigrateForBiography brings up a migrated database and empties the tables the
// coverage query reads, so each case starts from a known state.
func connectAndMigrateForBiography(t *testing.T) context.Context {
	t.Helper()
	ctx := context.Background()
	pool, err := Connect(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })
	require.NoError(t, RunMigrations(ctx, migrationsDir()))
	require.NoError(t, Exec(ctx, `TRUNCATE TABLE
		auction_player, auction_venue, auction,
		issued_prediction,
		player_biography, player_status, player_status_event,
		ball_event, match_player, batting_data, bowling_data, fielding_data,
		fielding_event, match_inning, match, player, opposition RESTART IDENTITY`))
	return ctx
}

// seedTwoPlayersInOneODI creates the smallest world the coverage query can be asked
// about: two players fielded in one men's ODI.
func seedTwoPlayersInOneODI(ctx context.Context, t *testing.T) (int64, int64) {
	t.Helper()
	covered, _, err := GetOrCreatePlayer(ctx, "aaa11111", "A Covered", "2024-01-01")
	require.NoError(t, err)
	missing, _, err := GetOrCreatePlayer(ctx, "bbb22222", "B Missing", "2024-01-01")
	require.NoError(t, err)

	require.NoError(t, Exec(ctx,
		`INSERT INTO opposition (id, opposition_name, gender) VALUES (1, 'Testland', 'male')`))
	require.NoError(t, Exec(ctx,
		`INSERT INTO match (match_id, format_id, match_date, original_match_type, gender)
		 VALUES (1, 2, DATE '2024-01-01', 'ODI', 'male')`))
	require.NoError(t, Exec(ctx,
		`INSERT INTO match_player (match_id, player_id, opposition_id) VALUES (1, $1, 1), (1, $2, 1)`,
		covered, missing))
	return covered, missing
}

// TestPlayerBiographyStore_CoverageWeightsByAppearances_Integration is the measurement the
// X-1b gate reads: the share of *appearances* a fact covers, and the gap listed by how
// much closing it would be worth.
func TestPlayerBiographyStore_CoverageWeightsByAppearances_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	ctx := connectAndMigrateForBiography(t)
	covered, missing := seedTwoPlayersInOneODI(ctx, t)
	born := time.Date(1990, 4, 1, 0, 0, 0, 0, time.UTC)
	store := NewPlayerBiographyStore()

	require.NoError(t, store.UpsertBiographies(ctx, []biography.Record{
		{
			PlayerID: covered, CricinfoID: "111", WikidataQID: "Q1", BirthDate: &born,
			BowlingStyle: biography.StyleLegSpin, BowlingStyleRaw: "leg break",
			Source: biography.SourceWikidata, License: biography.SourceLicense,
			FetchedAt: time.Now().UTC(),
		},
		{
			// Asked and not found: a measured miss, which is why it gets a row at all.
			PlayerID: missing, CricinfoID: "222",
			Source: biography.SourceWikidata, License: biography.SourceLicense,
			FetchedAt: time.Now().UTC(),
		},
	}))
	coverage, err := store.Coverage(ctx)

	require.NoError(t, err)
	require.Len(t, coverage.Rows, 1)
	row := coverage.Rows[0]
	assert.Equal(t, "ODI", row.Format)
	assert.EqualValues(t, 2, row.Appearances)
	assert.EqualValues(t, 2, row.Attempted, "both were looked up")
	assert.EqualValues(t, 1, row.Matched)
	assert.EqualValues(t, 1, row.BirthDate)
	assert.EqualValues(t, 1, row.BowlingStyle)
	assert.EqualValues(t, 2, coverage.Total.Players)
	assert.EqualValues(t, 1, coverage.Total.MatchedPlayers)
	require.NotNil(t, coverage.LastFetchedAt)

	require.Len(t, coverage.Unmatched, 1)
	assert.Equal(t, "B Missing", coverage.Unmatched[0].Name)
	assert.Equal(t, "222", coverage.Unmatched[0].CricinfoID)
	assert.True(t, coverage.Unmatched[0].Attempted)
}

// TestPlayerBiographyStore_UpsertReplacesTheRow_Integration: a second pass over the same
// player corrects the row rather than failing or duplicating it, which is what makes the
// backfill safe to re-run.
func TestPlayerBiographyStore_UpsertReplacesTheRow_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	ctx := connectAndMigrateForBiography(t)
	covered, _ := seedTwoPlayersInOneODI(ctx, t)
	store := NewPlayerBiographyStore()
	now := time.Now().UTC()

	require.NoError(t, store.UpsertBiographies(ctx, []biography.Record{
		{PlayerID: covered, Source: biography.SourceWikidata, FetchedAt: now},
	}))
	require.NoError(t, store.UpsertBiographies(ctx, []biography.Record{
		{PlayerID: covered, WikidataQID: "Q9", Source: biography.SourceOverride, FetchedAt: now},
	}))
	coverage, err := store.Coverage(ctx)

	require.NoError(t, err)
	require.Len(t, coverage.Rows, 1)
	assert.EqualValues(t, 1, coverage.Rows[0].Matched,
		"the second pass corrected the row rather than adding one")
}

// TestPlayerBiographyStore_ListPlayers_Integration returns the registry identifiers and
// the appearance weights the whole report is expressed in.
func TestPlayerBiographyStore_ListPlayers_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	ctx := connectAndMigrateForBiography(t)
	seedTwoPlayersInOneODI(ctx, t)

	players, err := NewPlayerBiographyStore().ListPlayers(ctx)

	require.NoError(t, err)
	require.Len(t, players, 2)
	assert.Equal(t, "aaa11111", players[0].ExternalID)
	assert.EqualValues(t, 1, players[0].Appearances)
}

// TestPlayerBiographyStore_UpsertNothingIsNotAnError keeps an empty registry from failing
// a run that had nothing to write.
func TestPlayerBiographyStore_UpsertNothingIsNotAnError_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	ctx := connectAndMigrateForBiography(t)

	require.NoError(t, NewPlayerBiographyStore().UpsertBiographies(ctx, nil))
}

// TestPlayerStatusStore_RetirementEvidenceReadsTheBiography_Integration is the D-12 seam
// X-1a closes: the age and career-end criteria reported themselves unavailable because
// nothing supplied these two fields, and now something does.
func TestPlayerStatusStore_RetirementEvidenceReadsTheBiography_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	ctx := connectAndMigrateForBiography(t)
	covered, missing := seedTwoPlayersInOneODI(ctx, t)
	born := time.Date(1980, 4, 1, 0, 0, 0, 0, time.UTC)
	ended := time.Date(2019, 6, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, NewPlayerBiographyStore().UpsertBiographies(ctx, []biography.Record{{
		PlayerID: covered, WikidataQID: "Q1", BirthDate: &born, CareerEndDate: &ended,
		Source: biography.SourceWikidata, FetchedAt: time.Now().UTC(),
	}}))

	withBiography, err := NewPlayerStatusStore().RetirementEvidence(ctx, covered)
	require.NoError(t, err)
	without, err := NewPlayerStatusStore().RetirementEvidence(ctx, missing)
	require.NoError(t, err)

	assert.Equal(t, born, withBiography.BirthDate.UTC())
	assert.Equal(t, ended, withBiography.CareerEnd.UTC())
	assert.True(t, without.BirthDate.IsZero(),
		"a player the pass matched nothing for still reports the criteria unavailable")
	assert.True(t, without.CareerEnd.IsZero())
}

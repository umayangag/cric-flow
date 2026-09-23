package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/db/dbtest"
)

// connectAndMigrateForVenues brings up a migrated database and empties `venue` together
// with everything that points at it, so each case starts from a known state.
func connectAndMigrateForVenues(t *testing.T) context.Context {
	t.Helper()
	ctx := context.Background()
	pool, err := Connect(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })
	require.NoError(t, RunMigrations(ctx, migrationsDir()))
	require.NoError(t, Exec(ctx, `TRUNCATE TABLE
		auction_player, auction_venue, auction_likely_xi,
		auction_opposition_player, auction_opposition, auction,
		issued_prediction,
		ball_event_wicket, ball_event, match_player, batting_data, bowling_data, fielding_data, fielding_event,
		match_inning, match, venue_weather, venue RESTART IDENTITY`))
	return ctx
}

func countVenueRows(ctx context.Context, t *testing.T) int {
	t.Helper()
	var rows int
	require.NoError(t, Pool.QueryRow(ctx, `SELECT count(*) FROM venue`).Scan(&rows))
	return rows
}

// TestGetOrCreateVenue_TwoSpellingsOfOneGroundAreOneRow_Integration is the defect itself.
//
// `normalized_name` and its unique index were in the schema from 0001 and nothing wrote
// them, so the importer upserted on `venue_name` and every punctuation variant of a ground
// became its own venue with its own familiarity and scoring history (IMPORT-08).
func TestGetOrCreateVenue_TwoSpellingsOfOneGroundAreOneRow_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	ctx := connectAndMigrateForVenues(t)

	first, err := GetOrCreateVenue(ctx, "M Chinnaswamy Stadium", "Bengaluru")
	require.NoError(t, err)
	second, err := GetOrCreateVenue(ctx, "M.Chinnaswamy Stadium", "Bengaluru")
	require.NoError(t, err)

	assert.Equal(t, first, second, "one ground under two spellings is one venue")
	assert.Equal(t, 1, countVenueRows(ctx, t))

	var storedName, storedKey string
	require.NoError(t, Pool.QueryRow(ctx,
		`SELECT venue_name, normalized_name FROM venue WHERE id = $1`, first).
		Scan(&storedName, &storedKey))
	assert.Equal(t, "M Chinnaswamy Stadium", storedName, "the row keeps the spelling that created it")
	assert.Equal(t, "m chinnaswamy stadium", storedKey)
}

// TestGetOrCreateVenue_GroundsSharingANameStemStayApart_Integration guards the fold's
// conservatism at the database. "County Ground" is nine different grounds in the archive,
// two of whose trailing parts (Bristol, Derby) are cities, so a rule that keyed on the
// first comma-part would collapse nine venues into one.
func TestGetOrCreateVenue_GroundsSharingANameStemStayApart_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	ctx := connectAndMigrateForVenues(t)

	countyGrounds := []string{
		"County Ground",
		"County Ground, Bristol",
		"County Ground, Chelmsford",
		"County Ground, Derby",
		"County Ground, Hove",
		"County Ground, New Road",
		"County Ground, New Road, Worcester",
		"County Ground, Northampton",
		"County Ground, Taunton",
	}
	ids := make(map[int64]struct{}, len(countyGrounds))
	for i := range countyGrounds {
		id, err := GetOrCreateVenue(ctx, countyGrounds[i], "")
		require.NoError(t, err)
		ids[id] = struct{}{}
	}

	assert.Len(t, ids, len(countyGrounds), "nine grounds must stay nine venues")
	assert.Equal(t, len(countyGrounds), countVenueRows(ctx, t))
}

// TestGetOrCreateVenue_CityIsTheFirstTheArchiveNames_Integration pins the other half of
// the city change: the city belongs in `venue.city`, filled once and never rewritten, not
// standing in for a ground's name.
func TestGetOrCreateVenue_CityIsTheFirstTheArchiveNames_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	ctx := connectAndMigrateForVenues(t)

	id, err := GetOrCreateVenue(ctx, "M Chinnaswamy Stadium", "")
	require.NoError(t, err)
	_, err = GetOrCreateVenue(ctx, "M.Chinnaswamy Stadium", "Bengaluru")
	require.NoError(t, err)
	_, err = GetOrCreateVenue(ctx, "M Chinnaswamy Stadium", "Bangalore")
	require.NoError(t, err)

	var city *string
	require.NoError(t, Pool.QueryRow(ctx, `SELECT city FROM venue WHERE id = $1`, id).Scan(&city))
	require.NotNil(t, city, "a ground first seen without a city takes one from a later file")
	assert.Equal(t, "Bengaluru", *city, "and no later file rewrites it")
}

// TestGetOrCreateVenue_ANameWithNoIdentityIsRefused_Integration keeps the NOT NULL key
// honest: a name that folds to nothing cannot become a row, because the row would have no
// identity for anything to resolve against.
func TestGetOrCreateVenue_ANameWithNoIdentityIsRefused_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	ctx := connectAndMigrateForVenues(t)

	_, err := GetOrCreateVenue(ctx, "...", "Hambantota")

	require.Error(t, err)
	assert.Equal(t, 0, countVenueRows(ctx, t))
}

// TestFindVenueIDByName_ResolvesAnotherSpellingAndCreatesNothing_Integration holds the
// two halves of GO-08 and IMPORT-08 together: the prediction path resolves on the same
// folded key the importer upserts on, and it still never brings a venue into existence.
func TestFindVenueIDByName_ResolvesAnotherSpellingAndCreatesNothing_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	ctx := connectAndMigrateForVenues(t)

	created, err := GetOrCreateVenue(ctx, "M Chinnaswamy Stadium", "Bengaluru")
	require.NoError(t, err)

	found, held, err := FindVenueIDByName(ctx, "M.Chinnaswamy Stadium")
	require.NoError(t, err)
	assert.True(t, held, "another spelling of a held ground is that ground")
	assert.Equal(t, created, found)

	_, held, err = FindVenueIDByName(ctx, "Ground That Is Not In This Database")
	require.NoError(t, err)
	assert.False(t, held)
	assert.Equal(t, 1, countVenueRows(ctx, t), "resolution must never create a venue")
}

// TestGetVenuesByQuery_EveryNameItOffersResolves_Integration pins the invariant the picker
// and the prediction path have to share: a string listed by /api/options/venues must
// resolve, or choosing it from the dropdown answers VENUE_NOT_FOUND.
//
// The list used to offer a trimmed display_name in place of venue_name while resolution
// matched venue_name. Nothing has ever written display_name, so the mismatch
// never fired -- it waited for the first writer. This test writes one.
func TestGetVenuesByQuery_EveryNameItOffersResolves_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	ctx := connectAndMigrateForVenues(t)

	plain, err := GetOrCreateVenue(ctx, "Chinnaswamy Park", "Bengaluru")
	require.NoError(t, err)
	require.NoError(t, Exec(ctx,
		`UPDATE venue SET display_name = $1 WHERE id = $2`, "Chinnaswamy Park (renamed)", plain))
	_, err = GetOrCreateVenue(ctx, "Chinnaswamy Oval", "Bengaluru")
	require.NoError(t, err)

	offered, err := GetVenuesByQuery(ctx, "Chinnaswamy")
	require.NoError(t, err)
	require.Len(t, offered, 2)

	for i := range offered {
		_, held, findErr := FindVenueIDByName(ctx, offered[i])
		require.NoError(t, findErr)
		assert.Truef(t, held, "the picker offered %q but the prediction path cannot resolve it", offered[i])
	}
}

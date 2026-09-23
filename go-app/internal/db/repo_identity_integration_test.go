package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/db/dbtest"
)

// connectAndMigrateForIdentity brings up a migrated database and empties the two identity
// tables, so each case starts from a known state. Truncating them together satisfies the
// foreign keys that point at them -- and the list has to name *every* table referencing
// one in it, so the per-player tables D-12 and X-1a added are in it too. Postgres refuses
// the whole statement otherwise, which is how those two additions were noticed.
func connectAndMigrateForIdentity(t *testing.T) context.Context {
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
		match_inning, match, player, opposition,
		player_status, player_status_event, player_biography RESTART IDENTITY`))
	return ctx
}

func TestGetOrCreatePlayer_TwoPeopleSharingANameAreTwoRows_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	ctx := connectAndMigrateForIdentity(t)

	westIndies, _, err := GetOrCreatePlayer(ctx, "92cf79a8", "SR Taylor", "2015-06-01")
	require.NoError(t, err)
	england, _, err := GetOrCreatePlayer(ctx, "a9231c3f", "SR Taylor", "2019-06-01")
	require.NoError(t, err)

	assert.NotEqual(t, westIndies, england, "two registry identifiers are two careers")
}

func TestGetOrCreatePlayer_OnePersonUnderTwoSpellingsIsOneRow_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	ctx := connectAndMigrateForIdentity(t)

	first, _, err := GetOrCreatePlayer(ctx, "f3a18a0c", "NR Sciver", "2022-06-01")
	require.NoError(t, err)
	second, storedName, err := GetOrCreatePlayer(ctx, "f3a18a0c", "NR Sciver-Brunt", "2024-07-01")
	require.NoError(t, err)

	assert.Equal(t, first, second)
	assert.Equal(t, "NR Sciver", storedName, "the row keeps its name until the import settles it")
}

func TestGetOrCreatePlayer_WithoutARegistryEntryFallsBackToOneRowPerName_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	ctx := connectAndMigrateForIdentity(t)

	first, _, err := GetOrCreatePlayer(ctx, "", "Unregistered Player", "2024-01-01")
	require.NoError(t, err)
	again, _, err := GetOrCreatePlayer(ctx, "", "Unregistered Player", "2024-02-01")
	require.NoError(t, err)
	identified, _, err := GetOrCreatePlayer(ctx, "0dc00542", "Unregistered Player", "2024-03-01")
	require.NoError(t, err)

	assert.Equal(t, first, again, "the fallback still keys by name")
	assert.NotEqual(t, first, identified, "a name-keyed row never absorbs an identified person")
}

// TestGetOrCreatePlayer_NameKnownUnderAnotherFilesRegistry_ReusesTheIdentifiedRow pins
// IMPORT-15: a name missing from one file's registry must not duplicate a person who is
// already known under a real Cricsheet identifier from another file. Before this fix the
// name-only fallback keyed on (player_name, external_id IS NULL) alone, which never looked
// at a same-named row that already carried an identifier.
func TestGetOrCreatePlayer_NameKnownUnderAnotherFilesRegistry_ReusesTheIdentifiedRow(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	ctx := connectAndMigrateForIdentity(t)

	identified, _, err := GetOrCreatePlayer(ctx, "92cf79a8", "SR Taylor", "2015-06-01")
	require.NoError(t, err)

	// A later file's registry has no entry for this name, so the importer resolves it
	// with an empty external id -- the fallback that, unfixed, would have created a
	// second "SR Taylor" row with a null external_id.
	fallback, _, err := GetOrCreatePlayer(ctx, "", "SR Taylor", "2019-06-01")
	require.NoError(t, err)

	assert.Equal(t, identified, fallback,
		"the registry-less lookup must find the already-identified row, not duplicate it")
}

func TestUpdatePlayerDisplayNames_RenamesToTheMostRecentSpelling_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	ctx := connectAndMigrateForIdentity(t)
	id, _, err := GetOrCreatePlayer(ctx, "f3a18a0c", "NR Sciver", "2022-06-01")
	require.NoError(t, err)

	require.NoError(t, UpdatePlayerDisplayNames(ctx, []PlayerDisplayName{
		{PlayerID: id, Name: "NR Sciver-Brunt", NameAsOf: "2024-07-01"},
	}))

	var name string
	require.NoError(t, Pool.QueryRow(ctx, "SELECT player_name FROM player WHERE id = $1", id).Scan(&name))
	assert.Equal(t, "NR Sciver-Brunt", name)
}

func TestUpdatePlayerDisplayNames_AnOlderMatchNeverRevertsAName_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	ctx := connectAndMigrateForIdentity(t)
	id, _, err := GetOrCreatePlayer(ctx, "f3a18a0c", "NR Sciver-Brunt", "2024-07-01")
	require.NoError(t, err)

	require.NoError(t, UpdatePlayerDisplayNames(ctx, []PlayerDisplayName{
		{PlayerID: id, Name: "NR Sciver", NameAsOf: "2022-06-01"},
	}))

	var name string
	require.NoError(t, Pool.QueryRow(ctx, "SELECT player_name FROM player WHERE id = $1", id).Scan(&name))
	assert.Equal(t, "NR Sciver-Brunt", name)
}

func TestGetOrCreateOpposition_SameNameDifferentGenderAreTwoTeams_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	ctx := connectAndMigrateForIdentity(t)

	mens, err := GetOrCreateOpposition(ctx, "Australia", "male")
	require.NoError(t, err)
	womens, err := GetOrCreateOpposition(ctx, "Australia", "female")
	require.NoError(t, err)
	againMens, err := GetOrCreateOpposition(ctx, "Australia", "male")
	require.NoError(t, err)

	assert.NotEqual(t, mens, womens)
	assert.Equal(t, mens, againMens)
}

func TestResolveTeamSide_UnknownTeamIsAnError_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	ctx := connectAndMigrateForIdentity(t)
	_, err := GetOrCreateOpposition(ctx, "Australia", "male")
	require.NoError(t, err)

	// The team row exists but has played nothing, so no format can claim it.
	_, err = ResolveTeamSide(ctx, TeamRef{Name: "Australia"}, "T20")

	require.ErrorIs(t, err, ErrOppositionNotFound)
}

func TestUpdatePlayerDisplayNames_LeavesTheNameKeyedFallbackAlone_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	ctx := connectAndMigrateForIdentity(t)
	id, _, err := GetOrCreatePlayer(ctx, "", "Unregistered Player", "2024-01-01")
	require.NoError(t, err)

	// For a fallback row the name is the identity, so renaming it would change who the
	// row refers to -- and two such rows renamed alike would collide on the partial
	// unique index that keeps one row per unidentified name.
	require.NoError(t, UpdatePlayerDisplayNames(ctx, []PlayerDisplayName{
		{PlayerID: id, Name: "Someone Else", NameAsOf: "2025-01-01"},
	}))

	var name string
	require.NoError(t, Pool.QueryRow(ctx, "SELECT player_name FROM player WHERE id = $1", id).Scan(&name))
	assert.Equal(t, "Unregistered Player", name)
}

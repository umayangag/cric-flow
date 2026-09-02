package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// connectAndMigrateForIdentity brings up a migrated database and empties the two identity
// tables, so each case starts from a known state. Truncating both together satisfies the
// foreign keys that point at them.
func connectAndMigrateForIdentity(t *testing.T) context.Context {
	t.Helper()
	ctx := context.Background()
	pool, err := Connect(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })
	require.NoError(t, RunMigrations(ctx, migrationsDir()))
	require.NoError(t, Exec(ctx, `TRUNCATE TABLE
		ball_event, match_player, batting_data, bowling_data, fielding_data, fielding_event,
		match_inning, match, player, opposition RESTART IDENTITY`))
	return ctx
}

func TestGetOrCreatePlayer_TwoPeopleSharingANameAreTwoRows_Integration(t *testing.T) {
	if !guardIntegration(t) {
		t.Skip("integration test skipped; set RUN_DB_TESTS=1 to run")
	}
	ctx := connectAndMigrateForIdentity(t)

	westIndies, _, err := GetOrCreatePlayer(ctx, "92cf79a8", "SR Taylor", "2015-06-01")
	require.NoError(t, err)
	england, _, err := GetOrCreatePlayer(ctx, "a9231c3f", "SR Taylor", "2019-06-01")
	require.NoError(t, err)

	assert.NotEqual(t, westIndies, england, "two registry identifiers are two careers")
}

func TestGetOrCreatePlayer_OnePersonUnderTwoSpellingsIsOneRow_Integration(t *testing.T) {
	if !guardIntegration(t) {
		t.Skip("integration test skipped; set RUN_DB_TESTS=1 to run")
	}
	ctx := connectAndMigrateForIdentity(t)

	first, _, err := GetOrCreatePlayer(ctx, "f3a18a0c", "NR Sciver", "2022-06-01")
	require.NoError(t, err)
	second, storedName, err := GetOrCreatePlayer(ctx, "f3a18a0c", "NR Sciver-Brunt", "2024-07-01")
	require.NoError(t, err)

	assert.Equal(t, first, second)
	assert.Equal(t, "NR Sciver", storedName, "the row keeps its name until the import settles it")
}

func TestGetOrCreatePlayer_WithoutARegistryEntryFallsBackToOneRowPerName_Integration(t *testing.T) {
	if !guardIntegration(t) {
		t.Skip("integration test skipped; set RUN_DB_TESTS=1 to run")
	}
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

func TestUpdatePlayerDisplayNames_RenamesToTheMostRecentSpelling_Integration(t *testing.T) {
	if !guardIntegration(t) {
		t.Skip("integration test skipped; set RUN_DB_TESTS=1 to run")
	}
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
	if !guardIntegration(t) {
		t.Skip("integration test skipped; set RUN_DB_TESTS=1 to run")
	}
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
	if !guardIntegration(t) {
		t.Skip("integration test skipped; set RUN_DB_TESTS=1 to run")
	}
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
	if !guardIntegration(t) {
		t.Skip("integration test skipped; set RUN_DB_TESTS=1 to run")
	}
	ctx := connectAndMigrateForIdentity(t)
	_, err := GetOrCreateOpposition(ctx, "Australia", "male")
	require.NoError(t, err)

	// The team row exists but has played nothing, so no format can claim it.
	_, err = ResolveTeamSide(ctx, TeamRef{Name: "Australia"}, "T20")

	require.ErrorIs(t, err, ErrOppositionNotFound)
}

func TestUpdatePlayerDisplayNames_LeavesTheNameKeyedFallbackAlone_Integration(t *testing.T) {
	if !guardIntegration(t) {
		t.Skip("integration test skipped; set RUN_DB_TESTS=1 to run")
	}
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

func TestApplyTeamLineage_LinksASupersededClubToItsCurrentRow_Integration(t *testing.T) {
	if !guardIntegration(t) {
		t.Skip("integration test skipped; set RUN_DB_TESTS=1 to run")
	}
	ctx := connectAndMigrateForIdentity(t)
	old, err := GetOrCreateOpposition(ctx, "Royal Challengers Bangalore", "male")
	require.NoError(t, err)
	current, err := GetOrCreateOpposition(ctx, "Royal Challengers Bengaluru", "male")
	require.NoError(t, err)

	linked, err := ApplyTeamLineage(ctx, []TeamRename{
		{FromName: "Royal Challengers Bangalore", ToName: "Royal Challengers Bengaluru", Gender: "male"},
	})

	require.NoError(t, err)
	assert.Equal(t, 1, linked)
	var canonical *int64
	require.NoError(t, Pool.QueryRow(ctx, "SELECT canonical_id FROM opposition WHERE id = $1", old).Scan(&canonical))
	require.NotNil(t, canonical)
	assert.Equal(t, current, *canonical)
	// The club's current row is not itself superseded: COALESCE(canonical_id, id) has to
	// terminate, and a self-reference would make "has this club renamed?" two questions.
	require.NoError(
		t,
		Pool.QueryRow(ctx, "SELECT canonical_id FROM opposition WHERE id = $1", current).Scan(&canonical),
	)
	assert.Nil(t, canonical)
}

func TestApplyTeamLineage_LeavesTheOtherGenderAlone_Integration(t *testing.T) {
	if !guardIntegration(t) {
		t.Skip("integration test skipped; set RUN_DB_TESTS=1 to run")
	}
	ctx := connectAndMigrateForIdentity(t)
	womensOld, err := GetOrCreateOpposition(ctx, "Lightning", "female")
	require.NoError(t, err)
	_, err = GetOrCreateOpposition(ctx, "The Blaze", "female")
	require.NoError(t, err)
	mensSameName, err := GetOrCreateOpposition(ctx, "Lightning", "male")
	require.NoError(t, err)

	linked, err := ApplyTeamLineage(ctx, []TeamRename{
		{FromName: "Lightning", ToName: "The Blaze", Gender: "female"},
	})

	require.NoError(t, err)
	assert.Equal(t, 1, linked, "a club may rename its women's side and not its men's")
	var canonical *int64
	require.NoError(
		t,
		Pool.QueryRow(ctx, "SELECT canonical_id FROM opposition WHERE id = $1", mensSameName).Scan(&canonical),
	)
	assert.Nil(t, canonical)
	require.NoError(
		t,
		Pool.QueryRow(ctx, "SELECT canonical_id FROM opposition WHERE id = $1", womensOld).Scan(&canonical),
	)
	assert.NotNil(t, canonical)
}

func TestApplyTeamLineage_IsIdempotentAndSkipsARenameThisDatasetDoesNotHave_Integration(t *testing.T) {
	if !guardIntegration(t) {
		t.Skip("integration test skipped; set RUN_DB_TESTS=1 to run")
	}
	ctx := connectAndMigrateForIdentity(t)
	_, err := GetOrCreateOpposition(ctx, "Kings XI Punjab", "male")
	require.NoError(t, err)
	_, err = GetOrCreateOpposition(ctx, "Punjab Kings", "male")
	require.NoError(t, err)
	renames := []TeamRename{
		{FromName: "Kings XI Punjab", ToName: "Punjab Kings", Gender: "male"},
		// A rename whose successor has not played yet: the mapping describes cricket, not
		// this particular import, so it is reported and skipped rather than failing.
		{FromName: "Deccan Chargers", ToName: "Sunrisers Hyderabad", Gender: "male"},
	}

	first, err := ApplyTeamLineage(ctx, renames)
	require.NoError(t, err)
	second, err := ApplyTeamLineage(ctx, renames)
	require.NoError(t, err)

	assert.Equal(t, 1, first)
	assert.Zero(t, second, "the count is rows changed, so a second run over the same data changes none")
}

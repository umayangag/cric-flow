package db

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/db/dbtest"
)

// renameRoyalChallengers is the mapping entry these cases exercise; it is a real one, from
// configs/team_lineage.json.
var renameRoyalChallengers = TeamRename{
	FromName: "Royal Challengers Bangalore",
	ToName:   "Royal Challengers Bengaluru",
	Gender:   "male",
}

func TestApplyTeamLineage_LinksASupersededClubToItsCurrentRow_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	ctx := connectAndMigrateForIdentity(t)
	old, err := GetOrCreateOpposition(ctx, renameRoyalChallengers.FromName, renameRoyalChallengers.Gender)
	require.NoError(t, err)
	current, err := GetOrCreateOpposition(ctx, renameRoyalChallengers.ToName, renameRoyalChallengers.Gender)
	require.NoError(t, err)

	report, err := ApplyTeamLineage(ctx, []TeamRename{renameRoyalChallengers})

	require.NoError(t, err)
	require.Len(t, report.Renames, 1)
	assert.Equal(t, TeamLineageLinked, report.Renames[0].State)
	assert.True(t, report.Renames[0].Changed, "this run wrote the link")
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
	dbtest.SkipUnlessScratchDatabase(t)
	ctx := connectAndMigrateForIdentity(t)
	womensOld, err := GetOrCreateOpposition(ctx, "Lightning", "female")
	require.NoError(t, err)
	_, err = GetOrCreateOpposition(ctx, "The Blaze", "female")
	require.NoError(t, err)
	mensSameName, err := GetOrCreateOpposition(ctx, "Lightning", "male")
	require.NoError(t, err)

	report, err := ApplyTeamLineage(ctx, []TeamRename{
		{FromName: "Lightning", ToName: "The Blaze", Gender: "female"},
	})

	require.NoError(t, err)
	assert.Equal(t, 1, report.Count(TeamLineageLinked), "a club may rename its women's side and not its men's")
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

// TestApplyTeamLineage_TellsAlreadyLinkedFromAbsent_Integration pins the distinction the
// row count could not make. A second pass over the same data changes nothing, and a rename
// whose successor has not played yet changes nothing, and before IMPORT-07 both were the
// integer 0 -- the same answer a pass that never ran would have given.
func TestApplyTeamLineage_TellsAlreadyLinkedFromAbsent_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
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

	assert.Equal(t, 1, first.Changed(), "the first pass writes the one link this dataset supports")
	assert.Zero(t, second.Changed(), "a second pass over unchanged data writes nothing")
	assert.Equal(t, 1, second.Count(TeamLineageLinked), "and says so by reporting the link it found")
	assert.Equal(
		t,
		[]string{"Deccan Chargers -> Sunrisers Hyderabad (male)"},
		second.InState(TeamLineageAbsent),
		"the rename this dataset has only one side of is named, not silently counted as zero",
	)
}

// TestTeamLineageCoverage_SeesTheArchiveAnImportNeverSettled_Integration is the read-only
// question /ops/status asks. Both clubs are in the archive and the link was never written,
// which is exactly what an import that aborted before settlement leaves behind, and what
// nothing in the system reported before.
func TestTeamLineageCoverage_SeesTheArchiveAnImportNeverSettled_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	ctx := connectAndMigrateForIdentity(t)
	_, err := GetOrCreateOpposition(ctx, renameRoyalChallengers.FromName, renameRoyalChallengers.Gender)
	require.NoError(t, err)
	_, err = GetOrCreateOpposition(ctx, renameRoyalChallengers.ToName, renameRoyalChallengers.Gender)
	require.NoError(t, err)

	before, err := TeamLineageCoverage(ctx, []TeamRename{renameRoyalChallengers})
	require.NoError(t, err)
	_, err = ApplyTeamLineage(ctx, []TeamRename{renameRoyalChallengers})
	require.NoError(t, err)
	after, err := TeamLineageCoverage(ctx, []TeamRename{renameRoyalChallengers})
	require.NoError(t, err)

	assert.Equal(t, 1, before.Count(TeamLineageUnlinked), "two rows of one club, no link between them")
	assert.Equal(t, 1, after.Count(TeamLineageLinked))
	assert.Zero(t, after.Changed(), "coverage reads; it does not link")
}

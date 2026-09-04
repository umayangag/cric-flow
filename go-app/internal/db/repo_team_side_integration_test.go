package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/db/dbtest"
	"github.com/umayangag/cric-flow/go-app/internal/teams"
)

// playFixture records one match between two sides in a format, which is all the side queries
// read: they answer "who has played this format", and an opposition row that has played
// nothing is not an option.
func playFixture(ctx context.Context, t *testing.T, matchID int64, formatCode string, home, away int64) {
	t.Helper()
	formatID, err := GetOrCreateMatchFormat(ctx, formatCode)
	require.NoError(t, err)
	require.NoError(t, Exec(ctx, `
		INSERT INTO match (match_id, format_id, match_date, original_match_type)
		VALUES ($1, $2, DATE '2026-01-01', $3)`, matchID, formatID, formatCode))
	require.NoError(t, Exec(ctx, `
		INSERT INTO match_inning (match_id, inning_number, batting_team_opposition_id, bowling_team_opposition_id)
		VALUES ($1, 1, $2, $3)`, matchID, home, away))
}

// bothIndias creates the two sides one name stands for, plus an opponent for each, and
// returns the men's and women's India ids. It is the shape D-10 is about: "India" in T20I
// names two teams, and 130 of the 394 names in the dataset are like this.
func bothIndias(ctx context.Context, t *testing.T) (mens, womens int64) {
	t.Helper()
	mens, err := GetOrCreateOpposition(ctx, "India", teams.GenderMale)
	require.NoError(t, err)
	womens, err = GetOrCreateOpposition(ctx, "India", teams.GenderFemale)
	require.NoError(t, err)
	australiaMen, err := GetOrCreateOpposition(ctx, "Australia", teams.GenderMale)
	require.NoError(t, err)
	australiaWomen, err := GetOrCreateOpposition(ctx, "Australia", teams.GenderFemale)
	require.NoError(t, err)

	playFixture(ctx, t, 900001, "T20I", mens, australiaMen)
	playFixture(ctx, t, 900002, "T20I", womens, australiaWomen)
	return mens, womens
}

func TestResolveTeamSide_ABareAmbiguousNameIsRefused_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	ctx := connectAndMigrateForIdentity(t)
	mens, womens := bothIndias(ctx, t)

	_, err := ResolveTeamSide(ctx, TeamRef{Name: "India"}, "T20I")

	var ambiguous *AmbiguousTeamNameError
	require.ErrorAs(t, err, &ambiguous, "a name that means two teams has no single answer")
	assert.ElementsMatch(t, []int64{mens, womens}, []int64{
		ambiguous.Candidates[0].ClubID, ambiguous.Candidates[1].ClubID,
	}, "the error carries both candidates, because picking one is the caller's next move")
	assert.ElementsMatch(t, []string{"India (men)", "India (women)"}, ambiguous.CandidateLabels())
}

func TestResolveTeamSide_AnExplicitSideIsHonoured_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	ctx := connectAndMigrateForIdentity(t)
	mens, womens := bothIndias(ctx, t)

	byGender, err := ResolveTeamSide(ctx, TeamRef{Name: "India", Gender: teams.GenderFemale}, "T20I")
	require.NoError(t, err)
	byClubID, err := ResolveTeamSide(ctx, TeamRef{ClubID: mens}, "T20I")
	require.NoError(t, err)

	assert.Equal(t, womens, byGender.ClubID)
	assert.Equal(t, teams.GenderFemale, byGender.Gender)
	assert.Equal(t, mens, byClubID.ClubID)
	assert.Equal(t, "India (men)", byClubID.Label())
}

// A name is unambiguous when the format holds one side of it, and then it still resolves:
// the fix refuses doubt, not names.
func TestResolveTeamSide_AnUnambiguousNameStillResolves_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	ctx := connectAndMigrateForIdentity(t)
	scotland, err := GetOrCreateOpposition(ctx, "Scotland", teams.GenderMale)
	require.NoError(t, err)
	namibia, err := GetOrCreateOpposition(ctx, "Namibia", teams.GenderMale)
	require.NoError(t, err)
	playFixture(ctx, t, 900010, "T20I", scotland, namibia)

	side, err := ResolveTeamSide(ctx, TeamRef{Name: "Scotland"}, "T20I")

	require.NoError(t, err)
	assert.Equal(t, scotland, side.ClubID)
}

func TestResolveTeamSide_AGenderThatPlayedNothingIsNotFound_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	ctx := connectAndMigrateForIdentity(t)
	bothIndias(ctx, t)
	// Only the men's side plays TEST here.
	mens, err := GetOrCreateOpposition(ctx, "India", teams.GenderMale)
	require.NoError(t, err)
	england, err := GetOrCreateOpposition(ctx, "England", teams.GenderMale)
	require.NoError(t, err)
	playFixture(ctx, t, 900020, "TEST", mens, england)

	_, err = ResolveTeamSide(ctx, TeamRef{Name: "India", Gender: teams.GenderFemale}, "TEST")

	require.ErrorIs(t, err, ErrOppositionNotFound)
}

// A club that renamed is one club, and the club id names the row it plays under now.
func TestResolveTeamSide_ARetiredNameResolvesToTheClub_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	ctx := connectAndMigrateForIdentity(t)
	bangalore, err := GetOrCreateOpposition(ctx, "Royal Challengers Bangalore", teams.GenderMale)
	require.NoError(t, err)
	bengaluru, err := GetOrCreateOpposition(ctx, "Royal Challengers Bengaluru", teams.GenderMale)
	require.NoError(t, err)
	chennai, err := GetOrCreateOpposition(ctx, "Chennai Super Kings", teams.GenderMale)
	require.NoError(t, err)
	playFixture(ctx, t, 900030, "T20", bangalore, chennai)
	playFixture(ctx, t, 900031, "T20", bengaluru, chennai)
	linked, err := ApplyTeamLineage(ctx, []TeamRename{{
		FromName: "Royal Challengers Bangalore",
		ToName:   "Royal Challengers Bengaluru",
		Gender:   teams.GenderMale,
	}})
	require.NoError(t, err)
	require.Equal(t, 1, linked)

	side, err := ResolveTeamSide(ctx, TeamRef{Name: "Royal Challengers Bangalore"}, "T20")

	require.NoError(t, err)
	assert.Equal(t, bengaluru, side.ClubID, "the club, not the years before it renamed")
	assert.Equal(t, "Royal Challengers Bengaluru", side.Name)
}

func TestListTeamSidesForFormat_ListsEachSideOnce_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	ctx := connectAndMigrateForIdentity(t)
	bothIndias(ctx, t)

	sides, err := ListTeamSidesForFormat(ctx, "T20I")

	require.NoError(t, err)
	labels := make([]string, 0, len(sides))
	for _, side := range sides {
		labels = append(labels, side.Label())
	}
	assert.Equal(t, []string{
		"Australia (women)", "Australia (men)", "India (women)", "India (men)",
	}, labels, "one entry per side, so the picker can say which it means")
}

func TestListOpponentSidesForFormat_ListsWhoThisClubHasPlayed_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	ctx := connectAndMigrateForIdentity(t)
	mens, _ := bothIndias(ctx, t)

	opponents, err := ListOpponentSidesForFormat(ctx, "T20I", mens)

	require.NoError(t, err)
	require.Len(t, opponents, 1)
	assert.Equal(t, "Australia (men)", opponents[0].Label(),
		"the men's side has played the men's side; the cascade cannot offer a cross-gender fixture")
}

// The data check D-10 records: no club's lineage may join a men's row to a women's row.
//
// ApplyTeamLineage requires both sides of a rename to share a gender, so the importer cannot
// write one -- but the column itself carries no such constraint, and a link written by any
// other means would silently merge two teams' Elo, form and head-to-head. Measured against
// the live database on 2026-09-02: 0 of 10 lineage links bridge a gender.
func TestTeamLineageNeverBridgesTwoGenders_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	ctx := connectAndMigrateForIdentity(t)
	mensFrom, err := GetOrCreateOpposition(ctx, "Delhi Daredevils", teams.GenderMale)
	require.NoError(t, err)
	_, err = GetOrCreateOpposition(ctx, "Delhi Capitals", teams.GenderMale)
	require.NoError(t, err)
	womensTo, err := GetOrCreateOpposition(ctx, "Delhi Capitals", teams.GenderFemale)
	require.NoError(t, err)

	// A rename whose two sides are the same gender links; the same names across genders
	// must not, whichever way the mapping is written.
	linked, err := ApplyTeamLineage(ctx, []TeamRename{
		{FromName: "Delhi Daredevils", ToName: "Delhi Capitals", Gender: teams.GenderMale},
	})
	require.NoError(t, err)
	require.Equal(t, 1, linked)

	var bridging int
	require.NoError(t, Pool.QueryRow(ctx, `
		SELECT count(*)
		FROM opposition predecessor
		JOIN opposition successor ON successor.id = predecessor.canonical_id
		WHERE predecessor.gender IS DISTINCT FROM successor.gender`).Scan(&bridging))

	assert.Zero(t, bridging, "a lineage link across genders would merge two teams' history")
	assert.NotEqual(t, womensTo, mensFrom, "the women's row is a different team, not a rename target")
}

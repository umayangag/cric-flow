package predictteam

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/availability"
	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// One player, one side (GO-04).
//
// The fixture throughout is the ordinary franchise case: a twelve-month window, a player
// who moved clubs inside it, and two pools built independently from one player table.

// playedFor builds one candidate row with a last appearance for this side.
func playedFor(id int64, name string, lastPlayed time.Time) db.PlayerPoolRow {
	return db.PlayerPoolRow{
		PlayerID:   id,
		ExternalID: fmt.Sprintf("reg%d", id),
		PlayerName: name,
		LastPlayed: lastPlayed,
	}
}

// on2026 is a date inside every fixture's window; the year is the same throughout because
// what these tests turn on is which of two dates is the later one.
func on2026(month time.Month, day int) time.Time {
	return time.Date(2026, month, day, 0, 0, 0, 0, time.UTC)
}

// movedClubFixture is the two sides, both of which hold player 7, plus a settled player
// each so a pool is never emptied by the resolution itself.
func movedClubFixture(
	lastPlayedForTeam1, lastPlayedForTeam2 time.Time,
) (side1, side2 candidateSide, summary1, summary2 *PoolSummary) {
	summary1, summary2 = &PoolSummary{Size: 2}, &PoolSummary{Size: 2}
	rows1 := []db.PlayerPoolRow{
		playedFor(1, "Settled One", on2026(8, 1)),
		playedFor(7, "Moved Player", lastPlayedForTeam1),
	}
	rows2 := []db.PlayerPoolRow{
		playedFor(2, "Settled Two", on2026(8, 1)),
		playedFor(7, "Moved Player", lastPlayedForTeam2),
	}
	side1 = newCandidateSide(indiaMen, rows1, summary1)
	side2 = newCandidateSide(australiaMen, rows2, summary2)
	return side1, side2, summary1, summary2
}

func poolPlayerIDs(rows []db.PlayerPoolRow) []int64 {
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.PlayerID)
	}
	return ids
}

// TestResolveSharedCandidates_TheClubHePlayedForMoreRecentlyKeepsHim: the recency evidence
// the pool window itself is built on decides it, either way round.
func TestResolveSharedCandidates_TheClubHePlayedForMoreRecentlyKeepsHim(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name               string
		team1LastPlayed    time.Time
		team2LastPlayed    time.Time
		wantTeam1PlayerIDs []int64
		wantTeam2PlayerIDs []int64
		wantExcludedBy     string
	}{
		{
			name:               "team1 played him more recently",
			team1LastPlayed:    on2026(7, 19),
			team2LastPlayed:    on2026(2, 14),
			wantTeam1PlayerIDs: []int64{1, 7},
			wantTeam2PlayerIDs: []int64{2},
			wantExcludedBy:     "team2",
		},
		{
			name:               "team2 played him more recently",
			team1LastPlayed:    on2026(2, 14),
			team2LastPlayed:    on2026(7, 19),
			wantTeam1PlayerIDs: []int64{1},
			wantTeam2PlayerIDs: []int64{2, 7},
			wantExcludedBy:     "team1",
		},
		{
			name:               "an exact tie stays with team1, and says so",
			team1LastPlayed:    on2026(7, 19),
			team2LastPlayed:    on2026(7, 19),
			wantTeam1PlayerIDs: []int64{1, 7},
			wantTeam2PlayerIDs: []int64{2},
			wantExcludedBy:     "team2",
		},
		{
			name:               "no appearance either side stays with team1",
			team1LastPlayed:    time.Time{},
			team2LastPlayed:    time.Time{},
			wantTeam1PlayerIDs: []int64{1, 7},
			wantTeam2PlayerIDs: []int64{2},
			wantExcludedBy:     "team2",
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			side1, side2, summary1, summary2 := movedClubFixture(tc.team1LastPlayed, tc.team2LastPlayed)

			err := resolveSharedCandidates(&side1, &side2)

			require.NoError(t, err)
			assert.Equal(t, tc.wantTeam1PlayerIDs, poolPlayerIDs(side1.rows))
			assert.Equal(t, tc.wantTeam2PlayerIDs, poolPlayerIDs(side2.rows))
			lost, kept := lostAndKept(tc.wantExcludedBy, summary1, summary2)
			assertLostHim(t, lost)
			assertKeptEveryone(t, kept)
		})
	}
}

// lostAndKept names the two summaries by what the case expects of them.
func lostAndKept(excludedBy string, summary1, summary2 *PoolSummary) (lost, kept *PoolSummary) {
	if excludedBy == "team1" {
		return summary1, summary2
	}
	return summary2, summary1
}

// assertLostHim is the whole §8.7 statement about a side that lost a shared candidate: he
// is out of the pool, he is named in the summary, and the reason is beside him.
func assertLostHim(t *testing.T, summary *PoolSummary) {
	t.Helper()
	require.Len(t, summary.Excluded, 1, "the side that lost him says so (§8.7)")
	assert.Equal(t, int64(7), summary.Excluded[0].PlayerID)
	assert.Equal(t, "Moved Player", summary.Excluded[0].PlayerName)
	assert.Equal(t, availability.ReasonBothSides, summary.Excluded[0].Reason)
	assert.NotEmpty(t, summary.Excluded[0].Detail, "the reason names the side that kept him")
	assert.Equal(t, 1, summary.Size, "the summary reports the pool that was actually offered")
}

func assertKeptEveryone(t *testing.T, summary *PoolSummary) {
	t.Helper()
	assert.Empty(t, summary.Excluded, "the side that kept him excluded nobody")
	assert.Equal(t, 2, summary.Size)
}

// TestResolveSharedCandidates_TheDetailSaysWhichSideKeptHimAndWhy: a name struck through
// with no reason beside it is the silent filter §8.7 forbids.
func TestResolveSharedCandidates_TheDetailSaysWhichSideKeptHimAndWhy(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name            string
		team1LastPlayed time.Time
		team2LastPlayed time.Time
		wantDetail      string
	}{
		{
			name:            "a transfer names the later date",
			team1LastPlayed: on2026(7, 19),
			team2LastPlayed: on2026(2, 14),
			wantDetail:      "also a candidate for India (men), whom he played for more recently (2026-07-19)",
		},
		{
			name:            "a tie says it is a tie, and how to overrule it",
			team1LastPlayed: on2026(7, 19),
			team2LastPlayed: on2026(7, 19),
			wantDetail: "also a candidate for India (men), and he last played for both on 2026-07-19, " +
				"so India (men) keeps him; pick the candidates by hand to decide it yourself",
		},
		{
			name:            "no appearance at all says that too",
			team1LastPlayed: time.Time{},
			team2LastPlayed: time.Time{},
			wantDetail: "also a candidate for India (men), and neither side has an appearance to " +
				"separate them, so India (men) keeps him; pick the candidates by hand to decide it yourself",
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			side1, side2, _, summary2 := movedClubFixture(tc.team1LastPlayed, tc.team2LastPlayed)

			require.NoError(t, resolveSharedCandidates(&side1, &side2))

			require.Len(t, summary2.Excluded, 1)
			assert.Equal(t, tc.wantDetail, summary2.Excluded[0].Detail)
		})
	}
}

// TestResolveSharedCandidates_ACallerWhoNamedHimKeepsHimWhateverTheDatesSay: a must-include
// id is a lock (B-10) and a pinned eleven is the caller's own answer. Either outranks an
// appearance record.
func TestResolveSharedCandidates_ACallerWhoNamedHimKeepsHimWhateverTheDatesSay(t *testing.T) {
	t.Parallel()
	side1, side2, summary1, _ := movedClubFixture(on2026(2, 14), on2026(7, 19))
	// team1 named him although team2 played him five months later.
	side1.claimed = map[int64]bool{7: true}

	require.NoError(t, resolveSharedCandidates(&side1, &side2))

	assert.Equal(t, []int64{1, 7}, poolPlayerIDs(side1.rows))
	assert.Equal(t, []int64{2}, poolPlayerIDs(side2.rows))
	assert.Empty(t, summary1.Excluded)
}

// TestResolveSharedCandidates_BothSidesNamingHimIsRefusedNotResolved is P1-4's rule applied
// to the one case where the caller, not the window, put him on both sides.
func TestResolveSharedCandidates_BothSidesNamingHimIsRefusedNotResolved(t *testing.T) {
	t.Parallel()
	side1, side2, summary1, summary2 := movedClubFixture(on2026(2, 14), on2026(7, 19))
	side1.claimed = map[int64]bool{7: true}
	side2.claimed = map[int64]bool{7: true}

	err := resolveSharedCandidates(&side1, &side2)

	var shared *SharedPlayerError
	require.ErrorAs(t, err, &shared)
	assert.Equal(t, []SharedPlayer{{PlayerID: 7, PlayerName: "Moved Player"}}, shared.Players)
	assert.Contains(t, err.Error(), "nobody plays both elevens")
	assert.Contains(t, err.Error(), "Moved Player")
	assert.Empty(t, summary1.Excluded, "a refused fixture resolves nothing")
	assert.Empty(t, summary2.Excluded)
}

// TestResolveSharedCandidates_LeavesTwoDisjointPoolsAlone: the ordinary fixture pays
// nothing for this.
func TestResolveSharedCandidates_LeavesTwoDisjointPoolsAlone(t *testing.T) {
	t.Parallel()
	summary1, summary2 := &PoolSummary{Size: 2}, &PoolSummary{Size: 2}
	side1 := newCandidateSide(indiaMen, pool(1, 2), summary1)
	side2 := newCandidateSide(australiaMen, pool(3, 4), summary2)

	require.NoError(t, resolveSharedCandidates(&side1, &side2))

	assert.Equal(t, []int64{1, 2}, poolPlayerIDs(side1.rows))
	assert.Equal(t, []int64{3, 4}, poolPlayerIDs(side2.rows))
	assert.Empty(t, summary1.Excluded)
	assert.Empty(t, summary2.Excluded)
}

// TestResolveSharedCandidates_APlayerWithNoRegistryIdCollidesWithNobody: ml-service resolves
// players on the registry id, so a row without one cannot be the player on the other side.
func TestResolveSharedCandidates_APlayerWithNoRegistryIdCollidesWithNobody(t *testing.T) {
	t.Parallel()
	summary1, summary2 := &PoolSummary{Size: 1}, &PoolSummary{Size: 1}
	unregistered := func(id int64) []db.PlayerPoolRow {
		return []db.PlayerPoolRow{{PlayerID: id, PlayerName: "Unregistered"}}
	}
	side1 := newCandidateSide(indiaMen, unregistered(11), summary1)
	side2 := newCandidateSide(australiaMen, unregistered(12), summary2)

	require.NoError(t, resolveSharedCandidates(&side1, &side2))

	assert.Len(t, side1.rows, 1)
	assert.Len(t, side2.rows, 1)
	assert.Empty(t, summary1.Excluded)
}

// TestNewCandidateSide_ClaimsAreTheIdsTheCallerNamed: both ways of naming a player for a
// side count, which is what makes rule 1 and rule 2 the same rule.
func TestNewCandidateSide_ClaimsAreTheIdsTheCallerNamed(t *testing.T) {
	t.Parallel()

	side := newCandidateSide(indiaMen, nil, &PoolSummary{}, []int64{4}, []int64{7, 9})

	assert.Equal(t, map[int64]bool{4: true, 7: true, 9: true}, side.claimed)
}

// TestRefuseSharedSelection_AcceptsTwoDisjointElevens: the ordinary answer passes.
func TestRefuseSharedSelection_AcceptsTwoDisjointElevens(t *testing.T) {
	t.Parallel()

	err := refuseSharedSelection([]string{"k1", "k2"}, []string{"k3", "k4"})

	assert.NoError(t, err)
}

// TestRefuseSharedSelection_RefusesTwoElevensHoldingOnePlayer is the postcondition: made
// true by construction upstream, checked because what it protects is silent.
func TestRefuseSharedSelection_RefusesTwoElevensHoldingOnePlayer(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name      string
		team1Keys []string
		team2Keys []string
		wantErr   string
	}{
		{
			name:      "one shared player is refused by name",
			team1Keys: []string{"k1", "k2"},
			team2Keys: []string{"k2", "k4"},
			wantErr:   "both elevens hold the same player (registry id k2)",
		},
		{
			name:      "two shared players are both named",
			team1Keys: []string{"k1", "k2"},
			team2Keys: []string{"k1", "k2"},
			wantErr:   "registry id k1, k2",
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := refuseSharedSelection(tc.team1Keys, tc.team2Keys)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

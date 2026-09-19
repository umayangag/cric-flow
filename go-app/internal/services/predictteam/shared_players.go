package predictteam

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/availability"
	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// One player, one side (GO-04).
//
// Both pools are loaded independently, from the same `player` table, by club. A player who
// moved clubs inside the window is in both — franchise T20 with a twelve-month window is
// not an edge case, it is the ordinary case — and until this file nothing noticed. The
// consequences were all silent: alternating best response could select him for both sides,
// every per-registry-id merge (marginal values, selection reasons, performance rows) had
// one side's answer overwrite the other's, and the objective would have been asked about a
// match in which one man fields for both teams.
//
// **Why he is dropped from a side rather than the fixture refused.** The fixture is real:
// the two clubs do play, and one of them is the club he plays for now. Refusing it would
// make a legitimate franchise fixture unanswerable over a data artefact of the window.
// **Why it is not silent.** §8.7: a filter that removes a player says so where the answer
// is read. He leaves in the losing side's `PoolSummary.Excluded`, with the reason, his last
// appearance for that club, and the evidence — exactly as the retirement ledger's
// exclusions do, and just as reversible: the user can pick the candidates by hand.
//
// **Whose claim wins.** In order:
//
//  1. Named by the caller on both sides — pinned in both elevens, or must-included on both.
//     Refused, not resolved. The caller has said two contradictory things about one player
//     and correcting one of them silently is the substitution P1-4 and §8.7 forbid; a user
//     who hand-builds two elevens sharing a player is told, and chooses.
//  2. Named by the caller on exactly one side. He stays there whatever the dates say: a
//     must-include id is a lock (B-10) and a pinned eleven is the caller's own answer, and
//     both are better evidence about who he plays for than his appearance record.
//  3. Named on neither. The club he appeared for more recently keeps him (`LastPlayed`),
//     which is the same recency evidence the pool window itself is built on.
//
// Equal dates — including two sides that both entered him by id with no appearance at all —
// keep him with team1. That is arbitrary, and it is arbitrary on purpose: there is no
// evidence left to separate the two clubs, the choice is stated in the summary of the side
// that lost him, and picking the candidates by hand overrides it.

// SharedPlayer is one player both sides' callers named, for an error a user can act on.
type SharedPlayer struct {
	PlayerID   int64
	PlayerName string
}

// SharedPlayerError refuses a fixture whose caller put one player on both sides.
//
// Reached only where the caller named him on both sides; a player who is merely in both
// pools is resolved and reported, not refused.
type SharedPlayerError struct {
	Team1   string
	Team2   string
	Players []SharedPlayer
}

func (e *SharedPlayerError) Error() string {
	names := make([]string, 0, len(e.Players))
	for _, player := range e.Players {
		names = append(names, fmt.Sprintf("%s (id %d)", player.PlayerName, player.PlayerID))
	}
	return fmt.Sprintf("%s and %s both name %s, and nobody plays both elevens",
		e.Team1, e.Team2, strings.Join(names, ", "))
}

// candidateSide is one side's half of the shared-player resolution: who is in its pool, who
// the caller named for it, and the summary its exclusions are reported in.
type candidateSide struct {
	team    db.TeamSide
	rows    []db.PlayerPoolRow
	claimed map[int64]bool
	summary *PoolSummary
}

// newCandidateSide gathers one side's inputs. claimedIDs are the player ids the caller
// named for this side: must-include ids and, in Play mode, the pinned eleven.
func newCandidateSide(
	team db.TeamSide,
	rows []db.PlayerPoolRow,
	summary *PoolSummary,
	claimedIDs ...[]int64,
) candidateSide {
	claimed := make(map[int64]bool)
	for _, ids := range claimedIDs {
		for _, id := range ids {
			claimed[id] = true
		}
	}
	return candidateSide{team: team, rows: rows, claimed: claimed, summary: summary}
}

// resolveSharedCandidates removes from each side's pool the players the other side keeps,
// and records every removal in the summary the response carries.
//
// It returns the two pools to use. The caller's own naming is refused rather than resolved
// (rule 1 above); everything else leaves with a reason attached.
func resolveSharedCandidates(side1, side2 *candidateSide) error {
	byKey2 := rowsByRegistryID(side2.rows)
	var refused []SharedPlayer
	drop1 := map[int64]db.PlayerPoolRow{}
	drop2 := map[int64]db.PlayerPoolRow{}

	for _, row1 := range side1.rows {
		row2, shared := byKey2[row1.ExternalID]
		if !shared || row1.ExternalID == "" {
			continue
		}
		claimed1, claimed2 := side1.claimed[row1.PlayerID], side2.claimed[row2.PlayerID]
		if claimed1 && claimed2 {
			refused = append(refused, SharedPlayer{PlayerID: row1.PlayerID, PlayerName: row1.PlayerName})
			continue
		}
		if keepsPlayer(claimed1, claimed2, row1.LastPlayed, row2.LastPlayed) {
			drop2[row2.PlayerID] = row2
			side2.summary.addSharedExclusion(row2, side1.team, row1.LastPlayed, claimed1)
			continue
		}
		drop1[row1.PlayerID] = row1
		side1.summary.addSharedExclusion(row1, side2.team, row2.LastPlayed, claimed2)
	}

	if len(refused) > 0 {
		err := &SharedPlayerError{Team1: side1.team.Label(), Team2: side2.team.Label(), Players: refused}
		slog.Warn("predictteam.PredictTeams refused a fixture naming one player on both sides",
			slog.String("team1", side1.team.Label()),
			slog.String("team2", side2.team.Label()),
			slog.Any("err", err))
		return err
	}
	recordSharedDrops(side1, drop1)
	recordSharedDrops(side2, drop2)
	return nil
}

// keepsPlayer reports whether side1 keeps a player both pools hold.
//
// The caller's own naming outranks the appearance record on either side; with neither side
// naming him, the later appearance wins and an exact tie stays with side1.
func keepsPlayer(claimedBySide1, claimedBySide2 bool, lastPlayed1, lastPlayed2 time.Time) bool {
	if claimedBySide1 != claimedBySide2 {
		return claimedBySide1
	}
	return !lastPlayed1.Before(lastPlayed2)
}

// rowsByRegistryID indexes a pool by the identity ml-service resolves players on.
//
// The registry id is the key rather than this database's player id because it is what
// collides: it is what the rating state is keyed on, what both elevens are sent as, and
// what every per-player answer comes back under. A row with no registry id cannot be sent
// to ml-service at all, so it can collide with nothing and is left where it is.
func rowsByRegistryID(rows []db.PlayerPoolRow) map[string]db.PlayerPoolRow {
	byKey := make(map[string]db.PlayerPoolRow, len(rows))
	for _, row := range rows {
		if row.ExternalID == "" {
			continue
		}
		byKey[row.ExternalID] = row
	}
	return byKey
}

// withoutPlayers returns the pool minus the dropped rows, order preserved.
func withoutPlayers(rows []db.PlayerPoolRow, dropped map[int64]db.PlayerPoolRow) []db.PlayerPoolRow {
	if len(dropped) == 0 {
		return rows
	}
	kept := make([]db.PlayerPoolRow, 0, len(rows))
	for _, row := range rows {
		if _, isDropped := dropped[row.PlayerID]; isDropped {
			continue
		}
		kept = append(kept, row)
	}
	return kept
}

// recordSharedDrops applies one side's removals: the pool it will be selected from, the
// size the summary reports, and a log line per player.
func recordSharedDrops(side *candidateSide, dropped map[int64]db.PlayerPoolRow) {
	side.rows = withoutPlayers(side.rows, dropped)
	side.summary.Size = len(side.rows)
	for _, row := range dropped {
		slog.Info("predictteam.PredictTeams dropped a candidate the other side keeps",
			slog.String("team", side.team.Label()),
			slog.Int64("player_id", row.PlayerID),
			slog.String("player_name", row.PlayerName))
	}
}

// addSharedExclusion records, in the summary the response carries, a candidate this side
// lost to the other one and the evidence the decision was made on.
func (s *PoolSummary) addSharedExclusion(
	row db.PlayerPoolRow,
	keptBy db.TeamSide,
	keptByLastPlayed time.Time,
	keptByCallerNamed bool,
) {
	s.Excluded = append(s.Excluded, ExcludedCandidate{
		PlayerID:   row.PlayerID,
		PlayerName: row.PlayerName,
		LastPlayed: formatDate(row.LastPlayed),
		Reason:     availability.ReasonBothSides,
		Detail:     sharedExclusionDetail(row, keptBy, keptByLastPlayed, keptByCallerNamed),
	})
}

// sharedExclusionDetail spells the decision in the terms it was made in, so a reader can
// tell a transfer from a tie-break and knows what to change if the answer is wrong.
func sharedExclusionDetail(
	row db.PlayerPoolRow,
	keptBy db.TeamSide,
	keptByLastPlayed time.Time,
	keptByCallerNamed bool,
) string {
	label := keptBy.Label()
	if keptByCallerNamed {
		return fmt.Sprintf("also a candidate for %s, where you named him; no player is on both sides", label)
	}
	switch {
	case keptByLastPlayed.IsZero() && row.LastPlayed.IsZero():
		return fmt.Sprintf(
			"also a candidate for %s, and neither side has an appearance to separate them, so %s keeps him; "+
				"pick the candidates by hand to decide it yourself", label, label)
	case row.LastPlayed.Equal(keptByLastPlayed):
		return fmt.Sprintf(
			"also a candidate for %s, and he last played for both on %s, so %s keeps him; "+
				"pick the candidates by hand to decide it yourself",
			label, formatDate(keptByLastPlayed), label)
	default:
		return fmt.Sprintf("also a candidate for %s, whom he played for more recently (%s)",
			label, formatDate(keptByLastPlayed))
	}
}

// refuseSharedSelection is the postcondition on the two chosen elevens: they hold no player
// in common.
//
// Everything above makes that true by construction — the pools are disjoint before a search
// starts, the opposing eleven is sent as `must_exclude`, and a pinned pair sharing a player
// is refused — so a failure here is a defect in this service, not a caller's mistake. It is
// still checked, because the thing it protects is silent: both elevens' aggregates would
// read the same man and every number on the answer would look ordinary.
func refuseSharedSelection(team1Keys, team2Keys []string) error {
	inTeam1 := make(map[string]bool, len(team1Keys))
	for _, key := range team1Keys {
		inTeam1[key] = true
	}
	var shared []string
	for _, key := range team2Keys {
		if inTeam1[key] {
			shared = append(shared, key)
		}
	}
	if len(shared) == 0 {
		return nil
	}
	err := fmt.Errorf("both elevens hold the same player (registry id %s), and nobody plays both sides",
		strings.Join(shared, ", "))
	slog.Error("xi selection: the two elevens are not disjoint", slog.Any("err", err))
	return err
}

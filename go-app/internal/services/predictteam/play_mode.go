package predictteam

import (
	"fmt"

	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// Play mode (P1-2): score the eleven the caller built, rather than searching for one.
//
// The numbers never came from the search. `/xi/optimize` chooses which eleven is scored;
// the displayed probability is `/xi/predict-win`'s display model and the totals, ranges
// and scorecard are `/simulate`'s draws, and both read the eleven they are given. So a
// hand-built eleven takes the *same* path with the selection step replaced by the
// caller's answer: one code path for every number on the surface, and no second way of
// computing a probability that could drift from the first.

// SelectionObjectiveFixed marks an eleven the caller pinned: a selection nothing searched
// for and nothing maximised, which is why no player on it carries a marginal value.
//
// It never crosses the ml-service boundary — ml-service is told which players to score
// and is never asked how they were chosen — so it is go-app's own vocabulary, alongside
// the two objectives that do reach `/xi/optimize`.
const SelectionObjectiveFixed = "fixed"

// fixedSelectionNote is what every surface showing a pinned eleven has to say: these
// eleven are the caller's, scored as sent.
const fixedSelectionNote = "Your eleven, scored as picked: nothing was searched for and nothing " +
	"was reordered, so no player carries a marginal value. Optimise to see the eleven this format's " +
	"policy would choose."

// MissingPlayer names a must-include player an eleven does not hold, so the broken
// constraint can be shown with a name rather than an id.
type MissingPlayer struct {
	PlayerID   int64  `json:"player_id"`
	PlayerName string `json:"player_name"`
}

// ConstraintStatus is one side's eleven measured against the constraints it was sent
// with: what the eleven holds, never what it was changed to hold.
//
// The counts come from ml-service, because "a bowling option" and "a keeper" are defined
// on the as-of vectors the optimiser reads (`ml.xi.contract.is_bowling_option`), not on
// anything this service stores. A second definition here would be a second answer.
type ConstraintStatus struct {
	Size      int  `json:"size"`
	Bowlers   int  `json:"bowlers"`
	HasKeeper bool `json:"has_keeper"`
	// MissingMustInclude names the must-include players this eleven leaves out.
	MissingMustInclude []MissingPlayer `json:"missing_must_include,omitempty"`
	// Met is true where the eleven satisfies every constraint asked of it.
	Met bool `json:"met"`
}

// ConstraintReport says whether each pinned eleven meets the constraints the request
// carried. It is present only where the caller pinned the elevens (Play mode).
//
// A hand-built eleven is never searched, so nothing applies a constraint to it — and
// repairing one silently would put a probability on screen for an eleven the user cannot
// see. So it is reported instead: broken, with the numbers that make it actionable
// ("4 of 5 bowlers"), and scored exactly as it was built (§8.7).
type ConstraintReport struct {
	TeamSize      int              `json:"team_size"`
	MinBowlers    int              `json:"min_bowlers"`
	RequireKeeper bool             `json:"require_keeper"`
	Team1         ConstraintStatus `json:"team1"`
	Team2         ConstraintStatus `json:"team2"`
}

// IncompleteXIError reports a pinned eleven that is not an eleven.
//
// It is refused rather than scored. Every number the surface shows aggregates one side
// into a single row — the objective's, the display model's and the simulator's alike — so
// a ten-man side would be scored as a match nobody plays, and the answer would look like
// every other answer.
type IncompleteXIError struct {
	Team string
	Size int
	Need int
}

func (e *IncompleteXIError) Error() string {
	return fmt.Sprintf("%s has %d players pinned, and an eleven is scored as %d", e.Team, e.Size, e.Need)
}

// UnknownXIPlayerError reports a pinned player this side's candidates do not hold.
//
// Dropping him and scoring the ten who remain is the substitution §8.7 forbids: the
// answer would be for an eleven the caller never sent, and nothing on it would say so.
type UnknownXIPlayerError struct {
	Team      string
	PlayerIDs []int64
}

func (e *UnknownXIPlayerError) Error() string {
	return fmt.Sprintf("%s: player ids %v are not candidates for this side, so no eleven holds them",
		e.Team, e.PlayerIDs)
}

// pinnedXI is both elevens as the caller pinned them, resolved to the registry ids
// ml-service scores, plus the must-include ids each side is checked against.
type pinnedXI struct {
	team1Keys, team2Keys []string
	mustInclude1         []string
	mustInclude2         []string
}

// resolvePinnedXI turns a caller's player ids into the registry ids ml-service knows,
// refusing an eleven that is short, doubled or holds a player this side cannot field.
func resolvePinnedXI(team db.TeamSide, pool []db.PlayerPoolRow, ids []int64, teamSize int) ([]string, error) {
	label := team.Label()
	if len(ids) != teamSize {
		return nil, &IncompleteXIError{Team: label, Size: len(ids), Need: teamSize}
	}
	byID := make(map[int64]db.PlayerPoolRow, len(pool))
	for _, row := range pool {
		byID[row.PlayerID] = row
	}
	keys := make([]string, 0, len(ids))
	seen := make(map[int64]bool, len(ids))
	var unknown []int64
	for _, id := range ids {
		if seen[id] {
			return nil, fmt.Errorf("%s: player id %d is pinned twice, and one player fills one place", label, id)
		}
		seen[id] = true
		row, ok := byID[id]
		if !ok || row.ExternalID == "" {
			unknown = append(unknown, id)
			continue
		}
		keys = append(keys, row.ExternalID)
	}
	if len(unknown) > 0 {
		return nil, &UnknownXIPlayerError{Team: label, PlayerIDs: unknown}
	}
	return keys, nil
}

// mustIncludeKeys resolves the caller's must-include ids to registry ids, keeping only
// those this side's candidates hold: an id no candidate matches cannot be in any eleven
// of theirs, and reporting it as a broken constraint would blame the user's eleven for
// the id being wrong.
func mustIncludeKeys(pool []db.PlayerPoolRow, ids []int64) []string {
	byID := make(map[int64]db.PlayerPoolRow, len(pool))
	for _, row := range pool {
		byID[row.PlayerID] = row
	}
	keys := make([]string, 0, len(ids))
	for _, id := range ids {
		if row, ok := byID[id]; ok && row.ExternalID != "" {
			keys = append(keys, row.ExternalID)
		}
	}
	return keys
}

// pinnedSelection is the selection step for a pinned eleven: the caller's answer, marked
// as the caller's.
func pinnedSelection(pinned pinnedXI) xiSelection {
	return xiSelection{
		Team1Keys: pinned.team1Keys,
		Team2Keys: pinned.team2Keys,
		Summary: SelectionSummary{
			Objective: SelectionObjectiveFixed,
			Optimised: false,
			Note:      fixedSelectionNote,
		},
	}
}

// constraintCheckRequests are the constraints ml-service is asked to check each pinned
// eleven against — the same three the optimiser would have selected under, plus the
// must-include ids, so "five bowlers" means the same thing whether an eleven was searched
// for or built.
func constraintCheckRequests(fix fixture) (*ConstraintCheckRequest, *ConstraintCheckRequest) {
	return &ConstraintCheckRequest{Constraints: fix.constraints, MustIncludeKeys: fix.pinned.mustInclude1},
		&ConstraintCheckRequest{Constraints: fix.constraints, MustIncludeKeys: fix.pinned.mustInclude2}
}

// newConstraintReport turns ml-service's two checks into the response's constraint block.
//
// A pinned prediction with no check is refused by the caller rather than reported as
// "met": an unanswered constraint shown as a satisfied one is the silent substitution
// this block exists to prevent.
func newConstraintReport(fix fixture, team1, team2 *XIConstraintCheck) *ConstraintReport {
	return &ConstraintReport{
		TeamSize:      fix.constraints.Size,
		MinBowlers:    fix.constraints.MinBowlers,
		RequireKeeper: fix.constraints.RequireKeeper,
		Team1:         newConstraintStatus(fix.pool1, team1),
		Team2:         newConstraintStatus(fix.pool2, team2),
	}
}

func newConstraintStatus(pool []db.PlayerPoolRow, check *XIConstraintCheck) ConstraintStatus {
	byKey := make(map[string]db.PlayerPoolRow, len(pool))
	for _, row := range pool {
		byKey[row.ExternalID] = row
	}
	missing := make([]MissingPlayer, 0, len(check.MissingMustIncludeKeys))
	for _, key := range check.MissingMustIncludeKeys {
		row, ok := byKey[key]
		if !ok {
			continue
		}
		missing = append(missing, MissingPlayer{PlayerID: row.PlayerID, PlayerName: row.PlayerName})
	}
	status := ConstraintStatus{
		Size:      check.TeamSize,
		Bowlers:   check.Bowlers,
		HasKeeper: check.HasKeeper,
		Met:       check.Met,
	}
	if len(missing) > 0 {
		status.MissingMustInclude = missing
	}
	return status
}

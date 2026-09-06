package predictteam

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// UnresolvableMustIncludeError refuses a request naming a must-include player this side
// cannot field at all (B-10).
//
// Two ids reach this: one that names no player row, and one whose player carries no
// Cricsheet registry id — the only identity the rating state is keyed on, so ml-service
// cannot be told about him and no eleven can hold him. Either way the lock is impossible,
// and answering with a perfectly good eleven that leaves him out would be the silent
// relaxation §8.7 forbids: the user asked for someone, and the answer must say that the
// ask could not be met rather than quietly not meeting it.
type UnresolvableMustIncludeError struct {
	Team      string
	PlayerIDs []int64
}

func (e *UnresolvableMustIncludeError) Error() string {
	ids := make([]string, 0, len(e.PlayerIDs))
	for _, id := range e.PlayerIDs {
		ids = append(ids, fmt.Sprintf("%d", id))
	}
	return fmt.Sprintf("%s: must-include player %s cannot be selected for this side",
		e.Team, strings.Join(ids, ", "))
}

// mustIncludeKeys resolves one side's must-include ids to the registry ids the selection
// is locked to, refusing any the side cannot field.
//
// It is the single resolution both paths read: the searched path sends these to
// `/xi/optimize` as `must_include`, and Play mode checks the caller's own eleven against
// the same list. Two resolutions would eventually disagree about who was asked for.
func mustIncludeKeys(team db.TeamSide, pool []db.PlayerPoolRow, ids []int64) ([]string, error) {
	byID := make(map[int64]db.PlayerPoolRow, len(pool))
	for _, row := range pool {
		byID[row.PlayerID] = row
	}
	keys := make([]string, 0, len(ids))
	var unresolvable []int64
	for _, id := range ids {
		row, inPool := byID[id]
		if !inPool || row.ExternalID == "" {
			unresolvable = append(unresolvable, id)
			continue
		}
		keys = append(keys, row.ExternalID)
	}
	if len(unresolvable) > 0 {
		err := &UnresolvableMustIncludeError{Team: team.Label(), PlayerIDs: unresolvable}
		slog.Warn("predictteam.PredictTeams refused an unresolvable must-include id",
			slog.String("team", team.Label()), slog.Any("err", err))
		return nil, err
	}
	return keys, nil
}

// mustIncludeFor is one side's must-include registry ids, by which side is asking.
func (f fixture) mustIncludeFor(isTeam1 bool) []string {
	if isTeam1 {
		return f.mustInclude1
	}
	return f.mustInclude2
}

// MustIncludeReport says what became of the must-include ids on a selected eleven.
//
// Since B-10 the ids are a genuine lock: they reach `/xi/optimize` as `must_include`, the
// seed holds them and no swap removes them, so on the searched *and* the rating-ordered
// path the eleven holds every one of them or the request was refused with its reason.
// This is therefore a postcondition rather than a consolation — the check that the lock
// actually held, on the wire, in the terms the user asked in (§8.7). A non-empty `missing`
// now means ml-service did not honour a lock it accepted, which is a defect and reads like
// one. Absent where no id was asked for. In Play mode the constraints block carries the
// same check against the eleven the caller built, so this is not repeated there.
type MustIncludeReport struct {
	Team1 MustIncludeStatus `json:"team1"`
	Team2 MustIncludeStatus `json:"team2"`
}

// MustIncludeStatus is one side's must-include ids against the eleven that was selected:
// how many were asked for, and each one the eleven does not hold.
type MustIncludeStatus struct {
	Requested int `json:"requested"`
	// Missing is always an array, so "checked, none missing" and "not checked" read
	// differently on the wire.
	Missing []MissingPlayer `json:"missing"`
}

// mustIncludeReport checks both selected elevens against the must-include ids the request
// carried. Nil where none were asked for, and nil in Play mode, where the constraints
// block already reports the same check on the caller's own eleven.
func mustIncludeReport(input Input, fix fixture, selection xiSelection) *MustIncludeReport {
	if fix.isPinned || (len(input.ExtraTeam1) == 0 && len(input.ExtraTeam2) == 0) {
		return nil
	}
	return &MustIncludeReport{
		Team1: mustIncludeStatus(fix.pool1, input.ExtraTeam1, selection.Team1Keys),
		Team2: mustIncludeStatus(fix.pool2, input.ExtraTeam2, selection.Team2Keys),
	}
}

func mustIncludeStatus(pool []db.PlayerPoolRow, ids []int64, selectedKeys []string) MustIncludeStatus {
	byID := make(map[int64]db.PlayerPoolRow, len(pool))
	for _, row := range pool {
		byID[row.PlayerID] = row
	}
	selected := make(map[string]bool, len(selectedKeys))
	for _, key := range selectedKeys {
		selected[key] = true
	}
	missing := make([]MissingPlayer, 0, len(ids))
	for _, id := range ids {
		row, inPool := byID[id]
		if inPool && selected[row.ExternalID] {
			continue
		}
		// An id the pool cannot resolve never reaches here: it is refused while the
		// fixture resolves (B-10), so this is a player the pool holds, whom the lock was
		// sent for and the eleven does not hold. His name is the pool's.
		missing = append(missing, MissingPlayer{PlayerID: id, PlayerName: row.PlayerName})
	}
	if len(missing) > 0 {
		slog.Error("xi selection: a must-include lock was accepted and not honoured",
			slog.Int("requested", len(ids)), slog.Int("missing", len(missing)))
	}
	return MustIncludeStatus{Requested: len(ids), Missing: missing}
}

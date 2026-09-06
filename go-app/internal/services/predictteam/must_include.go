package predictteam

import (
	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// MustIncludeReport says what became of the must-include ids on a selected eleven (P1-4).
//
// A must-include id joins its side's pool (P1-1) and is not enforced: go-app sends
// ml-service an empty `must_include`, so the search — and the rating order — may leave
// him out (docs/PRODUCT_ROADMAP.md § 4; the real fix, a lock the optimiser honours, is
// docs/BUG_BACKLOG.md § B-10). The Lab's label says exactly that, and this is the check
// the label promises: made after the selection, on the wire, naming who was left out
// rather than leaving the reader to count (§8.7). Absent where no id was asked for. In
// Play mode the constraints block carries the same check against the eleven the caller
// built, so this is not repeated there.
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
		// An id the pool cannot resolve stops the prediction upstream (P1-1), so this is
		// a player the pool holds and the eleven does not; his name is the pool's.
		missing = append(missing, MissingPlayer{PlayerID: id, PlayerName: row.PlayerName})
	}
	return MustIncludeStatus{Requested: len(ids), Missing: missing}
}

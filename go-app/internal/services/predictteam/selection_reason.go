package predictteam

import (
	"log/slog"

	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// P1-3, the "why this player" card: what the *selection* read about a player it picked.
//
// The rule this file exists to keep is the card's rule, and it is a rule about provenance
// rather than about presentation. Every field below is a value the selection itself
// consumed, carried forward unchanged:
//
//   - Roles are the constraint predicates ml-service's optimiser evaluates over the served
//     as-of vectors (`_Pool.is_keeper`, `_Pool.is_bowler`) — the same two the keeper and
//     minimum-bowler constraints count, and the same two the objective reads as its
//     `has_keeper` and `n_bowlers` features.
//   - SelectionRating is `ml.xi.optimizer.rating_order_score`: the one composite the pool
//     is ordered by, which fills the constraints and then the eleven. It is the seed the
//     search starts from, and the whole answer where a format is not searched.
//   - RatingPercentile is that composite's standing *within the pool as served*, which is
//     the set the selection actually chose out of. PoolSize says how many that was, because
//     a percentile over fourteen candidates is not a percentile over ninety.
//   - BestAlternative is the search's own last question asked once more: over the same
//     one-for-one swaps `_best_neighbour` scores, under the same constraints, against the
//     same opposing eleven. Absent where nothing was maximised.
//
// Nothing here is derived from a model the selection did not run, and nothing is
// approximated. Where the stack cannot produce a field from a consumed input, the field is
// absent and the reason is recorded in docs/PRODUCT_ROADMAP.md § 4 — never filled with a
// plausible-looking number.
//
// The marginal value and the expected contribution are the card's other two numbers and
// are already on SelectedPlayer (`marginal_value`, the runs/wickets points and their 10-90
// ranges), so they are not repeated here.

// The constraint state a card may name. Two, not the three § 4 first sketched:
//
//   - "must-include" is absent even though the search now honours it (B-10): a lock is the
//     caller's own input echoed back, not something the selection read *about* the player,
//     and the answer reports it per side (`selection.must_include`) where it belongs.
//   - "top-order anchor" is absent because the batting-order state (exp_bat_position) is
//     read by the performance model, not by the selection objective, so it explains the
//     expected contribution and never the pick.
//
// Both omissions are recorded in docs/PRODUCT_ROADMAP.md § 4. H-24: the vocabulary is
// declared here, published in contracts/ops-console.contract.json, and asserted from
// ml-service (ml.xi.optimizer.SELECTION_ROLES) and the frontend.
const (
	// RoleKeeper is a player the served state has credited with a stumping: the flag the
	// keeper constraint counts and the objective reads as `has_keeper`.
	RoleKeeper = "keeper"
	// RoleBowlingOption is a player whose expected balls bowled clear the format's
	// threshold: what the minimum-bowlers constraint counts and the objective reads as
	// `n_bowlers`.
	RoleBowlingOption = "bowling_option"
)

// SelectionRoles returns every role a selection reason may name, in the order a surface
// should show them.
func SelectionRoles() []string {
	return []string{RoleKeeper, RoleBowlingOption}
}

// BestAlternative is the pool player the objective would most like to have instead, and
// what putting him there would cost.
//
// The gap is a point estimate and carries no interval, because the computation that
// produces it — one evaluation of the objective per candidate swap — yields none. A gap at
// or below zero means the search stopped on its evaluation budget rather than at a local
// optimum, which is a fact about the search and is shown as one.
type BestAlternative struct {
	PlayerID   int64  `json:"player_id"`
	PlayerName string `json:"player_name"`
	// WinProbabilityGap is P(win) with the selected player minus P(win) with this
	// alternative in his place.
	WinProbabilityGap float64 `json:"win_probability_gap"`
}

// PlayerSelectionReason is one selected player's entry on the card.
type PlayerSelectionReason struct {
	// Roles are the constraint state this player answers, from
	// ml.xi.optimizer.SELECTION_ROLES. Empty where he answers neither and was picked on
	// his rating alone, which is a true thing to show rather than a gap to fill.
	Roles            []string `json:"roles"`
	SelectionRating  float64  `json:"selection_rating"`
	RatingPercentile float64  `json:"rating_percentile"`
	PoolSize         int      `json:"pool_size"`
	// BestAlternative is absent on a rating-ordered XI, where nothing was maximised and
	// so nothing was compared. BestAlternativeNote says why it is absent in the cases
	// where the objective *was* asked and could not answer (§8.7: the reason is on the
	// wire, not only in a log).
	BestAlternative     *BestAlternative `json:"best_alternative,omitempty"`
	BestAlternativeNote string           `json:"best_alternative_note,omitempty"`
}

// XISelectionReason is ml-service's answer before the pool resolves it: the alternative is
// still a registry id, which is not an identifier this repo puts on the wire.
type XISelectionReason struct {
	Roles              []string
	SelectionRating    float64
	RatingPercentile   float64
	PoolSize           int
	BestAlternativeKey string
	BestAlternativeGap float64
	// BestAlternativeNote is ml-service's own sentence for an objective that was asked
	// for an alternative and had none to give.
	BestAlternativeNote string
}

// unresolvedAlternativeNote is shown when ml-service names an alternative this pool cannot
// resolve to a player. It is on the wire rather than only in the log because a card that
// silently dropped its comparison would read as "there is no alternative", which is a
// different claim from "we could not name the one there is" (§8.7).
const unresolvedAlternativeNote = "the objective named an alternative this pool cannot resolve to a player, " +
	"so the comparison is not shown"

// newSelectionReason resolves one player's reason for the wire, turning the alternative's
// registry id into the player id and name a client can use.
func newSelectionReason(reason XISelectionReason, byKey map[string]db.PlayerPoolRow) PlayerSelectionReason {
	roles := reason.Roles
	if roles == nil {
		roles = []string{}
	}
	out := PlayerSelectionReason{
		Roles:               roles,
		SelectionRating:     reason.SelectionRating,
		RatingPercentile:    reason.RatingPercentile,
		PoolSize:            reason.PoolSize,
		BestAlternativeNote: reason.BestAlternativeNote,
	}
	if reason.BestAlternativeKey == "" {
		return out
	}
	row, ok := byKey[reason.BestAlternativeKey]
	if !ok {
		slog.Warn("selection reason names an alternative the pool does not hold",
			slog.String("player_key", reason.BestAlternativeKey))
		out.BestAlternativeNote = unresolvedAlternativeNote
		return out
	}
	out.BestAlternative = &BestAlternative{
		PlayerID:          row.PlayerID,
		PlayerName:        row.PlayerName,
		WinProbabilityGap: reason.BestAlternativeGap,
	}
	return out
}

// mergeSelectionReasons merges both sides' reasons; registry ids are unique across sides,
// as they are for the marginal values beside them.
func mergeSelectionReasons(sides ...map[string]XISelectionReason) map[string]XISelectionReason {
	out := map[string]XISelectionReason{}
	for _, side := range sides {
		for key, reason := range side {
			out[key] = reason
		}
	}
	return out
}

package predictteam

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/config"
	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// Selection objectives, mirroring ml-service's /xi/optimize.
const (
	// SelectionObjectiveWin searches for the XI that maximises the objective model's P(win)
	// against the opposing XI.
	SelectionObjectiveWin = "win"
	// SelectionObjectiveRatings returns the rating-ordered pick and maximises nothing. It is
	// the only mode offered where the objective does not rank (H-17).
	SelectionObjectiveRatings = "ratings"
)

// SelectionObjectives returns every value `selection.objective` may carry, in the order a
// surface should offer them (H-24).
//
// It became a declared vocabulary with the prediction record (P2-3): the store keeps the
// objective as a column and the record's listing puts it on the wire, where it is the one
// thing that separates an eleven this service chose from one the caller pinned in Play
// mode — a scenario, which the track record lists and never scores. A reader that spelled
// `fixed` differently would score a hypothetical eleven as though it were a forecast.
func SelectionObjectives() []string {
	return []string{SelectionObjectiveWin, SelectionObjectiveRatings, SelectionObjectiveFixed}
}

// notOptimisedReasons names every format that is served a rating-ordered XI rather than an
// optimised one, with the reason every surface showing that XI has to give. One sentence
// per format, in one place, so the API and the UI cannot drift apart about it.
//
// Two rules put a format here. H-17 (P-5): an objective whose holdout AUC is under 0.65
// does not rank, so it is not searched over (TEST). E5 (P-7, plan §8.8): an objective that
// ranks but whose preference between one side's consecutive elevens does not agree with
// the result change at the bar derived from its own claimed effect size has not shown it
// selects, so the search is not served there either. A format absent from this map is
// optimised. Mirrors ml.xi.optimizer.NOT_OPTIMISED_REASONS; ml-service refuses the win
// objective for every format listed there, so a drift here is a 503 with a message, never
// a silent fallback.
var notOptimisedReasons = map[string]string{
	"TEST": "Rating-ordered XI: this format has no win objective that ranks (holdout AUC below " +
		"0.65, H-17), so the eleven is picked by as-of rating under the same constraints and is " +
		"not optimised for win probability.",
	"T20": "Rating-ordered XI: this format's win objective ranks but has not shown it selects — " +
		"in the natural experiment (E5) its preference between one side's consecutive elevens " +
		"agreed with the result change 0.503 of the time against a bar of 0.506 derived from its " +
		"own claimed effect (0.490 against 0.501 when first decided; neither of A-3's feature " +
		"families changed the verdict) — so the eleven is picked by as-of rating under the same constraints " +
		"and is not optimised for win probability; the win probability shown still reads the eleven.",
}

// isOptimisedSelectionFormat reports whether a format's XIs are searched for on the win
// objective; the rest are rating-ordered and labelled with their reason.
func isOptimisedSelectionFormat(format string) bool {
	_, scopedOff := notOptimisedReasons[format]
	return !scopedOff
}

// XISelectionOptimizer is the ml-service /xi/optimize contract: pick the XI from a pool of
// player ids, either against the objective model or by rating alone.
//
// It carries no per-player features. The rating state the model reads lives in ml-service
// and is a function of the ids alone, which is what makes the training and serving paths
// compute the same function of the same eleven names.
type XISelectionOptimizer interface {
	OptimizeXI(ctx context.Context, req XIOptimizationRequest) (*XIOptimizationResult, error)
}

// XIWinPredictor is the ml-service /xi/predict-win contract: P(team1 wins) for two elevens.
type XIWinPredictor interface {
	PredictMatchWinXI(ctx context.Context, req XIWinRequest) (*XIWinResult, error)
}

// XIWinResult is the Go-side response from POST /xi/predict-win: the displayed
// probability, and the rating state it was read from.
type XIWinResult struct {
	Team1WinProbability float64
	// Team1Check and Team2Check are the constraint checks, present only where the
	// request asked for them — that is, only where the caller pinned the elevens.
	Team1Check *XIConstraintCheck
	Team2Check *XIConstraintCheck
	Served     ServedRatings
}

// XIConstraintCheck is one eleven measured against its constraints by ml-service, which
// is where "a bowling option" and "a keeper" are defined (P1-2).
type XIConstraintCheck struct {
	TeamSize               int
	Bowlers                int
	MinBowlers             int
	HasKeeper              bool
	RequireKeeper          bool
	MissingMustIncludeKeys []string
	Met                    bool
}

// ConstraintCheckRequest asks ml-service to check an eleven against these constraints
// rather than select under them: the same three the optimiser reads, plus the registry
// ids the caller requires in the side.
type ConstraintCheckRequest struct {
	Constraints
	MustIncludeKeys []string
}

// XIOptimizationRequest is the Go-side payload for POST /xi/optimize.
type XIOptimizationRequest struct {
	Format    string
	Objective string
	// PoolPlayerKeys and OpponentPlayerKeys are registry ids: the identity the rating state
	// is keyed on, and the only one ml-service can resolve to a rated player.
	PoolPlayerKeys     []string
	OpponentPlayerKeys []string
	TeamIsTeam1        bool
	Constraints        Constraints
	// MustIncludeKeys are the registry ids the search must keep in the eleven: the seed
	// holds them, no swap removes them, and a constraint set that cannot hold them is
	// refused with its reason rather than quietly relaxed (B-10).
	MustIncludeKeys []string
	MaxEvaluations  int
	// AsOf asks for ratings as they stood strictly before this date (backtests); zero
	// means the serving state through today.
	AsOf time.Time
}

// XIOptimizationResult is the Go-side response from POST /xi/optimize.
type XIOptimizationResult struct {
	SelectedPlayerKeys []string
	Objective          string
	Optimised          bool
	UnknownPlayerKeys  []string
	MarginalValues     map[string]float64
	// SelectionReasons is what the selection read about each player it picked, keyed by
	// registry id (P1-3). Present on both objectives; a rating-ordered one carries no
	// best alternative, because nothing was maximised.
	SelectionReasons map[string]XISelectionReason
	// Served is the rating state the selection was made from.
	Served ServedRatings
}

// xiSelection is both chosen elevens, how they were chosen, and the rating state every
// optimiser call that chose them was answered from.
type xiSelection struct {
	Team1Keys []string
	Team2Keys []string
	Summary   SelectionSummary
	Marginals map[string]float64
	// Reasons is the "why this player" state for both elevens, keyed by registry id.
	Reasons map[string]XISelectionReason
	Served  ServedRatings
}

// XIWinRequest is the Go-side payload for POST /xi/predict-win.
type XIWinRequest struct {
	Format          string
	Team1PlayerKeys []string
	Team2PlayerKeys []string
	Team1ID         int64
	Team2ID         int64
	VenueID         int64
	AsOf            time.Time
	// Team1Constraints and Team2Constraints ask for a constraint check on the eleven
	// being scored. Nil on the searched path, where the optimiser applied them already.
	Team1Constraints *ConstraintCheckRequest
	Team2Constraints *ConstraintCheckRequest
}

// chooseXIs settles which eleven each side plays: the caller's, where they built one
// (Play mode), or the search's.
//
// It is the only step Play mode replaces. Everything after it — the displayed
// probability, the simulated totals and the scorecard — runs on the elevens this returns,
// so a hand-built eleven and a searched one are scored by the same code reading the same
// models (P1-2).
func chooseXIs(ctx context.Context, optimizer XISelectionOptimizer, fix fixture) (xiSelection, error) {
	if fix.isPinned {
		slog.InfoContext(ctx, "scoring the caller's elevens", slog.String("format", fix.format))
		return pinnedSelection(fix.pinned), nil
	}
	return selectBothXIs(ctx, optimizer, fix)
}

// selectBothXIs picks an XI for each side and reports how.
//
// Where the objective ranks, the two XIs come from alternating best response: each side is
// optimised against the other side's currently selected XI, never against the other side's
// whole pool, because every feature the objective reads is an aggregate over one eleven —
// scoring an eleven against an eighteen-man squad evaluates a match that cannot happen.
// Where it does not rank, rating order does not depend on the opponent at all, so one call
// per side is the whole selection.
func selectBothXIs(ctx context.Context, optimizer XISelectionOptimizer, fix fixture) (xiSelection, error) {
	if !isOptimisedSelectionFormat(fix.format) {
		return selectByRatings(ctx, optimizer, fix)
	}
	return selectByWinProbability(ctx, optimizer, fix)
}

// selectByRatings is the H-17 path: a selection, but not an optimised one.
func selectByRatings(ctx context.Context, optimizer XISelectionOptimizer, fix fixture) (xiSelection, error) {
	selection := xiSelection{Summary: SelectionSummary{
		Objective: SelectionObjectiveRatings,
		Optimised: false,
		Note:      notOptimisedReasons[fix.format],
	}}
	xi1, err := optimizeSide(ctx, optimizer, fix, SelectionObjectiveRatings, fix.pool1, nil, true)
	if err != nil {
		return selection, fmt.Errorf("select %s: %w", fix.team1.Label(), err)
	}
	xi2, err := optimizeSide(ctx, optimizer, fix, SelectionObjectiveRatings, fix.pool2, nil, false)
	if err != nil {
		return selection, fmt.Errorf("select %s: %w", fix.team2.Label(), err)
	}
	for _, answer := range []*XIOptimizationResult{xi1, xi2} {
		if err := selection.Served.adopt(answer.Served); err != nil {
			return selection, fmt.Errorf("select: %w", err)
		}
	}
	selection.Team1Keys, selection.Team2Keys = xi1.SelectedPlayerKeys, xi2.SelectedPlayerKeys
	selection.Reasons = mergeSelectionReasons(xi1.SelectionReasons, xi2.SelectionReasons)
	slog.InfoContext(ctx, "rating-ordered XIs selected", slog.String("format", fix.format))
	return selection, nil
}

// selectByWinProbability alternates: optimise team1 against team2's XI, then team2 against
// team1's new XI, and repeat.
//
// It stops early when a round changes neither XI — a fixed point, where neither side can
// improve on what the other has picked. Rounds are capped because best response can cycle
// rather than converge; the last completed round is returned, and no claim of equilibrium
// is made about it. Each side is seeded with its own rating-ordered XI, so the very first
// optimisation already plays against an eleven.
func selectByWinProbability(ctx context.Context, optimizer XISelectionOptimizer, fix fixture) (xiSelection, error) {
	selection := xiSelection{
		Summary:   SelectionSummary{Objective: SelectionObjectiveWin, Optimised: true},
		Marginals: map[string]float64{},
		Reasons:   map[string]XISelectionReason{},
	}
	seed1, err := optimizeSide(ctx, optimizer, fix, SelectionObjectiveRatings, fix.pool1, nil, true)
	if err != nil {
		return selection, fmt.Errorf("seed %s: %w", fix.team1.Label(), err)
	}
	seed2, err := optimizeSide(ctx, optimizer, fix, SelectionObjectiveRatings, fix.pool2, nil, false)
	if err != nil {
		return selection, fmt.Errorf("seed %s: %w", fix.team2.Label(), err)
	}
	for _, answer := range []*XIOptimizationResult{seed1, seed2} {
		if err := selection.Served.adopt(answer.Served); err != nil {
			return selection, fmt.Errorf("seed: %w", err)
		}
	}
	selection.Team1Keys, selection.Team2Keys = seed1.SelectedPlayerKeys, seed2.SelectedPlayerKeys

	for round := 1; round <= config.SelectionBestResponseRounds(config.Load()); round++ {
		next1, err := optimizeSide(ctx, optimizer, fix, SelectionObjectiveWin, fix.pool1, selection.Team2Keys, true)
		if err != nil {
			return selection, fmt.Errorf("optimize %s (round %d): %w", fix.team1.Label(), round, err)
		}
		next2, err := optimizeSide(
			ctx,
			optimizer,
			fix,
			SelectionObjectiveWin,
			fix.pool2,
			next1.SelectedPlayerKeys,
			false,
		)
		if err != nil {
			return selection, fmt.Errorf("optimize %s (round %d): %w", fix.team2.Label(), round, err)
		}
		for _, answer := range []*XIOptimizationResult{next1, next2} {
			if err := selection.Served.adopt(answer.Served); err != nil {
				return selection, fmt.Errorf("optimize (round %d): %w", round, err)
			}
		}
		settled := sameXI(selection.Team1Keys, next1.SelectedPlayerKeys) &&
			sameXI(selection.Team2Keys, next2.SelectedPlayerKeys)
		selection.Team1Keys, selection.Team2Keys = next1.SelectedPlayerKeys, next2.SelectedPlayerKeys
		selection.Marginals = mergeMarginals(next1.MarginalValues, next2.MarginalValues)
		// The reasons come from the same round as the marginal values beside them, so the
		// card and the headline describe one search against one opposing eleven.
		selection.Reasons = mergeSelectionReasons(next1.SelectionReasons, next2.SelectionReasons)
		if settled {
			slog.InfoContext(ctx, "win-probability selection settled",
				slog.String("format", fix.format), slog.Int("rounds", round))
			break
		}
	}
	return selection, nil
}

// optimizeSide is one call to /xi/optimize for one side.
func optimizeSide(
	ctx context.Context,
	optimizer XISelectionOptimizer,
	fix fixture,
	objective string,
	pool []db.PlayerPoolRow,
	opponentXI []string,
	isTeam1 bool,
) (*XIOptimizationResult, error) {
	poolKeys := poolPlayerKeys(pool)
	if len(poolKeys) < fix.constraints.Size {
		return nil, fmt.Errorf("pool resolves to %d registry ids, need %d", len(poolKeys), fix.constraints.Size)
	}
	if objective == SelectionObjectiveWin && len(opponentXI) == 0 {
		return nil, fmt.Errorf("the win objective needs an opposing XI, and none was selected")
	}
	result, err := optimizer.OptimizeXI(ctx, XIOptimizationRequest{
		Format:             fix.format,
		Objective:          objective,
		PoolPlayerKeys:     poolKeys,
		OpponentPlayerKeys: opponentXI,
		TeamIsTeam1:        isTeam1,
		Constraints:        fix.constraints,
		MustIncludeKeys:    fix.mustIncludeFor(isTeam1),
		MaxEvaluations:     config.SelectionMaxWinProbEvalBudget(config.Load()),
		AsOf:               fix.asOf,
	})
	if err != nil {
		return nil, err
	}
	if len(result.UnknownPlayerKeys) > 0 {
		slog.WarnContext(ctx, "xi selection: pool players with no rating history were treated as debutants",
			slog.Int("count", len(result.UnknownPlayerKeys)))
	}
	return result, nil
}

// sameXI reports whether two selections name the same players. Order is not significant: a
// fixed-point test that depended on it would break quietly the day the optimiser stopped
// returning a stable order.
func sameXI(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[string]bool, len(a))
	for _, key := range a {
		seen[key] = true
	}
	for _, key := range b {
		if !seen[key] {
			return false
		}
	}
	return true
}

// mergeMarginals merges both sides' marginal values; registry ids are unique across sides.
func mergeMarginals(sides ...map[string]float64) map[string]float64 {
	out := map[string]float64{}
	for _, side := range sides {
		for key, v := range side {
			out[key] = v
		}
	}
	return out
}

// NormalizeFormat is the one place a format string is folded, so the constant maps, the
// wire payloads and the prediction record agree on spelling.
//
// It is exported because the record stores the format a prediction was computed for
// (P2-3), and a second folding rule in the server would be a stored format that did not
// match the one the prediction was actually answered under.
func NormalizeFormat(format string) string {
	return strings.TrimSpace(strings.ToUpper(format))
}

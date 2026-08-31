package predictteam

import (
	"context"
	"fmt"
	"log/slog"
	"sort"

	"github.com/umayangag/cric-flow/go-app/internal/config"
	"github.com/umayangag/cric-flow/go-app/internal/services/teamselect"
)

// XISelectionOptimizer is the ml-service /xi/optimize contract: pick the XI from a pool of
// player ids that maximises the XI-responsive win model against a fixed opponent XI.
//
// Unlike TeamSelectionOptimizer it carries no per-player features. The rating state the
// model reads lives in ml-service and is a function of the ids alone, which is what makes
// the training and serving paths compute the same function of the same eleven names.
type XISelectionOptimizer interface {
	OptimizeXI(ctx context.Context, req XIOptimizationRequest) (*XIOptimizationResult, error)
}

// XIWinPredictor is the ml-service /xi/predict-win contract: P(team1 wins) for two elevens.
type XIWinPredictor interface {
	PredictMatchWinXI(ctx context.Context, req XIWinRequest) (float64, error)
}

// XIOptimizationRequest is the Go-side payload for POST /xi/optimize.
type XIOptimizationRequest struct {
	Format            string
	PoolPlayerIDs     []int64
	OpponentPlayerIDs []int64
	TeamIsTeam1       bool
	Constraints       teamselect.Constraints
	MaxEvaluations    int
}

// XIOptimizationResult is the Go-side response from POST /xi/optimize.
type XIOptimizationResult struct {
	SelectedPlayerIDs []int64
	WinProbability    float64
	Evaluations       int
	ImprovedOverSeed  float64
	UnknownPlayerIDs  []int64
	MarginalValues    map[int64]float64
}

// XIWinRequest is the Go-side payload for POST /xi/predict-win.
type XIWinRequest struct {
	Format         string
	Team1PlayerIDs []int64
	Team2PlayerIDs []int64
	Team1ID        int64
	Team2ID        int64
	VenueID        int64
}

// selectionUsesXIWinModel reports whether selection and the displayed probability should
// come from the XI-responsive model (selection.win_model = "xi") rather than the
// windowed-form one. A variable so tests can pin it without touching the cached config.
var selectionUsesXIWinModel = func() bool {
	return config.SelectionUsesXIWinModel(config.Load())
}

// newXISideOptimizer runs one side's search inside ml-service against the XI-responsive
// model. The pool and the opposing XI are sent as player ids; names are resolved back
// against the pool so the returned players carry the scores the scorecard displays.
func newXISideOptimizer(optimizer XISelectionOptimizer, in winProbSelectionInputs) sideOptimizer {
	maxEvals := config.SelectionMaxWinProbEvalBudget(config.Load())
	return func(ctx context.Context, s selectionSide, opponentXI []teamselect.Player) ([]teamselect.Player, error) {
		poolIDs, idToPlayer := poolPlayerIDs(s.pool, s.nameToID)
		if len(poolIDs) < in.constraints.Size {
			return nil, fmt.Errorf("xi selection: pool resolves to %d ids, need %d", len(poolIDs), in.constraints.Size)
		}
		opponentIDs := selectedPlayerIDs(opponentXI, s.opponentNameToID)
		if len(opponentIDs) == 0 {
			return nil, fmt.Errorf("xi selection: opponent XI resolves to no ids")
		}
		result, err := optimizer.OptimizeXI(ctx, XIOptimizationRequest{
			Format:            in.format,
			PoolPlayerIDs:     poolIDs,
			OpponentPlayerIDs: opponentIDs,
			TeamIsTeam1:       s.isTeam1,
			Constraints:       in.constraints,
			MaxEvaluations:    maxEvals,
		})
		if err != nil {
			return nil, err
		}
		if len(result.UnknownPlayerIDs) > 0 {
			slog.WarnContext(ctx, "xi selection: pool players with no rating history were treated as debutants",
				slog.Int("count", len(result.UnknownPlayerIDs)))
		}
		return xiResultToPlayers(result, idToPlayer), nil
	}
}

// poolPlayerIDs resolves a pool's names to ids, skipping names the id map does not know,
// and returns the reverse map used to turn the optimiser's answer back into players.
func poolPlayerIDs(pool []teamselect.Player, nameToID map[string]int64) ([]int64, map[int64]teamselect.Player) {
	ids := make([]int64, 0, len(pool))
	byID := make(map[int64]teamselect.Player, len(pool))
	for _, p := range pool {
		pid, ok := nameToID[p.Name]
		if !ok {
			continue
		}
		if _, dup := byID[pid]; dup {
			continue
		}
		ids = append(ids, pid)
		byID[pid] = p
	}
	return ids, byID
}

func xiResultToPlayers(result *XIOptimizationResult, byID map[int64]teamselect.Player) []teamselect.Player {
	out := make([]teamselect.Player, 0, len(result.SelectedPlayerIDs))
	for _, pid := range result.SelectedPlayerIDs {
		if p, ok := byID[pid]; ok {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Package selection provides DB-backed team selection: it builds the candidate pool,
// scores it from ML predictions, and applies the XI constraints.
package selection

import "github.com/umayangag/cric-flow/go-app/internal/predictor"

// Options controls constraints for picking the final XI.
type Options struct {
	TeamSize      int  // number of players to pick (default 11 if <=0)
	MinBowlers    int  // minimum number of bowlers to include
	RequireKeeper bool // require at least one wicket-keeper
}

// Result is the outcome from the selection pipeline.
type Result struct {
	Players            []predictor.PlayerPrediction // selected XI, sorted by score desc
	TeamWinProbability float64                      // mean selection score across the XI
}

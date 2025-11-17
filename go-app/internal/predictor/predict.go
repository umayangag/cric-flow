package predictor

import (
	"context"
)

// BuildTeam predicts winning probabilities for the given players using the provided
// predictor and returns the top-N by probability (deterministic tie-breakers).
// Pure w.r.t. external systems: requires caller-provided context and predictor.
func BuildTeam(
	ctx context.Context,
	p Predictor,
	players []PlayerPrediction,
	teamSize int,
) ([]PlayerPrediction, error) {
	preds, err := p.PredictWin(ctx, players)
	if err != nil {
		return nil, err
	}
	return selectTop(preds, teamSize), nil
}

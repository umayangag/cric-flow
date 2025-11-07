package main

import (
	"context"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/mlclient"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/predictor"
)

// buildTeam predicts winning probabilities for the given players using the provided
// predictor and returns the top-N by probability (deterministic tie-breakers).
// Pure w.r.t. external systems: requires caller-provided context and predictor.
func buildTeam(
	ctx context.Context,
	p mlclient.Predictor,
	players []predictor.PlayerPrediction,
	teamSize int,
) ([]predictor.PlayerPrediction, error) {
	preds, err := p.PredictWin(ctx, players)
	if err != nil {
		return nil, err
	}
	return selectTop(preds, teamSize), nil
}

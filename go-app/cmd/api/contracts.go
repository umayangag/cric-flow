package main

import (
	"context"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/contracts"
)

type Client interface {
	PredictBatting(ctx context.Context, feats []contracts.BattingFeatures) ([]contracts.BattingPrediction, error)
	PredictBowling(ctx context.Context, feats []contracts.BowlingFeatures) ([]contracts.BowlingPrediction, error)
}

package mlclient

import (
	"context"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/predictor"
)

// Predictor defines the minimal interface required by command packages
// to obtain winning predictions. The concrete Client implements this.
type Predictor interface {
	PredictWin(ctx context.Context, players []predictor.PlayerPrediction) ([]predictor.PlayerPrediction, error)
}

package teampredictor

import (
	"context"

	cli "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/teampredictor"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/mlclient"
)

// MLClient abstracts the ML prediction dependency for this service.
// Kept local to the service for easier mocking and to avoid test import cycles.
type MLClient interface {
	PredictTeam(ctx context.Context, in mlclient.PredictRequest) (mlclient.PredictResponse, error)
	Reload(ctx context.Context) error
}

// Predictor abstracts the service behavior used by this command.
type Predictor interface {
	Predict(ctx context.Context, opts cli.Options) (mlclient.PredictResponse, error)
}

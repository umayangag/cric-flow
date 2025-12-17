package teampredictor

import (
	"context"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/mlclient"
)

// MLClient abstracts the ML prediction dependency for this service.
// Kept local to the service for easier mocking and to avoid test import cycles.
type MLClient interface {
	PredictTeam(ctx context.Context, in mlclient.PredictRequest) (mlclient.PredictResponse, error)
	Reload(ctx context.Context) error
}

package mlclient

import "context"

// PredictRequest contains inputs for team prediction.
type PredictRequest struct {
	MatchID int64
	Format  string
	Season  string
	Bat     int
	Bowl    int
}

// PredictResponse contains the predicted lineup and optional score/likelihood.
type PredictResponse struct {
	Players []string
	Score   float64
}

// Client is the abstraction for an ML predictor service.
//
//go:generate mockery --name Client --output internal/mocks --case underscore
type Client interface {
	PredictTeam(ctx context.Context, in PredictRequest) (PredictResponse, error)
	Reload(ctx context.Context) error
}

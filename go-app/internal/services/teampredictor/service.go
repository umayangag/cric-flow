package teampredictor

import (
	"context"
	"errors"

	"github.com/umayangag/cric-flow/go-app/internal/mlclient"
)

// Service orchestrates team prediction via mlclient.
// Keep it small and deterministic; no logging here.

type Service struct {
	mlClient MLClient
}

// NewService wires the ML client dependency.
func NewService(c MLClient) *Service { return &Service{mlClient: c} }

func (s *Service) Predict(ctx context.Context, opts Options) (mlclient.PredictResponse, error) {
	if s == nil || s.mlClient == nil {
		return mlclient.PredictResponse{}, errors.New("nil service or client client")
	}
	if opts.MatchID <= 0 || opts.Format == "" || opts.Season == "" || opts.Bat < 0 || opts.Bowl < 0 {
		return mlclient.PredictResponse{}, errors.New("invalid options")
	}
	req := mlclient.PredictRequest{
		MatchID: opts.MatchID,
		Format:  opts.Format,
		Season:  opts.Season,
		Bat:     opts.Bat,
		Bowl:    opts.Bowl,
	}
	return s.mlClient.PredictTeam(ctx, req)
}

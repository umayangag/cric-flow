package fake

import (
	"context"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/mlclient"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/predictor"
)

// Client is a deterministic fake for tests. It implements mlclient.Predictor.
type Client struct {
	// Responses, if non-nil, are returned directly from PredictWin.
	Responses []predictor.PlayerPrediction
	// Err, if set, is returned by PredictWin to simulate failures.
	Err error
}

var _ mlclient.Predictor = (*Client)(nil)

func (f *Client) PredictWin(
	_ context.Context,
	players []predictor.PlayerPrediction,
) ([]predictor.PlayerPrediction, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	if f.Responses != nil {
		return f.Responses, nil
	}
	// Default behavior: echo back players unchanged.
	return players, nil
}

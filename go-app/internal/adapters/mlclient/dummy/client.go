package dummy

import (
	"context"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/mlclient"
)

// Client is a trivial in-process ML client adapter used for wiring during refactors.
// It returns a deterministic list of players for demonstration/testing purposes.

type Client struct{}

func New() *Client { return &Client{} }

func (c *Client) PredictTeam(_ context.Context, in mlclient.PredictRequest) (mlclient.PredictResponse, error) {
	// Return a small deterministic lineup using inputs to vary slightly
	base := []string{"P1","P2","P3","P4","P5","P6","P7","P8","P9","P10","P11"}
	// Respect requested bat/bowl counts only to shape a plausible output length
	_ = in
	return mlclient.PredictResponse{Players: base, Score: 0.75}, nil
}

func (c *Client) Reload(_ context.Context) error { return nil }

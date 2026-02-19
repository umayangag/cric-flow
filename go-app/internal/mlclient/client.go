// Package mlclient provides a typed HTTP client for the Python mlCleint service.
package mlclient

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/models"
)

// Service calls the Python mlCleint service.
type Client struct {
	BaseURL   string
	HTTP      *http.Client
	UserAgent string
	Timeout   time.Duration
}

// New returns a client with defaults. Override base URL via ML_BASE_URL env.
func New() *Client {
	base := os.Getenv("ML_BASE_URL")
	if base == "" {
		base = "http://localhost:8000"
	}
	return &Client{
		BaseURL:   base,
		HTTP:      &http.Client{Timeout: 20 * time.Second},
		UserAgent: "cric-app-mlclient (+github.com/umayangag/cric-flow)",
		Timeout:   20 * time.Second,
	}
}

func (c *Client) postJSON(ctx context.Context, path string, in any, out any) error {
	req, err := newRequest(ctx, http.MethodPost, c.BaseURL+path, in, c.UserAgent)
	if err != nil {
		return err
	}
	_, err = doJSON(c.HTTP, req, out)
	return err
}

// PredictBatting sends batting feature rows to the mlCleint service and returns predictions.
func (c *Client) PredictBatting(
	ctx context.Context,
	feats []models.BattingFeatures,
) ([]models.BattingPrediction, error) {
	var preds []models.BattingPrediction
	if err := c.postJSON(ctx, "/predict/batting", feats, &preds); err != nil {
		return nil, err
	}
	return preds, nil
}

// PredictBowling sends bowling feature rows to the mlCleint service and returns predictions.
func (c *Client) PredictBowling(
	ctx context.Context,
	feats []models.BowlingFeatures,
) ([]models.BowlingPrediction, error) {
	var preds []models.BowlingPrediction
	if err := c.postJSON(ctx, "/predict/bowling", feats, &preds); err != nil {
		return nil, err
	}
	return preds, nil
}

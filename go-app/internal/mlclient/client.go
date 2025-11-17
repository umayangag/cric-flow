// Package mlclient provides a typed HTTP client for the Python ML service.
package mlclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/models"
)

// Service calls the Python ML service.
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
		UserAgent: "cric-app-mlclient (+github.com/umayangag/cric-info-scrapers)",
		Timeout:   20 * time.Second,
	}
}

func (c *Client) postJSON(ctx context.Context, path string, in any, out any) error {
	b, err := json.Marshal(in)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("ml-service status: %s", resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// PredictBatting sends batting feature rows to the ML service and returns predictions.
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

// PredictBowling sends bowling feature rows to the ML service and returns predictions.
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

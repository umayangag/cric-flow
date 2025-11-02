package mlclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/predictor"
)

// Client is a client for the ML service.

type Client struct {
	baseURL string
	http    *http.Client
}

// New returns a new ML service client.
func New() *Client {
	return &Client{
		baseURL: "http://localhost:8000",
		http:    http.DefaultClient,
	}
}

// PredictWin calls the /predict-win endpoint of the ML service.
func (c *Client) PredictWin(ctx context.Context, players []predictor.PlayerPrediction) ([]predictor.PlayerPrediction, error) {
	body, err := json.Marshal(players)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/predict-win", bytes.NewReader(tbody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var predictions []predictor.PlayerPrediction
	if err := json.NewDecoder(resp.Body).Decode(&predictions); err != nil {
		return nil, fmt.Errorf("failed to decode response body: %w", err)
	}

	return predictions, nil
}

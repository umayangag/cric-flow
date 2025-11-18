package mlclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/predictor"
)

// PredictWin calls the /predict-win endpoint of the mlCleint service.
func (c *Client) PredictWin(
	ctx context.Context,
	players []predictor.PlayerPrediction,
) ([]predictor.PlayerPrediction, error) {
	body, err := json.Marshal(players)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/predict-win", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var predictions []predictor.PlayerPrediction
	if err := json.NewDecoder(resp.Body).Decode(&predictions); err != nil {
		return nil, fmt.Errorf("failed to decode response body: %w", err)
	}

	return predictions, nil
}

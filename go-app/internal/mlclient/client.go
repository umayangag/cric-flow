// Package mlclient provides a typed HTTP client for the Python mlCleint service.
package mlclient

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/config"
	"github.com/umayangag/cric-flow/go-app/internal/models"
)

// Service calls the Python mlCleint service.
type Client struct {
	BaseURL   string
	HTTP      *http.Client
	UserAgent string
	Timeout   time.Duration
}

// New returns a client with defaults. Override base URL via ML_BASE_URL env; fallback from config server.ml_base_url_fallback.
func New() *Client {
	base := os.Getenv("ML_BASE_URL")
	if base == "" {
		base = config.ServerMLBaseURLFallback(config.Load())
	}
	cfg := config.Load()
	sec := config.ServerMLClientTimeoutSec(cfg)
	timeout := time.Duration(sec) * time.Second
	return &Client{
		BaseURL:   base,
		HTTP:      &http.Client{Timeout: timeout},
		UserAgent: "cric-app-mlclient (+github.com/umayangag/cric-flow)",
		Timeout:   timeout,
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
//
// Deprecated: Use PredictPlayers which calls the unified /ml/backtest/predict endpoint.
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
//
// Deprecated: Use PredictPlayers which calls the unified /ml/backtest/predict endpoint.
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

// UnifiedPlayerPrediction holds per-player predictions from the unified endpoint.
type UnifiedPlayerPrediction struct {
	Runs    float64
	Balls   float64
	Fours   float64
	Sixes   float64
	Wickets float64
	Economy float64
	Catches float64
	RunOuts float64
}

type unifiedPredictRequest struct {
	CutoffDate     string                        `json:"cutoff_date"`
	PlayerIDs      []int64                       `json:"player_ids,omitempty"`
	Format         string                        `json:"format,omitempty"`
	Features       map[string]map[string]float64 `json:"features,omitempty"`
	UseLatestModel bool                          `json:"use_latest_model,omitempty"`
}

type unifiedPlayerPredResponse struct {
	PlayerID int64    `json:"player_id"`
	Runs     float64  `json:"runs,omitempty"`
	Balls    *float64 `json:"balls,omitempty"`
	Fours    *float64 `json:"fours,omitempty"`
	Sixes    *float64 `json:"sixes,omitempty"`
	Wickets  float64  `json:"wickets,omitempty"`
	Economy  float64  `json:"economy,omitempty"`
	Catches  float64  `json:"catches,omitempty"`
	RunOuts  float64  `json:"run_outs,omitempty"`
}

type unifiedPlayersResponse struct {
	Players []unifiedPlayerPredResponse `json:"players"`
}

// PredictPlayers calls the unified /ml/backtest/predict endpoint for batting,
// bowling, and fielding predictions in a single HTTP call.
func (c *Client) PredictPlayers(
	ctx context.Context,
	cutoff time.Time,
	format string,
	playerIDs []int64,
	features map[int64]map[string]float64,
) (map[int64]UnifiedPlayerPrediction, error) {
	var featsStr map[string]map[string]float64
	if len(features) > 0 {
		featsStr = make(map[string]map[string]float64, len(features))
		for pid, m := range features {
			featsStr[strconv.FormatInt(pid, 10)] = m
		}
	}

	// use_latest_model stays true here, and only here. This client serves team
	// selection for a *future* match, so training on the newest data is the correct
	// answer rather than a preference; every other caller was removed with the
	// request parameter (consumer plan W0-2). It is read by ml-service only in the
	// train-on-the-fly fallback, where it rounds the training cutoff to now instead
	// of a cutoff that has not happened yet.
	req := unifiedPredictRequest{
		CutoffDate:     cutoff.Format(time.RFC3339),
		PlayerIDs:      playerIDs,
		Format:         format,
		Features:       featsStr,
		UseLatestModel: true,
	}

	var resp unifiedPlayersResponse
	if err := c.postJSON(ctx, "/ml/backtest/predict", req, &resp); err != nil {
		return nil, fmt.Errorf("predict players: %w", err)
	}

	out := make(map[int64]UnifiedPlayerPrediction, len(resp.Players))
	for _, p := range resp.Players {
		pred := UnifiedPlayerPrediction{
			Runs:    p.Runs,
			Wickets: p.Wickets,
			Economy: p.Economy,
			Catches: p.Catches,
			RunOuts: p.RunOuts,
		}
		if p.Balls != nil {
			pred.Balls = *p.Balls
		}
		if p.Fours != nil {
			pred.Fours = *p.Fours
		}
		if p.Sixes != nil {
			pred.Sixes = *p.Sixes
		}
		out[p.PlayerID] = pred
	}
	return out, nil
}

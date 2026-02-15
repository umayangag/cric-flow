package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// BacktestMLClient is a tiny HTTP client to call the ml-service backtest endpoint.
type BacktestMLClient struct {
	BaseURL string
	HTTP    *http.Client
}

func NewBacktestMLClient() *BacktestMLClient {
	base := os.Getenv("ML_SERVICE_URL")
	if base == "" {
		base = "http://localhost:8000"
	}
	return &BacktestMLClient{BaseURL: base, HTTP: &http.Client{Timeout: 15 * time.Second}}
}

// request/response DTOs kept local to avoid leaking server internals.
type mlBacktestPredictRequest struct {
	Cutoff    string                        `json:"cutoff_date"`
	PlayerIDs []int64                       `json:"player_ids,omitempty"`
	Format    string                        `json:"format,omitempty"`
	Features  map[string]map[string]float64 `json:"features,omitempty"`
}

type mlBacktestPlayerPred struct {
	PlayerID int64   `json:"player_id"`
	Runs     float64 `json:"runs,omitempty"`
	Wickets  float64 `json:"wickets,omitempty"`
	Economy  float64 `json:"economy,omitempty"`
	Catches  float64 `json:"catches,omitempty"`
	RunOuts  float64 `json:"run_outs,omitempty"`
}

type mlBacktestPlayersResponse struct {
	Players []mlBacktestPlayerPred `json:"players"`
}

type mlBacktestMatchAggRequest struct {
	Cutoff string    `json:"cutoff_date"`
	Teams  [2]string `json:"teams"`
}

type mlBacktestMatchAgg struct {
	Runs           float64 `json:"runs"`
	Wickets        float64 `json:"wickets"`
	Extras         float64 `json:"extras"`
	WinnerTeamCode string  `json:"winner_team_code"`
}

type mlBacktestMatchAggResponse struct {
	Match        mlBacktestMatchAgg `json:"match"`
	ModelVersion string             `json:"model_version,omitempty"`
}

// ---------------- Historical match backtest DTOs ----------------

type mlHistoricalMatchFilters struct {
	Format    string `json:"format"`
	Team1     string `json:"team1"`
	Team2     string `json:"team2"`
	MatchDate string `json:"match_date"`
}

type mlHistoricalBacktestRequest struct {
	Cutoff  string                    `json:"cutoff_date"`
	MatchID *int64                    `json:"match_id,omitempty"`
	Filters *mlHistoricalMatchFilters `json:"filters,omitempty"`
}

type mlBacktestPlayerPoint struct {
	Runs    float64  `json:"runs"`
	Wickets *float64 `json:"wickets,omitempty"`
	Economy *float64 `json:"economy,omitempty"`
}

type mlBacktestPlayerComparison struct {
	PlayerID        int64                 `json:"player_id"`
	PlayerName      *string               `json:"player_name,omitempty"`
	Predicted       mlBacktestPlayerPoint `json:"predicted"`
	Actual          mlBacktestPlayerPoint `json:"actual"`
	AbsErrorRuns    float64               `json:"abs_error_runs"`
	AbsErrorWickets *float64              `json:"abs_error_wickets,omitempty"`
}

type mlBacktestMatchComparison struct {
	Predicted mlBacktestMatchAgg `json:"predicted"`
	Actual    mlBacktestMatchAgg `json:"actual"`
}

type mlBacktestMetrics struct {
	MAERuns       float64  `json:"mae_runs"`
	RMSERuns      float64  `json:"rmse_runs"`
	MAEWickets    *float64 `json:"mae_wickets,omitempty"`
	WinnerCorrect *bool    `json:"winner_correct,omitempty"`
}

type mlHistoricalBacktestResponse struct {
	Players      []mlBacktestPlayerComparison `json:"players"`
	Match        mlBacktestMatchComparison    `json:"match"`
	Metrics      mlBacktestMetrics            `json:"metrics"`
	ModelVersion string                       `json:"model_version"`
}

// HistoricalMatchFilters represents selector parameters for a historical match.
type HistoricalMatchFilters struct {
	Format    string
	Team1     string
	Team2     string
	MatchDate time.Time
}

// HistoricalBacktestResult is a thin wrapper of the ML response for consumers.
type HistoricalBacktestResult struct {
	Players      []mlBacktestPlayerComparison
	Match        mlBacktestMatchComparison
	Metrics      mlBacktestMetrics
	ModelVersion string
}

// predictPlayers calls the ML backtest endpoint to get player-level predictions.
// When format is non-empty and features is non-nil, they are sent so the ML service can run the full pipeline (real models).
func (c *BacktestMLClient) predictPlayers(
	ctx context.Context,
	cutoff time.Time,
	format string,
	playerIDs []int64,
	features map[int64]map[string]float64,
) (map[int64]playerPredictions, error) {
	if len(playerIDs) == 0 {
		return map[int64]playerPredictions{}, nil
	}
	body := mlBacktestPredictRequest{
		Cutoff:    cutoff.Format(time.RFC3339),
		PlayerIDs: playerIDs,
		Format:    strings.TrimSpace(format),
	}
	if len(features) > 0 {
		body.Features = make(map[string]map[string]float64, len(features))
		for pid, m := range features {
			body.Features[strconv.FormatInt(pid, 10)] = m
		}
	}
	payload, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.BaseURL+"/ml/backtest/predict",
		bytes.NewReader(payload),
	)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ml backtest predict http %d", resp.StatusCode)
	}
	var out mlBacktestPlayersResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	res := make(map[int64]playerPredictions, len(out.Players))
	for _, p := range out.Players {
		res[p.PlayerID] = playerPredictions{
			Runs:    p.Runs,
			Wickets: p.Wickets,
			Economy: p.Economy,
			Catches: p.Catches,
			RunOuts: p.RunOuts,
		}
	}
	return res, nil
}

// predictMatchAggregates calls the ML backtest endpoint to get match-level aggregate predictions.
func (c *BacktestMLClient) predictMatchAggregates(
	ctx context.Context,
	cutoff time.Time,
	teams [2]string,
) (matchAggregates, string, error) {
	if teams[0] == "" || teams[1] == "" {
		return matchAggregates{}, "", errors.New("teams required")
	}
	body := mlBacktestMatchAggRequest{Cutoff: cutoff.Format(time.RFC3339), Teams: teams}
	payload, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.BaseURL+"/ml/backtest/predict",
		bytes.NewReader(payload),
	)
	if err != nil {
		return matchAggregates{}, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return matchAggregates{}, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return matchAggregates{}, "", fmt.Errorf("ml backtest match http %d", resp.StatusCode)
	}
	var out mlBacktestMatchAggResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return matchAggregates{}, "", err
	}
	return matchAggregates{
		Runs:           out.Match.Runs,
		Wickets:        out.Match.Wickets,
		Extras:         out.Match.Extras,
		WinnerTeamCode: out.Match.WinnerTeamCode,
	}, out.ModelVersion, nil
}

// historicalMatchBacktest calls the ML service to evaluate a specific, already-played match.
// Exactly one of matchID or filters must be provided (the other must be nil/zero).
func (c *BacktestMLClient) historicalMatchBacktest(
	ctx context.Context,
	cutoff time.Time,
	matchID *int64,
	filters *HistoricalMatchFilters,
) (HistoricalBacktestResult, error) {
	// Validate selector
	hasID := matchID != nil && *matchID > 0
	hasFilters := filters != nil && filters.Format != "" && filters.Team1 != "" && filters.Team2 != ""
	if (hasID && hasFilters) || (!hasID && !hasFilters) {
		return HistoricalBacktestResult{}, errors.New("provide exactly one of matchID or filters")
	}
	reqBody := mlHistoricalBacktestRequest{Cutoff: cutoff.Format(time.RFC3339)}
	if hasID {
		reqBody.MatchID = matchID
	} else if hasFilters {
		reqBody.Filters = &mlHistoricalMatchFilters{
			Format:    filters.Format,
			Team1:     filters.Team1,
			Team2:     filters.Team2,
			MatchDate: filters.MatchDate.Format(time.RFC3339),
		}
	}
	payload, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.BaseURL+"/ml/backtest/match",
		bytes.NewReader(payload),
	)
	if err != nil {
		return HistoricalBacktestResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return HistoricalBacktestResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return HistoricalBacktestResult{}, fmt.Errorf("ml historical backtest http %d", resp.StatusCode)
	}
	var out mlHistoricalBacktestResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return HistoricalBacktestResult{}, err
	}
	return HistoricalBacktestResult(out), nil
}

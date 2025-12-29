package server

import (
    "bytes"
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "net/http"
    "os"
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
    Cutoff   string  `json:"cutoff_date"`
    PlayerIDs []int64 `json:"player_ids,omitempty"`
}

type mlBacktestPlayerPred struct {
    PlayerID int64    `json:"player_id"`
    Runs     float64  `json:"runs,omitempty"`
    Wickets  float64  `json:"wickets,omitempty"`
    Economy  float64  `json:"economy,omitempty"`
    Catches  float64  `json:"catches,omitempty"`
    RunOuts  float64  `json:"run_outs,omitempty"`
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
    Match mlBacktestMatchAgg `json:"match"`
}

// PredictPlayers calls the ML backtest endpoint to get player-level predictions.
func (c *BacktestMLClient) PredictPlayers(ctx context.Context, cutoff time.Time, playerIDs []int64) (map[int64]playerPredictions, error) {
    if len(playerIDs) == 0 {
        return map[int64]playerPredictions{}, nil
    }
    body := mlBacktestPredictRequest{Cutoff: cutoff.Format(time.RFC3339), PlayerIDs: playerIDs}
    payload, _ := json.Marshal(body)
    req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/ml/backtest/predict", bytes.NewReader(payload))
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
        res[p.PlayerID] = playerPredictions{Runs: p.Runs, Wickets: p.Wickets, Economy: p.Economy, Catches: p.Catches, RunOuts: p.RunOuts}
    }
    return res, nil
}

// PredictMatchAggregates calls the ML backtest endpoint to get match-level aggregate predictions.
func (c *BacktestMLClient) PredictMatchAggregates(ctx context.Context, cutoff time.Time, teams [2]string) (matchAggregates, error) {
    if teams[0] == "" || teams[1] == "" {
        return matchAggregates{}, errors.New("teams required")
    }
    body := mlBacktestMatchAggRequest{Cutoff: cutoff.Format(time.RFC3339), Teams: teams}
    payload, _ := json.Marshal(body)
    req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/ml/backtest/predict", bytes.NewReader(payload))
    if err != nil {
        return matchAggregates{}, err
    }
    req.Header.Set("Content-Type", "application/json")
    resp, err := c.HTTP.Do(req)
    if err != nil {
        return matchAggregates{}, err
    }
    defer resp.Body.Close()
    if resp.StatusCode < 200 || resp.StatusCode >= 300 {
        return matchAggregates{}, fmt.Errorf("ml backtest match http %d", resp.StatusCode)
    }
    var out mlBacktestMatchAggResponse
    if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
        return matchAggregates{}, err
    }
    return matchAggregates{
        Runs:           out.Match.Runs,
        Wickets:        out.Match.Wickets,
        Extras:         out.Match.Extras,
        WinnerTeamCode: out.Match.WinnerTeamCode,
    }, nil
}

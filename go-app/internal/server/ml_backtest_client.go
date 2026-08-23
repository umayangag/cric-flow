package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/services/backtest"
	"github.com/umayangag/cric-flow/go-app/internal/services/predictteam"
)

// BacktestMLClient is a tiny HTTP client to call the ml-service backtest endpoint.
type BacktestMLClient struct {
	BaseURL string
	HTTP    *http.Client
}

// backtestMLClientTimeout: train-on-the-fly can take minutes (fetch data, train, predict). Use a long timeout so the client does not exceed deadline before the ML service responds.
const backtestMLClientTimeout = 6 * time.Hour

func NewBacktestMLClient() *BacktestMLClient {
	base := os.Getenv("ML_SERVICE_URL")
	if base == "" {
		base = "http://localhost:8000"
	}
	return &BacktestMLClient{BaseURL: base, HTTP: &http.Client{Timeout: backtestMLClientTimeout}}
}

// mlBacktestMatchContext is optional match context for hybrid reconciliation.
type mlBacktestMatchContext struct {
	Team1PlayerIDs    []int64 `json:"team1_player_ids"`
	Team2PlayerIDs    []int64 `json:"team2_player_ids"`
	VenueID           float64 `json:"venue_id,omitempty"`
	FormatID          float64 `json:"format_id,omitempty"`
	Team1OppositionID float64 `json:"team1_opposition_id,omitempty"`
	Team2OppositionID float64 `json:"team2_opposition_id,omitempty"`
	Temp              int     `json:"temp,omitempty"`
	Wind              int     `json:"wind,omitempty"`
	Rain              int     `json:"rain,omitempty"`
	Humidity          int     `json:"humidity,omitempty"`
	Cloud             int     `json:"cloud,omitempty"`
	Pressure          int     `json:"pressure,omitempty"`
	Viscosity         int     `json:"viscosity,omitempty"`
}

// request/response DTOs kept local to avoid leaking server internals.
type mlBacktestPredictRequest struct {
	Cutoff         string                        `json:"cutoff_date"`
	PlayerIDs      []int64                       `json:"player_ids,omitempty"`
	Format         string                        `json:"format,omitempty"`
	Features       map[string]map[string]float64 `json:"features,omitempty"`
	UseLatestModel bool                          `json:"use_latest_model,omitempty"`
	MatchContext   *mlBacktestMatchContext       `json:"match_context,omitempty"`
}

type mlBacktestPlayerPred struct {
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

type mlBacktestPlayersResponse struct {
	Players []mlBacktestPlayerPred `json:"players"`
}

// mlGenerateMatchRequest matches ml-service GenerateMatchRequest (POST /api/ml/generate-match).
type mlGenerateMatchRequest struct {
	CutoffDate     string                        `json:"cutoff_date"`
	PlayerIDs      []int64                       `json:"player_ids"`
	Format         string                        `json:"format"`
	Features       map[string]map[string]float64 `json:"features,omitempty"`
	MatchContext   *mlBacktestMatchContext       `json:"match_context,omitempty"`
	UseLatestModel bool                          `json:"use_latest_model,omitempty"`
}

// mlInningsSummary matches ml-service InningsSummary.
type mlInningsSummary struct {
	InningNumber int     `json:"inning_number"`
	Runs         float64 `json:"runs"`
	Wickets      float64 `json:"wickets"`
}

// MlGenerateMatchResponse matches ml-service GenerateMatchResponse.
// Exported so the public GenerateMatch method can return it.
type MlGenerateMatchResponse struct {
	Players             []mlBacktestPlayerPred `json:"players"`
	Innings             []mlInningsSummary     `json:"innings"`
	WinProbabilityTeam1 float64                `json:"win_probability_team1"`
	ModelVersion        string                 `json:"model_version"`
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

// mlWinFeatures matches ML service WinFeatures (POST /predict/win request body element).
type mlWinFeatures struct {
	FormatID                int     `json:"format_id"`
	VenueID                 int     `json:"venue_id"`
	Team1OppositionID       int     `json:"team1_opposition_id"`
	Team2OppositionID       int     `json:"team2_opposition_id"`
	TossWinnerOppositionID  int     `json:"toss_winner_opposition_id"`
	Temp                    int     `json:"temp"`
	Wind                    int     `json:"wind"`
	Rain                    int     `json:"rain"`
	Humidity                int     `json:"humidity"`
	Cloud                   int     `json:"cloud"`
	Pressure                int     `json:"pressure"`
	Viscosity               int     `json:"viscosity"`
	Team1BatConsistencySum  float64 `json:"team1_bat_consistency_sum"`
	Team1BowlConsistencySum float64 `json:"team1_bowl_consistency_sum"`
	Team2BatConsistencySum  float64 `json:"team2_bat_consistency_sum"`
	Team2BowlConsistencySum float64 `json:"team2_bowl_consistency_sum"`
	Team1BatFormSum         float64 `json:"team1_bat_form_sum"`
	Team1BowlFormSum        float64 `json:"team1_bowl_form_sum"`
	Team2BatFormSum         float64 `json:"team2_bat_form_sum"`
	Team2BowlFormSum        float64 `json:"team2_bowl_form_sum"`
	Format                  string  `json:"format,omitempty"`
}

// mlWinFeaturesEnhanced matches the ML service WinFeaturesEnhanced request body
// for POST /predict/win-enhanced. Sends per-player feature maps for on-the-fly aggregation.
type mlWinFeaturesEnhanced struct {
	FormatID               int                           `json:"format_id"`
	VenueID                int                           `json:"venue_id"`
	Team1OppositionID      int                           `json:"team1_opposition_id"`
	Team2OppositionID      int                           `json:"team2_opposition_id"`
	TossWinnerOppositionID int                           `json:"toss_winner_opposition_id"`
	Temp                   int                           `json:"temp"`
	Wind                   int                           `json:"wind"`
	Rain                   int                           `json:"rain"`
	Humidity               int                           `json:"humidity"`
	Cloud                  int                           `json:"cloud"`
	Pressure               int                           `json:"pressure"`
	Viscosity              int                           `json:"viscosity"`
	Team1PlayerFeatures    map[string]map[string]float64 `json:"team1_player_features"`
	Team2PlayerFeatures    map[string]map[string]float64 `json:"team2_player_features"`
	Format                 string                        `json:"format,omitempty"`
}

// mlTeamOptPoolPlayer is a single player in the optimization pool (JSON wire DTO).
type mlTeamOptPoolPlayer struct {
	PlayerID   int64              `json:"player_id"`
	Name       string             `json:"name"`
	IsBowler   bool               `json:"is_bowler"`
	IsKeeper   bool               `json:"is_keeper"`
	BatScore   float64            `json:"bat_score"`
	BowlScore  float64            `json:"bowl_score"`
	FieldScore float64            `json:"field_score"`
	Features   map[string]float64 `json:"features"`
}

type mlTeamOptWeights struct {
	Bat         float64 `json:"bat"`
	Bowl        float64 `json:"bowl"`
	Field       float64 `json:"field"`
	KeeperBonus float64 `json:"keeper_bonus"`
}

type mlTeamOptConstraints struct {
	Size          int  `json:"size"`
	MinBowlers    int  `json:"min_bowlers"`
	RequireKeeper bool `json:"require_keeper"`
}

// mlTeamOptRequest is the JSON body for POST /optimize/team-selection.
type mlTeamOptRequest struct {
	Pool             []mlTeamOptPoolPlayer         `json:"pool"`
	OpponentFeatures map[string]map[string]float64 `json:"opponent_features"`
	MatchContext     map[string]float64            `json:"match_context"`
	Constraints      mlTeamOptConstraints          `json:"constraints"`
	Weights          mlTeamOptWeights              `json:"weights"`
	TeamIsTeam1      bool                          `json:"team_is_team1"`
	Format           string                        `json:"format,omitempty"`
	MaxIterations    int                           `json:"max_iterations"`
	MaxEvals         int                           `json:"max_evals"`
}

type mlTeamOptSelectedPlayer struct {
	PlayerID int64  `json:"player_id"`
	Name     string `json:"name"`
}

type mlTeamOptResponse struct {
	Selected       []mlTeamOptSelectedPlayer `json:"selected"`
	WinProbability float64                   `json:"win_probability"`
	IterationsUsed int                       `json:"iterations_used"`
	EvalsPerformed int                       `json:"evals_performed"`
}

// mlWinPrediction matches ML service WinPrediction (POST /predict/win response element).
type mlWinPrediction struct {
	Team1WinProbability float64 `json:"team1_win_probability"`
}

// mlErrorDetail is a subset of the ML service error response (FastAPI sends {"detail": ...}).
type mlErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint"`
}

// logMLNon2xx reads the response body, logs status and body for debugging, and returns an error
// that includes status and a short message extracted from the body if present.
func logMLNon2xx(resp *http.Response, endpoint string) error {
	const maxResponseBody = 1 << 20 // 1 MiB
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	if err != nil {
		slog.Error("ml service non-2xx: failed to read response body",
			slog.String("endpoint", endpoint),
			slog.Int("status", resp.StatusCode),
			slog.Any("err", err))
		return fmt.Errorf("%s http %d (body read failed: %v)", endpoint, resp.StatusCode, err)
	}
	bodyStr := string(body)
	const maxLog = 2000
	if len(bodyStr) > maxLog {
		bodyStr = bodyStr[:maxLog] + "..."
	}
	slog.Error("ml service returned non-2xx",
		slog.String("endpoint", endpoint),
		slog.Int("status", resp.StatusCode),
		slog.String("body", bodyStr))

	// Try to extract a short message from FastAPI-style {"detail": "..."} or {"detail": {"code","message","hint"}}
	var detail struct {
		Detail json.RawMessage `json:"detail"`
	}
	if err := json.Unmarshal(body, &detail); err == nil && len(detail.Detail) > 0 {
		var s string
		if err := json.Unmarshal(detail.Detail, &s); err == nil {
			return fmt.Errorf("%s http %d: %s", endpoint, resp.StatusCode, s)
		}
		var d mlErrorDetail
		if err := json.Unmarshal(detail.Detail, &d); err == nil {
			msg := d.Message
			if d.Hint != "" {
				msg += " — " + d.Hint
			}
			return fmt.Errorf("%s http %d: %s", endpoint, resp.StatusCode, msg)
		}
	}
	return fmt.Errorf("%s http %d: %s", endpoint, resp.StatusCode, strings.TrimSpace(bodyStr))
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

// MatchContextForReconciliation holds data for hybrid innings reconciliation.
type MatchContextForReconciliation struct {
	Team1PlayerIDs                                         []int64
	Team2PlayerIDs                                         []int64
	VenueID                                                int64
	FormatID                                               int64
	Team1OppositionID                                      int64 // team2's ID when team1 bats (innings 1)
	Team2OppositionID                                      int64 // team1's ID when team2 bats (innings 2)
	Temp, Wind, Rain, Humidity, Cloud, Pressure, Viscosity int
}

// predictPlayers calls the ML backtest endpoint to get player-level predictions.
// When format is non-empty and features is non-nil, they are sent so the ML service can run the full pipeline (real models).
// useLatestModel: when true, ML uses the latest available model (may include post-cutoff training data).
// matchCtx: when non-nil and innings model is loaded, ML rescales predictions for consistency.
func (c *BacktestMLClient) predictPlayers(
	ctx context.Context,
	cutoff time.Time,
	format string,
	playerIDs []int64,
	features map[int64]map[string]float64,
	useLatestModel bool,
	matchCtx *MatchContextForReconciliation,
) (map[int64]playerPredictions, error) {
	if len(playerIDs) == 0 {
		return map[int64]playerPredictions{}, nil
	}
	body := mlBacktestPredictRequest{
		Cutoff:         cutoff.Format(time.RFC3339),
		PlayerIDs:      playerIDs,
		Format:         strings.TrimSpace(format),
		UseLatestModel: useLatestModel,
	}
	if len(features) > 0 {
		body.Features = make(map[string]map[string]float64, len(features))
		for pid, m := range features {
			body.Features[strconv.FormatInt(pid, 10)] = m
		}
	}
	if matchCtx != nil {
 	body.MatchContext = &mlBacktestMatchContext{
			Team1PlayerIDs:    matchCtx.Team1PlayerIDs,
			Team2PlayerIDs:    matchCtx.Team2PlayerIDs,
			VenueID:           float64(matchCtx.VenueID),
			FormatID:          float64(matchCtx.FormatID),
			Team1OppositionID: float64(matchCtx.Team1OppositionID),
			Team2OppositionID: float64(matchCtx.Team2OppositionID),
			Temp:              matchCtx.Temp,
			Wind:              matchCtx.Wind,
			Rain:              matchCtx.Rain,
			Humidity:          matchCtx.Humidity,
			Cloud:             matchCtx.Cloud,
			Pressure:          matchCtx.Pressure,
			Viscosity:         matchCtx.Viscosity,
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
		return nil, logMLNon2xx(resp, "ml backtest predict")
	}
	var out mlBacktestPlayersResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	res := make(map[int64]playerPredictions, len(out.Players))
	for _, p := range out.Players {
		pp := playerPredictions{
			Runs:    p.Runs,
			Wickets: p.Wickets,
			Economy: p.Economy,
			Catches: p.Catches,
			RunOuts: p.RunOuts,
		}
		if p.Balls != nil {
			pp.Balls = *p.Balls
		}
		if p.Fours != nil {
			pp.Fours = *p.Fours
		}
		if p.Sixes != nil {
			pp.Sixes = *p.Sixes
		}
		res[p.PlayerID] = pp
	}
	return res, nil
}

// ---------------------------------------------------------------------------
// Batch prediction (POST /ml/backtest/predict-batch)
// ---------------------------------------------------------------------------

type mlBatchPredictItem struct {
	CutoffDate     string                        `json:"cutoff_date"`
	PlayerIDs      []int64                       `json:"player_ids"`
	Format         string                        `json:"format"`
	Features       map[string]map[string]float64 `json:"features,omitempty"`
	UseLatestModel bool                          `json:"use_latest_model,omitempty"`
}

type mlBatchPredictRequest struct {
	Requests []mlBatchPredictItem `json:"requests"`
}

type mlBatchPredictResultItem struct {
	Players []mlBacktestPlayerPred `json:"players"`
}

type mlBatchPredictResponse struct {
	Results []mlBatchPredictResultItem `json:"results"`
}

// BatchPredictPlayersInput holds the inputs for one item in a batch prediction.
type BatchPredictPlayersInput struct {
	Cutoff         time.Time
	Format         string
	PlayerIDs      []int64
	Features       map[int64]map[string]float64
	UseLatestModel bool
}

// PredictPlayersBatch sends multiple prediction requests to the ML service
// in a single HTTP call, returning one result map per input item.
func (c *BacktestMLClient) PredictPlayersBatch(
	ctx context.Context,
	inputs []BatchPredictPlayersInput,
) ([]map[int64]backtest.PlayerPredictions, error) {
	if len(inputs) == 0 {
		return nil, nil
	}

	items := make([]mlBatchPredictItem, 0, len(inputs))
	for _, in := range inputs {
		item := mlBatchPredictItem{
			CutoffDate:     in.Cutoff.Format(time.RFC3339),
			PlayerIDs:      in.PlayerIDs,
			Format:         strings.TrimSpace(in.Format),
			UseLatestModel: in.UseLatestModel,
		}
		if len(in.Features) > 0 {
			item.Features = make(map[string]map[string]float64, len(in.Features))
			for pid, m := range in.Features {
				item.Features[strconv.FormatInt(pid, 10)] = m
			}
		}
		items = append(items, item)
	}

	payload, _ := json.Marshal(mlBatchPredictRequest{Requests: items})
	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost,
		c.BaseURL+"/ml/backtest/predict-batch",
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
		return nil, logMLNon2xx(resp, "ml batch predict")
	}
	var out mlBatchPredictResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if len(out.Results) != len(inputs) {
		return nil, fmt.Errorf("batch predict: got %d results, expected %d", len(out.Results), len(inputs))
	}
	results := make([]map[int64]backtest.PlayerPredictions, len(out.Results))
	for i, result := range out.Results {
		m := make(map[int64]backtest.PlayerPredictions, len(result.Players))
		for _, p := range result.Players {
			pp := backtest.PlayerPredictions{
				Runs:    p.Runs,
				Wickets: p.Wickets,
				Economy: p.Economy,
				Catches: p.Catches,
				RunOuts: p.RunOuts,
			}
			if p.Balls != nil {
				pp.Balls = *p.Balls
			}
			if p.Fours != nil {
				pp.Fours = *p.Fours
			}
			if p.Sixes != nil {
				pp.Sixes = *p.Sixes
			}
			m[p.PlayerID] = pp
		}
		results[i] = m
	}
	return results, nil
}

// GenerateMatch calls the ml-service /api/ml/generate-match endpoint to get a reconciled
// per-player projection, innings totals, and win probability for a future match.
// playerIDs must include all players in the match (typically both XIs).
// features is a per-player feature map (player_id -> feature_name -> value).
// When useLatestModel is true, ML may ignore the strict cutoff and use the latest available model.
func (c *BacktestMLClient) GenerateMatch(
	ctx context.Context,
	cutoff time.Time,
	format string,
	playerIDs []int64,
	features map[int64]map[string]float64,
	useLatestModel bool,
	matchCtx *MatchContextForReconciliation,
) (MlGenerateMatchResponse, error) {
	if len(playerIDs) == 0 {
		return MlGenerateMatchResponse{}, errors.New("playerIDs required")
	}
	reqBody := mlGenerateMatchRequest{
		CutoffDate:     cutoff.Format(time.RFC3339),
		PlayerIDs:      playerIDs,
		Format:         strings.TrimSpace(format),
		UseLatestModel: useLatestModel,
	}
	if len(features) > 0 {
		reqBody.Features = make(map[string]map[string]float64, len(features))
		for pid, m := range features {
			reqBody.Features[strconv.FormatInt(pid, 10)] = m
		}
	}
	if matchCtx != nil {
 	reqBody.MatchContext = &mlBacktestMatchContext{
			Team1PlayerIDs:    matchCtx.Team1PlayerIDs,
			Team2PlayerIDs:    matchCtx.Team2PlayerIDs,
			VenueID:           float64(matchCtx.VenueID),
			FormatID:          float64(matchCtx.FormatID),
			Team1OppositionID: float64(matchCtx.Team1OppositionID),
			Team2OppositionID: float64(matchCtx.Team2OppositionID),
			Temp:              matchCtx.Temp,
			Wind:              matchCtx.Wind,
			Rain:              matchCtx.Rain,
			Humidity:          matchCtx.Humidity,
			Cloud:             matchCtx.Cloud,
			Pressure:          matchCtx.Pressure,
			Viscosity:         matchCtx.Viscosity,
		}
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return MlGenerateMatchResponse{}, fmt.Errorf("failed to marshal generate match request: %w", err)
	}
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.BaseURL+"/api/ml/generate-match",
		bytes.NewReader(payload),
	)
	if err != nil {
		return MlGenerateMatchResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return MlGenerateMatchResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return MlGenerateMatchResponse{}, logMLNon2xx(resp, "api/ml/generate-match")
	}
	var out MlGenerateMatchResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return MlGenerateMatchResponse{}, err
	}
	return out, nil
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
		return matchAggregates{}, "", logMLNon2xx(resp, "ml backtest match")
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

// PredictMatchWin calls the ML service POST /predict/win with one WinFeatures row.
// Returns team1 (batting first) win probability in [0,1]. Returns error if win model is not loaded or request fails.
func (c *BacktestMLClient) PredictMatchWin(ctx context.Context, features mlWinFeatures) (float64, error) {
	body := []mlWinFeatures{features}
	payload, err := json.Marshal(body)
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/predict/win", bytes.NewReader(payload))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, logMLNon2xx(resp, "predict/win")
	}
	var out []mlWinPrediction
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, err
	}
	if len(out) == 0 {
		return 0, errors.New("predict/win: empty response")
	}
	return out[0].Team1WinProbability, nil
}

// PredictMatchWinEnhanced calls POST /predict/win-enhanced with per-player feature maps.
// Returns team1 (batting first) win probability.
func (c *BacktestMLClient) PredictMatchWinEnhanced(
	ctx context.Context,
	features mlWinFeaturesEnhanced,
) (float64, error) {
	payload, err := json.Marshal(features)
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.BaseURL+"/predict/win-enhanced",
		bytes.NewReader(payload),
	)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, logMLNon2xx(resp, "predict/win-enhanced")
	}
	var out mlWinPrediction
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, err
	}
	return out.Team1WinProbability, nil
}

// OptimizeTeamSelection calls POST /optimize/team-selection to run server-side
// hill-climb team optimisation with batch model inference.
func (c *BacktestMLClient) OptimizeTeamSelection(
	ctx context.Context,
	req predictteam.TeamOptimizationRequest,
) (*predictteam.TeamOptimizationResult, error) {
	mlPool := make([]mlTeamOptPoolPlayer, len(req.Pool))
	for i, p := range req.Pool {
		mlPool[i] = mlTeamOptPoolPlayer{
			PlayerID:   p.PlayerID,
			Name:       p.Name,
			IsBowler:   p.IsBowler,
			IsKeeper:   p.IsKeeper,
			BatScore:   p.BatScore,
			BowlScore:  p.BowlScore,
			FieldScore: p.FieldScore,
			Features:   p.Features,
		}
	}
	oppFeats := make(map[string]map[string]float64, len(req.OpponentFeatures))
	for pid, feats := range req.OpponentFeatures {
		oppFeats[strconv.FormatInt(pid, 10)] = feats
	}
	mlReq := mlTeamOptRequest{
		Pool:             mlPool,
		OpponentFeatures: oppFeats,
		MatchContext:     req.MatchContext,
		Constraints: mlTeamOptConstraints{
			Size:          req.Constraints.Size,
			MinBowlers:    req.Constraints.MinBowlers,
			RequireKeeper: req.Constraints.RequireKeeper,
		},
		Weights: mlTeamOptWeights{
			Bat:         req.Weights.Bat,
			Bowl:        req.Weights.Bowl,
			Field:       req.Weights.Field,
			KeeperBonus: req.Weights.KeeperBonus,
		},
		TeamIsTeam1:   req.TeamIsTeam1,
		Format:        req.Format,
		MaxIterations: req.MaxIterations,
		MaxEvals:      req.MaxEvals,
	}

	payload, err := json.Marshal(mlReq)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost,
		c.BaseURL+"/optimize/team-selection",
		bytes.NewReader(payload),
	)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, logMLNon2xx(resp, "optimize/team-selection")
	}
	var out mlTeamOptResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	selected := make([]predictteam.TeamOptSelectedPlayer, len(out.Selected))
	for i, s := range out.Selected {
		selected[i] = predictteam.TeamOptSelectedPlayer{PlayerID: s.PlayerID, Name: s.Name}
	}
	return &predictteam.TeamOptimizationResult{
		Selected:       selected,
		WinProbability: out.WinProbability,
		IterationsUsed: out.IterationsUsed,
		EvalsPerformed: out.EvalsPerformed,
	}, nil
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
		return HistoricalBacktestResult{}, logMLNon2xx(resp, "ml historical backtest")
	}
	var out mlHistoricalBacktestResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return HistoricalBacktestResult{}, err
	}
	return HistoricalBacktestResult(out), nil
}

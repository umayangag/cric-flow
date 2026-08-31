package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/umayangag/cric-flow/go-app/internal/services/predictteam"
)

// Wire shapes for the ml-service /xi/* endpoints (app/models/xi.py).

type mlXIConstraints struct {
	TeamSize      int     `json:"team_size"`
	MinBowlers    int     `json:"min_bowlers"`
	RequireKeeper bool    `json:"require_keeper"`
	MustInclude   []int64 `json:"must_include"`
	MustExclude   []int64 `json:"must_exclude"`
}

type mlXIOptimizeRequest struct {
	Format            string          `json:"format"`
	PoolPlayerIDs     []int64         `json:"pool_player_ids"`
	OpponentPlayerIDs []int64         `json:"opponent_player_ids"`
	TeamIsTeam1       bool            `json:"team_is_team1"`
	Constraints       mlXIConstraints `json:"constraints"`
	MaxEvaluations    int             `json:"max_evaluations"`
}

type mlXIOptimizeResponse struct {
	SelectedPlayerIDs []int64            `json:"selected_player_ids"`
	WinProbability    float64            `json:"win_probability"`
	Evaluations       int                `json:"evaluations"`
	ImprovedOverSeed  float64            `json:"improved_over_seed"`
	UnknownPlayerIDs  []int64            `json:"unknown_player_ids"`
	MarginalValues    map[string]float64 `json:"marginal_values"`
}

type mlXIWinRequest struct {
	Format         string  `json:"format"`
	Team1PlayerIDs []int64 `json:"team1_player_ids"`
	Team2PlayerIDs []int64 `json:"team2_player_ids"`
	Team1ID        *int64  `json:"team1_id,omitempty"`
	Team2ID        *int64  `json:"team2_id,omitempty"`
	VenueID        *int64  `json:"venue_id,omitempty"`
}

type mlXIWinResponse struct {
	Team1WinProbability  float64 `json:"team1_win_probability"`
	ObjectiveProbability float64 `json:"objective_probability"`
}

// OptimizeXI calls POST /xi/optimize: server-side selection over player ids against the
// XI-responsive win model.
func (c *BacktestMLClient) OptimizeXI(
	ctx context.Context,
	req predictteam.XIOptimizationRequest,
) (*predictteam.XIOptimizationResult, error) {
	maxEvals := req.MaxEvaluations
	if maxEvals < 100 {
		maxEvals = 20000
	}
	payload, err := json.Marshal(mlXIOptimizeRequest{
		Format:            req.Format,
		PoolPlayerIDs:     req.PoolPlayerIDs,
		OpponentPlayerIDs: req.OpponentPlayerIDs,
		TeamIsTeam1:       req.TeamIsTeam1,
		Constraints: mlXIConstraints{
			TeamSize:      req.Constraints.Size,
			MinBowlers:    req.Constraints.MinBowlers,
			RequireKeeper: req.Constraints.RequireKeeper,
			MustInclude:   []int64{},
			MustExclude:   []int64{},
		},
		MaxEvaluations: maxEvals,
	})
	if err != nil {
		return nil, err
	}
	var out mlXIOptimizeResponse
	if err := c.postJSON(ctx, "/xi/optimize", payload, &out); err != nil {
		return nil, err
	}
	marginal := make(map[int64]float64, len(out.MarginalValues))
	for k, v := range out.MarginalValues {
		if pid, perr := strconv.ParseInt(k, 10, 64); perr == nil {
			marginal[pid] = v
		}
	}
	return &predictteam.XIOptimizationResult{
		SelectedPlayerIDs: out.SelectedPlayerIDs,
		WinProbability:    out.WinProbability,
		Evaluations:       out.Evaluations,
		ImprovedOverSeed:  out.ImprovedOverSeed,
		UnknownPlayerIDs:  out.UnknownPlayerIDs,
		MarginalValues:    marginal,
	}, nil
}

// PredictMatchWinXI calls POST /xi/predict-win and returns the displayed P(team1 wins).
func (c *BacktestMLClient) PredictMatchWinXI(ctx context.Context, req predictteam.XIWinRequest) (float64, error) {
	payload, err := json.Marshal(mlXIWinRequest{
		Format:         req.Format,
		Team1PlayerIDs: req.Team1PlayerIDs,
		Team2PlayerIDs: req.Team2PlayerIDs,
		Team1ID:        optionalID(req.Team1ID),
		Team2ID:        optionalID(req.Team2ID),
		VenueID:        optionalID(req.VenueID),
	})
	if err != nil {
		return 0, err
	}
	var out mlXIWinResponse
	if err := c.postJSON(ctx, "/xi/predict-win", payload, &out); err != nil {
		return 0, err
	}
	return out.Team1WinProbability, nil
}

func optionalID(id int64) *int64 {
	if id <= 0 {
		return nil
	}
	return &id
}

// postJSON posts a JSON body to the ml-service and decodes a 2xx JSON response into out.
func (c *BacktestMLClient) postJSON(ctx context.Context, path string, payload []byte, out interface{}) error {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return logMLNon2xx(resp, path)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

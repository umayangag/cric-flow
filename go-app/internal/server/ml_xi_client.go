package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

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
	// AsOf (YYYY-MM-DD) asks for ratings as of that date; omitted = through today.
	AsOf string `json:"as_of,omitempty"`
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
	// AsOf (YYYY-MM-DD) asks for ratings as of that date; omitted = through today.
	AsOf string `json:"as_of,omitempty"`
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
		AsOf:           asOfParam(req.AsOf),
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
		AsOf:           asOfParam(req.AsOf),
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

// asOfParam renders an as-of date for the wire; the zero time means "through today"
// and is omitted.
func asOfParam(asOf time.Time) string {
	if asOf.IsZero() {
		return ""
	}
	return asOf.Format("2006-01-02")
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

// Wire shapes for POST /simulate (app/models/xi.py: SimulateRequest / SimulateResponse).

type mlSimulateRequest struct {
	Format         string  `json:"format"`
	Team1PlayerIDs []int64 `json:"team1_player_ids"`
	Team2PlayerIDs []int64 `json:"team2_player_ids"`
	Team1ID        *int64  `json:"team1_id,omitempty"`
	Team2ID        *int64  `json:"team2_id,omitempty"`
	VenueID        *int64  `json:"venue_id,omitempty"`
	Team1BatsFirst *bool   `json:"team1_bats_first,omitempty"`
	AsOf           string  `json:"as_of,omitempty"`
	NSamples       int     `json:"n_samples,omitempty"`
	Seed           int     `json:"seed"`
}

type mlSimulatedRange struct {
	Q10    float64 `json:"q10"`
	Median float64 `json:"median"`
	Q90    float64 `json:"q90"`
}

type mlSimulatedTotal struct {
	mlSimulatedRange
	Mean      float64 `json:"mean"`
	Sd        float64 `json:"sd"`
	Scorecard float64 `json:"scorecard"`
}

type mlSimulatedScorecardLine struct {
	Runs         float64 `json:"runs"`
	BallsFaced   float64 `json:"balls_faced"`
	Wickets      float64 `json:"wickets"`
	RunsConceded float64 `json:"runs_conceded"`
	BallsBowled  float64 `json:"balls_bowled"`
}

type mlSimulatedPlayer struct {
	PlayerID     int64                    `json:"player_id"`
	Runs         mlSimulatedRange         `json:"runs"`
	BallsFaced   mlSimulatedRange         `json:"balls_faced"`
	Wickets      mlSimulatedRange         `json:"wickets"`
	RunsConceded mlSimulatedRange         `json:"runs_conceded"`
	Scorecard    mlSimulatedScorecardLine `json:"scorecard"`
	SpreadShare  float64                  `json:"spread_share"`
	SpreadRuns   float64                  `json:"spread_runs"`
}

type mlSimulatedSide struct {
	Total           mlSimulatedTotal    `json:"total"`
	ExtrasScorecard float64             `json:"extras_scorecard"`
	Players         []mlSimulatedPlayer `json:"players"`
}

type mlSimulatedWinProbability struct {
	Simulated      float64 `json:"simulated"`
	Display        float64 `json:"display"`
	Headline       float64 `json:"headline"`
	HeadlineSource string  `json:"headline_source"`
}

type mlSimulateResponse struct {
	NSamples         int                       `json:"n_samples"`
	TossMarginalised bool                      `json:"toss_marginalised"`
	Team1            mlSimulatedSide           `json:"team1"`
	Team2            mlSimulatedSide           `json:"team2"`
	WinProbability   mlSimulatedWinProbability `json:"win_probability"`
}

// SimulateMatchXI calls POST /simulate: the match drawn from the performance model's
// forecasts for the two elevens (L2-C) -- totals, per-player ranges, the median-band
// scorecard and P(win), all from the same draws.
func (c *BacktestMLClient) SimulateMatchXI(
	ctx context.Context,
	req predictteam.XISimulationRequest,
) (*predictteam.XISimulationResult, error) {
	payload, err := json.Marshal(mlSimulateRequest{
		Format:         req.Format,
		Team1PlayerIDs: req.Team1PlayerIDs,
		Team2PlayerIDs: req.Team2PlayerIDs,
		Team1ID:        optionalID(req.Team1ID),
		Team2ID:        optionalID(req.Team2ID),
		VenueID:        optionalID(req.VenueID),
		Team1BatsFirst: req.Team1BatsFirst,
		AsOf:           asOfParam(req.AsOf),
		NSamples:       req.Samples,
		Seed:           req.Seed,
	})
	if err != nil {
		return nil, err
	}
	var out mlSimulateResponse
	if err := c.postJSON(ctx, "/simulate", payload, &out); err != nil {
		return nil, err
	}
	return &predictteam.XISimulationResult{
		Samples:                      out.NSamples,
		TossMarginalised:             out.TossMarginalised,
		Team1:                        simulatedSide(out.Team1),
		Team2:                        simulatedSide(out.Team2),
		SimulatedTeam1WinProbability: out.WinProbability.Simulated,
		DisplayTeam1WinProbability:   out.WinProbability.Display,
		HeadlineTeam1WinProbability:  out.WinProbability.Headline,
		HeadlineSource:               out.WinProbability.HeadlineSource,
	}, nil
}

func simulatedRange(r mlSimulatedRange) predictteam.XISimulatedRange {
	return predictteam.XISimulatedRange{P10: r.Q10, Median: r.Median, P90: r.Q90}
}

func simulatedSide(side mlSimulatedSide) predictteam.XISimulatedSide {
	players := make([]predictteam.XISimulatedPlayer, 0, len(side.Players))
	for _, p := range side.Players {
		players = append(players, predictteam.XISimulatedPlayer{
			PlayerID:              p.PlayerID,
			Runs:                  simulatedRange(p.Runs),
			BallsFaced:            simulatedRange(p.BallsFaced),
			Wickets:               simulatedRange(p.Wickets),
			RunsConceded:          simulatedRange(p.RunsConceded),
			ScorecardRuns:         p.Scorecard.Runs,
			ScorecardBalls:        p.Scorecard.BallsFaced,
			ScorecardWickets:      p.Scorecard.Wickets,
			ScorecardRunsConceded: p.Scorecard.RunsConceded,
			ScorecardBallsBowled:  p.Scorecard.BallsBowled,
			SpreadShare:           p.SpreadShare,
			SpreadRuns:            p.SpreadRuns,
		})
	}
	return predictteam.XISimulatedSide{
		Total:           simulatedRange(side.Total.mlSimulatedRange),
		TotalMean:       side.Total.Mean,
		TotalScorecard:  side.Total.Scorecard,
		ExtrasScorecard: side.ExtrasScorecard,
		Players:         players,
	}
}

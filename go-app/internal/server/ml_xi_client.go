package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/services/predictteam"
)

// Wire shapes for the ml-service /xi/* endpoints (app/models/xi.py).

// mlServedRatings is the stamp every prediction response carries (ServedRatings in
// app/models/xi.py): which run answered and the date its ratings run through.
type mlServedRatings struct {
	RunID          string `json:"run_id"`
	RatingsThrough string `json:"ratings_through"`
}

func (s mlServedRatings) served() predictteam.ServedRatings {
	return predictteam.ServedRatings{RunID: s.RunID, RatingsThrough: s.RatingsThrough}
}

type mlXIConstraints struct {
	TeamSize      int      `json:"team_size"`
	MinBowlers    int      `json:"min_bowlers"`
	RequireKeeper bool     `json:"require_keeper"`
	MustInclude   []string `json:"must_include"`
	MustExclude   []string `json:"must_exclude"`
}

type mlXIOptimizeRequest struct {
	Format            string          `json:"format"`
	Objective         string          `json:"objective"`
	PoolPlayerIDs     []string        `json:"pool_player_ids"`
	OpponentPlayerIDs []string        `json:"opponent_player_ids"`
	TeamIsTeam1       bool            `json:"team_is_team1"`
	Constraints       mlXIConstraints `json:"constraints"`
	MaxEvaluations    int             `json:"max_evaluations"`
	// AsOf (YYYY-MM-DD) asks for ratings as of that date; omitted = through today.
	AsOf string `json:"as_of,omitempty"`
}

type mlXIOptimizeResponse struct {
	SelectedPlayerIDs []string           `json:"selected_player_ids"`
	Objective         string             `json:"objective"`
	Optimised         bool               `json:"optimised"`
	UnknownPlayerIDs  []string           `json:"unknown_player_ids"`
	MarginalValues    map[string]float64 `json:"marginal_values"`
	ServedRatings     mlServedRatings    `json:"served_ratings"`
}

type mlXIWinRequest struct {
	Format         string   `json:"format"`
	Team1PlayerIDs []string `json:"team1_player_ids"`
	Team2PlayerIDs []string `json:"team2_player_ids"`
	Team1ID        *int64   `json:"team1_id,omitempty"`
	Team2ID        *int64   `json:"team2_id,omitempty"`
	VenueID        *int64   `json:"venue_id,omitempty"`
	// AsOf (YYYY-MM-DD) asks for ratings as of that date; omitted = through today.
	AsOf string `json:"as_of,omitempty"`
}

type mlXIWinResponse struct {
	Team1WinProbability  float64         `json:"team1_win_probability"`
	ObjectiveProbability float64         `json:"objective_probability"`
	ServedRatings        mlServedRatings `json:"served_ratings"`
}

// OptimizeXI calls POST /xi/optimize: server-side selection over player ids against the
// XI-responsive win model.
func (c *MLClient) OptimizeXI(
	ctx context.Context,
	req predictteam.XIOptimizationRequest,
) (*predictteam.XIOptimizationResult, error) {
	maxEvals := req.MaxEvaluations
	if maxEvals < 100 {
		maxEvals = 20000
	}
	opponent := req.OpponentPlayerKeys
	if opponent == nil {
		opponent = []string{}
	}
	payload, err := json.Marshal(mlXIOptimizeRequest{
		Format:            req.Format,
		Objective:         req.Objective,
		PoolPlayerIDs:     req.PoolPlayerKeys,
		OpponentPlayerIDs: opponent,
		TeamIsTeam1:       req.TeamIsTeam1,
		Constraints: mlXIConstraints{
			TeamSize:      req.Constraints.Size,
			MinBowlers:    req.Constraints.MinBowlers,
			RequireKeeper: req.Constraints.RequireKeeper,
			MustInclude:   []string{},
			MustExclude:   []string{},
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
	return &predictteam.XIOptimizationResult{
		SelectedPlayerKeys: out.SelectedPlayerIDs,
		Objective:          out.Objective,
		Optimised:          out.Optimised,
		UnknownPlayerKeys:  out.UnknownPlayerIDs,
		MarginalValues:     out.MarginalValues,
		Served:             out.ServedRatings.served(),
	}, nil
}

// PredictMatchWinXI calls POST /xi/predict-win and returns the displayed P(team1 wins)
// with the rating state it was read from.
func (c *MLClient) PredictMatchWinXI(
	ctx context.Context,
	req predictteam.XIWinRequest,
) (*predictteam.XIWinResult, error) {
	payload, err := json.Marshal(mlXIWinRequest{
		Format:         req.Format,
		Team1PlayerIDs: req.Team1PlayerKeys,
		Team2PlayerIDs: req.Team2PlayerKeys,
		Team1ID:        optionalID(req.Team1ID),
		Team2ID:        optionalID(req.Team2ID),
		VenueID:        optionalID(req.VenueID),
		AsOf:           asOfParam(req.AsOf),
	})
	if err != nil {
		return nil, err
	}
	var out mlXIWinResponse
	if err := c.postJSON(ctx, "/xi/predict-win", payload, &out); err != nil {
		return nil, err
	}
	return &predictteam.XIWinResult{
		Team1WinProbability: out.Team1WinProbability,
		Served:              out.ServedRatings.served(),
	}, nil
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
func (c *MLClient) postJSON(ctx context.Context, path string, payload []byte, out interface{}) error {
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
	Format         string   `json:"format"`
	Team1PlayerIDs []string `json:"team1_player_ids"`
	Team2PlayerIDs []string `json:"team2_player_ids"`
	Team1ID        *int64   `json:"team1_id,omitempty"`
	Team2ID        *int64   `json:"team2_id,omitempty"`
	VenueID        *int64   `json:"venue_id,omitempty"`
	Team1BatsFirst *bool    `json:"team1_bats_first,omitempty"`
	AsOf           string   `json:"as_of,omitempty"`
	NSamples       int      `json:"n_samples,omitempty"`
	Seed           int      `json:"seed"`
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
	PlayerID     string                   `json:"player_id"`
	Runs         mlSimulatedRange         `json:"runs"`
	BallsFaced   mlSimulatedRange         `json:"balls_faced"`
	Wickets      mlSimulatedRange         `json:"wickets"`
	RunsConceded mlSimulatedRange         `json:"runs_conceded"`
	Scorecard    mlSimulatedScorecardLine `json:"scorecard"`
	SpreadShare  float64                  `json:"spread_share"`
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
	ServedRatings    mlServedRatings           `json:"served_ratings"`
}

// SimulateMatchXI calls POST /simulate: the match drawn from the performance model's
// forecasts for the two elevens (L2-C) -- totals, per-player ranges, the median-band
// scorecard and P(win), all from the same draws.
func (c *MLClient) SimulateMatchXI(
	ctx context.Context,
	req predictteam.XISimulationRequest,
) (*predictteam.XISimulationResult, error) {
	payload, err := json.Marshal(mlSimulateRequest{
		Format:         req.Format,
		Team1PlayerIDs: req.Team1PlayerKeys,
		Team2PlayerIDs: req.Team2PlayerKeys,
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
		Served:                       out.ServedRatings.served(),
	}, nil
}

func simulatedRange(r mlSimulatedRange) predictteam.XISimulatedRange {
	return predictteam.XISimulatedRange{P10: r.Q10, Median: r.Median, P90: r.Q90}
}

func simulatedSide(side mlSimulatedSide) predictteam.XISimulatedSide {
	players := make([]predictteam.XISimulatedPlayer, 0, len(side.Players))
	for _, p := range side.Players {
		players = append(players, predictteam.XISimulatedPlayer{
			PlayerKey:             p.PlayerID,
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
		})
	}
	return predictteam.XISimulatedSide{
		Total:           simulatedRange(side.Total.mlSimulatedRange),
		TotalScorecard:  side.Total.Scorecard,
		ExtrasScorecard: side.ExtrasScorecard,
		Players:         players,
	}
}

// Wire shapes for POST /performance/predict (app/models/xi.py).

type mlPerformanceRequest struct {
	Format         string   `json:"format"`
	Team1PlayerIDs []string `json:"team1_player_ids"`
	Team2PlayerIDs []string `json:"team2_player_ids"`
	Team1ID        *int64   `json:"team1_id,omitempty"`
	Team2ID        *int64   `json:"team2_id,omitempty"`
	VenueID        *int64   `json:"venue_id,omitempty"`
	AsOf           string   `json:"as_of,omitempty"`
}

type mlWicketDistribution struct {
	Expected float64 `json:"expected"`
}

type mlPerformancePlayer struct {
	PlayerID     string               `json:"player_id"`
	Runs         mlSimulatedRange     `json:"runs"`
	BallsFaced   mlSimulatedRange     `json:"balls_faced"`
	RunsConceded mlSimulatedRange     `json:"runs_conceded"`
	Wickets      mlWicketDistribution `json:"wickets"`
}

type mlPerformanceResponse struct {
	Players             []mlPerformancePlayer `json:"players"`
	InningsMarginalised bool                  `json:"innings_marginalised"`
	ServedRatings       mlServedRatings       `json:"served_ratings"`
}

// PredictPerformance calls POST /performance/predict: L2-B's per-player distributions for
// two elevens. It is what a format with no innings length shows instead of a simulated
// scorecard -- the same forecasts, reported rather than drawn from.
func (c *MLClient) PredictPerformance(
	ctx context.Context,
	req predictteam.XIPerformanceRequest,
) (*predictteam.XIPerformanceResult, error) {
	payload, err := json.Marshal(mlPerformanceRequest{
		Format:         req.Format,
		Team1PlayerIDs: req.Team1PlayerKeys,
		Team2PlayerIDs: req.Team2PlayerKeys,
		Team1ID:        optionalID(req.Team1ID),
		Team2ID:        optionalID(req.Team2ID),
		VenueID:        optionalID(req.VenueID),
		AsOf:           asOfParam(req.AsOf),
	})
	if err != nil {
		return nil, err
	}
	var out mlPerformanceResponse
	if err := c.postJSON(ctx, "/performance/predict", payload, &out); err != nil {
		return nil, err
	}
	players := make([]predictteam.XIPerformancePlayer, 0, len(out.Players))
	for _, p := range out.Players {
		players = append(players, predictteam.XIPerformancePlayer{
			PlayerKey:    p.PlayerID,
			Runs:         simulatedRange(p.Runs),
			BallsFaced:   simulatedRange(p.BallsFaced),
			RunsConceded: simulatedRange(p.RunsConceded),
			Wickets:      p.Wickets.Expected,
		})
	}
	return &predictteam.XIPerformanceResult{
		InningsMarginalised: out.InningsMarginalised,
		Players:             players,
		Served:              out.ServedRatings.served(),
	}, nil
}

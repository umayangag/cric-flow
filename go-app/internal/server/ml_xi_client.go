package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
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
	SelectedPlayerIDs []string                           `json:"selected_player_ids"`
	Objective         string                             `json:"objective"`
	Optimised         bool                               `json:"optimised"`
	UnknownPlayerIDs  []string                           `json:"unknown_player_ids"`
	MarginalValues    map[string]float64                 `json:"marginal_values"`
	SelectionReasons  map[string]mlPlayerSelectionReason `json:"selection_reasons"`
	ServedRatings     mlServedRatings                    `json:"served_ratings"`
}

// mlBestAlternative and mlPlayerSelectionReason are the "why this player" state
// (app/models/xi.py BestAlternativeModel / PlayerSelectionReasonModel, P1-3). The
// alternative is a registry id here; go-app resolves it to a player id and name against
// the pool the selection was made from.
type mlBestAlternative struct {
	PlayerID          string  `json:"player_id"`
	WinProbabilityGap float64 `json:"win_probability_gap"`
}

type mlPlayerSelectionReason struct {
	Roles               []string           `json:"roles"`
	SelectionRating     float64            `json:"selection_rating"`
	RatingPercentile    float64            `json:"rating_percentile"`
	PoolSize            int                `json:"pool_size"`
	BestAlternative     *mlBestAlternative `json:"best_alternative"`
	BestAlternativeNote string             `json:"best_alternative_note"`
}

// selectionReasons maps ml-service's reasons onto the service types. An absent block stays
// absent: a player with no reason gets no card rather than a card of zeroes.
func selectionReasons(reasons map[string]mlPlayerSelectionReason) map[string]predictteam.XISelectionReason {
	if len(reasons) == 0 {
		return nil
	}
	out := make(map[string]predictteam.XISelectionReason, len(reasons))
	for key, reason := range reasons {
		mapped := predictteam.XISelectionReason{
			Roles:               reason.Roles,
			SelectionRating:     reason.SelectionRating,
			RatingPercentile:    reason.RatingPercentile,
			PoolSize:            reason.PoolSize,
			BestAlternativeNote: reason.BestAlternativeNote,
		}
		if reason.BestAlternative != nil {
			mapped.BestAlternativeKey = reason.BestAlternative.PlayerID
			mapped.BestAlternativeGap = reason.BestAlternative.WinProbabilityGap
		}
		out[key] = mapped
	}
	return out
}

type mlXIWinRequest struct {
	Format         string   `json:"format"`
	Team1PlayerIDs []string `json:"team1_player_ids"`
	Team2PlayerIDs []string `json:"team2_player_ids"`
	Team1ID        *int64   `json:"team1_id,omitempty"`
	Team2ID        *int64   `json:"team2_id,omitempty"`
	VenueID        *int64   `json:"venue_id,omitempty"`
	// Team1BatsFirst is the toss, nil before it. The display model reads the batting
	// order in every format, so a caller who names one is answered at that order rather
	// than over the average of both (GO-07); the response says which it answered.
	Team1BatsFirst *bool `json:"team1_bats_first,omitempty"`
	// Team1Constraints and Team2Constraints ask ml-service to check the eleven it is
	// scoring against these constraints instead of selecting under them (P1-2). Omitted
	// on the searched path, where the optimiser applied them while it searched.
	Team1Constraints *mlXIConstraints `json:"team1_constraints,omitempty"`
	Team2Constraints *mlXIConstraints `json:"team2_constraints,omitempty"`
	// AsOf (YYYY-MM-DD) asks for ratings as of that date; omitted = through today.
	AsOf string `json:"as_of,omitempty"`
}

// mlXIConstraintCheck is one eleven measured against its constraints (app/models/xi.py
// XiConstraintCheck).
type mlXIConstraintCheck struct {
	TeamSize           int      `json:"team_size"`
	Bowlers            int      `json:"bowlers"`
	MinBowlers         int      `json:"min_bowlers"`
	HasKeeper          bool     `json:"has_keeper"`
	RequireKeeper      bool     `json:"require_keeper"`
	MissingMustInclude []string `json:"missing_must_include"`
	Met                bool     `json:"met"`
}

func (c *mlXIConstraintCheck) check() *predictteam.XIConstraintCheck {
	if c == nil {
		return nil
	}
	return &predictteam.XIConstraintCheck{
		TeamSize:               c.TeamSize,
		Bowlers:                c.Bowlers,
		MinBowlers:             c.MinBowlers,
		HasKeeper:              c.HasKeeper,
		RequireKeeper:          c.RequireKeeper,
		MissingMustIncludeKeys: c.MissingMustInclude,
		Met:                    c.Met,
	}
}

// constraintPayload renders a constraint check request for the wire; nil stays absent, so
// a request that asks for no check sends no field.
func constraintPayload(req *predictteam.ConstraintCheckRequest) *mlXIConstraints {
	if req == nil {
		return nil
	}
	mustInclude := req.MustIncludeKeys
	if mustInclude == nil {
		mustInclude = []string{}
	}
	return &mlXIConstraints{
		TeamSize:      req.Size,
		MinBowlers:    req.MinBowlers,
		RequireKeeper: req.RequireKeeper,
		MustInclude:   mustInclude,
		MustExclude:   []string{},
	}
}

type mlXIWinResponse struct {
	Team1WinProbability float64 `json:"team1_win_probability"`
	// ObjectiveProbability is the optimiser's value and is marginalised over the batting
	// order whatever the request said — its model has no batting-order feature. It is
	// decoded and not served: nothing on the surface shows it.
	ObjectiveProbability float64              `json:"objective_probability"`
	TossMarginalised     bool                 `json:"toss_marginalised"`
	Team1ConstraintCheck *mlXIConstraintCheck `json:"team1_constraint_check"`
	Team2ConstraintCheck *mlXIConstraintCheck `json:"team2_constraint_check"`
	ServedRatings        mlServedRatings      `json:"served_ratings"`
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
	// The lock the search is held to (B-10). It used to be sent empty whatever the request
	// carried, so a "must include" was a pool entry and nothing more.
	mustInclude := req.MustIncludeKeys
	if mustInclude == nil {
		mustInclude = []string{}
	}
	// The opposing eleven, where there is one: no search may field a player the other
	// side is fielding (GO-04).
	mustExclude := req.MustExcludeKeys
	if mustExclude == nil {
		mustExclude = []string{}
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
			MustInclude:   mustInclude,
			MustExclude:   mustExclude,
		},
		MaxEvaluations: maxEvals,
		AsOf:           dateParam(req.AsOf),
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
		SelectionReasons:   selectionReasons(out.SelectionReasons),
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
		Format:           req.Format,
		Team1PlayerIDs:   req.Team1PlayerKeys,
		Team2PlayerIDs:   req.Team2PlayerKeys,
		Team1ID:          optionalID(req.Team1ID),
		Team2ID:          optionalID(req.Team2ID),
		VenueID:          optionalID(req.VenueID),
		Team1BatsFirst:   req.Team1BatsFirst,
		Team1Constraints: constraintPayload(req.Team1Constraints),
		Team2Constraints: constraintPayload(req.Team2Constraints),
		AsOf:             dateParam(req.AsOf),
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
		TossMarginalised:    out.TossMarginalised,
		Team1Check:          out.Team1ConstraintCheck.check(),
		Team2Check:          out.Team2ConstraintCheck.check(),
		Served:              out.ServedRatings.served(),
	}, nil
}

// dateParam renders a calendar date for the wire. The zero time is "not given" and is
// omitted: on `as_of` that means "ratings through today", and on `match_date` it means
// "date the fixture yourself", which ml-service reports having done (SERVE-04).
func dateParam(day time.Time) string {
	if day.IsZero() {
		return ""
	}
	return day.Format("2006-01-02")
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
		return unreachableError(path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return logMLNon2xx(resp, path)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// mlUnreachableCode is the code a prediction is refused with when ml-service did not
// answer at all — a refused connection, a timeout, a DNS failure.
const mlUnreachableCode = "ML_UNREACHABLE"

// unreachableError names a transport failure for what it is (P1-4).
//
// A bare transport error reached the surface as `500 INTERNAL` with a dial string for a
// message, which blames this service for a dependency that is down and gives the operator
// nothing to act on. It is a structured 502 instead: the dependency, the endpoint, the
// reason the transport gave, and the step that fixes it — on the wire, not only in a log.
func unreachableError(path string, err error) error {
	slog.Error("ml-service unreachable",
		slog.String("endpoint", path),
		slog.Any("err", err))
	return &mlServiceError{
		Endpoint: path,
		Status:   http.StatusBadGateway,
		Code:     mlUnreachableCode,
		Message:  fmt.Sprintf("ml-service did not answer %s: %v", path, err),
		Hint:     "start ml-service (make dev-up), or check ML_SERVICE_URL; no prediction is served without it",
	}
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
	// MatchDate (YYYY-MM-DD) is the day the fixture is played: what ml-service reads every
	// date-dependent feature at (SERVE-04). Omitting it lets ml-service date the fixture
	// itself, which it reports as such; this client always knows the date and sends it.
	MatchDate string `json:"match_date,omitempty"`
	// Gender picks the context baseline the fixture's scoring rates are read from; empty
	// is the unsplit baseline and is omitted.
	Gender   string `json:"gender,omitempty"`
	NSamples int    `json:"n_samples,omitempty"`
	Seed     int    `json:"seed"`
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
	SharedFactor     bool                      `json:"shared_factor"`
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
		AsOf:           dateParam(req.AsOf),
		MatchDate:      dateParam(req.MatchDate),
		Gender:         req.Gender,
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
		SharedFactor:                 out.SharedFactor,
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
	// Team1BatsFirst is nil before the toss, when both batting orders are averaged.
	// `bats_first` is a per-player feature of the row this model predicts, so it orients
	// the forecast in every format — including the ones with no innings length, which the
	// Lab's quantiles path used to leave nil on the theory that they had no toss (GO-07).
	Team1BatsFirst *bool  `json:"team1_bats_first,omitempty"`
	AsOf           string `json:"as_of,omitempty"`
	// MatchDate (YYYY-MM-DD) is the day the fixture is played (SERVE-04), as on the
	// simulate payload: the rows behind both answers are built from the same fixture and
	// must be dated identically.
	MatchDate string `json:"match_date,omitempty"`
	Gender    string `json:"gender,omitempty"`
}

// mlWicketDistribution is the wicket count's distribution (app/models/xi.py
// WicketDistribution). It is a count distribution and not a quantile head: there are no
// q10/q90 on this path, and deriving an interval from the three probabilities would be an
// interval the model never produced.
type mlWicketDistribution struct {
	Expected float64 `json:"expected"`
	P0       float64 `json:"p0"`
	P1       float64 `json:"p1"`
	P2Plus   float64 `json:"p2_plus"`
}

type mlPerformancePlayer struct {
	PlayerID string `json:"player_id"`
	// Side is 1 for team1 and 2 for team2. Both elevens come back in one flat list, so it
	// is the other half of a row's identity and was, until GO-04, read by nobody.
	Side         int                  `json:"side"`
	Runs         mlSimulatedRange     `json:"runs"`
	BallsFaced   mlSimulatedRange     `json:"balls_faced"`
	RunsConceded mlSimulatedRange     `json:"runs_conceded"`
	Wickets      mlWicketDistribution `json:"wickets"`
}

// mlVenueContext is what the served state knew about the ground these rows were built with
// (app/models/xi.py VenueContext, P3-2): read off the rows the model consumed, so a caller
// comparing two grounds can say which of them the model had anything to go on for.
type mlVenueContext struct {
	VenueBFRate float64 `json:"venue_bf_rate"`
	VenueN      float64 `json:"venue_n"`
	Neutral     bool    `json:"neutral"`
}

type mlPerformanceResponse struct {
	Players             []mlPerformancePlayer `json:"players"`
	InningsMarginalised bool                  `json:"innings_marginalised"`
	VenueContext        mlVenueContext        `json:"venue_context"`
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
		Team1BatsFirst: req.Team1BatsFirst,
		AsOf:           dateParam(req.AsOf),
		MatchDate:      dateParam(req.MatchDate),
		Gender:         req.Gender,
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
			PlayerKey:     p.PlayerID,
			Side:          p.Side,
			Runs:          simulatedRange(p.Runs),
			BallsFaced:    simulatedRange(p.BallsFaced),
			RunsConceded:  simulatedRange(p.RunsConceded),
			Wickets:       p.Wickets.Expected,
			WicketsP0:     p.Wickets.P0,
			WicketsP1:     p.Wickets.P1,
			WicketsP2Plus: p.Wickets.P2Plus,
		})
	}
	return &predictteam.XIPerformanceResult{
		InningsMarginalised: out.InningsMarginalised,
		Players:             players,
		VenueBatFirstRate:   out.VenueContext.VenueBFRate,
		VenueMatches:        out.VenueContext.VenueN,
		VenueNeutral:        out.VenueContext.Neutral,
		Served:              out.ServedRatings.served(),
	}, nil
}

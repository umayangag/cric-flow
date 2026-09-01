package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/config"
	"github.com/umayangag/cric-flow/go-app/internal/services/predictteam"
)

// mlPredictorAdapter adapts the backtest ML client to predictteam.MLPredictor.
type mlPredictorAdapter struct{}

func (mlPredictorAdapter) PredictPlayers(
	ctx context.Context,
	cutoff time.Time,
	format string,
	playerIDs []int64,
	features map[int64]map[string]float64,
	matchCtx *predictteam.MatchContext,
) (map[int64]predictteam.PlayerPred, error) {
	var mc *MatchContextForReconciliation
	if matchCtx != nil {
		mc = &MatchContextForReconciliation{
			Team1PlayerIDs:    matchCtx.Team1PlayerIDs,
			Team2PlayerIDs:    matchCtx.Team2PlayerIDs,
			VenueID:           matchCtx.VenueID,
			FormatID:          matchCtx.FormatID,
			Team1OppositionID: matchCtx.Team1OppositionID,
			Team2OppositionID: matchCtx.Team2OppositionID,
			Temp:              matchCtx.Temp,
			Wind:              matchCtx.Wind,
			Rain:              matchCtx.Rain,
			Humidity:          matchCtx.Humidity,
			Cloud:             matchCtx.Cloud,
			Pressure:          matchCtx.Pressure,
			Viscosity:         matchCtx.Viscosity,
		}
	}
	preds, err := mlBacktestPredictFunc(ctx, cutoff, format, playerIDs, features, mc)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]predictteam.PlayerPred, len(preds))
	for pid, p := range preds {
		out[pid] = predictteam.PlayerPred{
			Runs:    p.Runs,
			Wickets: p.Wickets,
			Economy: p.Economy,
			Catches: p.Catches,
			RunOuts: p.RunOuts,
		}
	}
	return out, nil
}

func (mlPredictorAdapter) PredictMatchWin(ctx context.Context, w predictteam.WinFeatures) (float64, error) {
	return mlPredictMatchWinFunc(ctx, w)
}

func (mlPredictorAdapter) PredictMatchWinEnhanced(
	ctx context.Context,
	w predictteam.WinFeaturesEnhanced,
) (float64, error) {
	return mlPredictMatchWinEnhancedFunc(ctx, w)
}

func (mlPredictorAdapter) OptimizeTeamSelection(
	ctx context.Context,
	req predictteam.TeamOptimizationRequest,
) (*predictteam.TeamOptimizationResult, error) {
	return mlOptimizeTeamSelectionFunc(ctx, req)
}

func (mlPredictorAdapter) OptimizeXI(
	ctx context.Context,
	req predictteam.XIOptimizationRequest,
) (*predictteam.XIOptimizationResult, error) {
	return mlOptimizeXIFunc(ctx, req)
}

func (mlPredictorAdapter) PredictMatchWinXI(ctx context.Context, req predictteam.XIWinRequest) (float64, error) {
	return mlPredictMatchWinXIFunc(ctx, req)
}

func (mlPredictorAdapter) SimulateMatchXI(
	ctx context.Context,
	req predictteam.XISimulationRequest,
) (*predictteam.XISimulationResult, error) {
	return mlSimulateMatchXIFunc(ctx, req)
}

// predictTeamRequest holds the parsed request body for team-selection prediction.
type predictTeamRequest struct {
	Format             string `json:"format"`
	Team1              string `json:"team1"`
	Team2              string `json:"team2"`
	Venue              string `json:"venue"`
	MatchDate          string `json:"match_date"`
	Simulate           *bool  `json:"simulate,omitempty"`
	SimulationTopK     int    `json:"simulation_top_k,omitempty"`
	SimulationSamples  int    `json:"simulation_samples,omitempty"`
	SimulationMaxPairs int    `json:"simulation_max_pairs,omitempty"`
	// Weather is decoded only to refuse it. It is retired (consumer plan W0-3), and an
	// unknown field is silently dropped by encoding/json — so a caller still sending a
	// forecast would get a prediction computed without it and no indication why.
	Weather                json.RawMessage `json:"weather,omitempty"`
	ExtraTeam1             []int64         `json:"extra_team1"`
	ExtraTeam2             []int64         `json:"extra_team2"`
	MinBowlers             int             `json:"min_bowlers"`
	RequireKeeper          *bool           `json:"require_keeper"`
	UseReconciledScorecard *bool           `json:"use_reconciled_scorecard,omitempty"`
	IncludeBothScorecards  *bool           `json:"include_both_scorecards,omitempty"`
}

// parsePredictTeamRequest decodes the request body from JSON or query params.
func parsePredictTeamRequest(r *http.Request) (predictTeamRequest, error) {
	var body predictTeamRequest
	if r.Method == http.MethodPost && r.Header.Get("Content-Type") == "application/json" {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return body, err
		}
		if err := rejectRetiredBodyField(len(body.Weather) > 0, "weather"); err != nil {
			return body, err
		}
		return body, nil
	}
	q := r.URL.Query()
	body.Format = strings.TrimSpace(q.Get("format"))
	body.Team1 = strings.TrimSpace(q.Get("team1"))
	body.Team2 = strings.TrimSpace(q.Get("team2"))
	body.Venue = strings.TrimSpace(q.Get("venue"))
	body.MatchDate = strings.TrimSpace(q.Get("match_date"))
	if s := q.Get("min_bowlers"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			body.MinBowlers = n
		}
	}
	if s := q.Get("require_keeper"); s != "" {
		v := strings.EqualFold(s, "true") || s == "1"
		body.RequireKeeper = &v
	}
	if s := q.Get("simulate"); s == "1" || strings.EqualFold(s, "true") {
		t := true
		body.Simulate = &t
	}
	if s := q.Get("simulation_top_k"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			body.SimulationTopK = n
		}
	}
	if s := q.Get("simulation_samples"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			body.SimulationSamples = n
		}
	}
	if s := q.Get("simulation_max_pairs"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 0 {
			body.SimulationMaxPairs = n
		}
	}
	if s := q.Get("use_reconciled_scorecard"); s == "1" || strings.EqualFold(s, "true") {
		t := true
		body.UseReconciledScorecard = &t
	}
	if s := q.Get("include_both_scorecards"); s == "1" || strings.EqualFold(s, "true") {
		t := true
		body.IncludeBothScorecards = &t
	}
	return body, nil
}

// buildPredictInput converts a parsed request into a predictteam.Input.
func buildPredictInput(body predictTeamRequest, matchDate time.Time) predictteam.Input {
	input := predictteam.Input{
		Format:                 body.Format,
		Team1:                  body.Team1,
		Team2:                  body.Team2,
		Venue:                  body.Venue,
		MatchDate:              matchDate,
		ExtraTeam1:             body.ExtraTeam1,
		ExtraTeam2:             body.ExtraTeam2,
		MinBowlers:             body.MinBowlers,
		RequireKeeper:          true,
		UseReconciledScorecard: body.UseReconciledScorecard != nil && *body.UseReconciledScorecard,
		IncludeBothScorecards:  body.IncludeBothScorecards != nil && *body.IncludeBothScorecards,
	}
	if body.RequireKeeper != nil {
		input.RequireKeeper = *body.RequireKeeper
	}
	if input.MinBowlers <= 0 {
		if cfg := config.Load(); cfg != nil && cfg.Team.MinBowlers > 0 {
			input.MinBowlers = cfg.Team.MinBowlers
		} else {
			input.MinBowlers = config.DefaultMinBowlers
		}
	}
	return input
}

// buildSimulationOpts builds simulation options from the request body and config.
func buildSimulationOpts(body predictTeamRequest) (predictteam.SimulationOpts, error) {
	cfg := config.Load()
	opts := predictteam.DefaultSimulationOpts()
	if body.SimulationTopK > 0 {
		opts.TopKPerTeam = body.SimulationTopK
	} else {
		opts.TopKPerTeam = config.EffectiveSimulationTopKPerTeam(cfg)
	}
	if body.SimulationSamples > 0 {
		opts.NumSamplesPerMatchup = body.SimulationSamples
	} else {
		opts.NumSamplesPerMatchup = config.EffectiveSimulationNumSamplesPerMatchup(cfg)
	}
	if body.SimulationMaxPairs > 0 {
		opts.MaxMatchups = body.SimulationMaxPairs
	}
	runsCV, wicketsCV, economyCV := config.EffectiveSimulationCVs(cfg)
	opts.RunsCV, opts.WicketsCV, opts.EconomyCV = runsCV, wicketsCV, economyCV
	maxTotalSamples := config.EffectiveMaxTotalSamples(cfg)
	matchups := opts.TopKPerTeam * opts.TopKPerTeam
	if opts.MaxMatchups > 0 && opts.MaxMatchups < matchups {
		matchups = opts.MaxMatchups
	}
	if matchups*opts.NumSamplesPerMatchup > maxTotalSamples {
		return opts, errors.New(
			"simulation would exceed max samples (reduce simulation_top_k, simulation_samples, or simulation_max_pairs)",
		)
	}
	return opts, nil
}

func newReconciledGenerator(client *BacktestMLClient) predictteam.GenerateMatchFunc {
	return func(
		ctx context.Context,
		cutoff time.Time,
		format string,
		playerIDs []int64,
		features map[int64]map[string]float64,
		matchCtx *predictteam.MatchContext,
	) (map[int64]predictteam.PlayerPred, float64, float64, float64, string, error) {
		var mc *MatchContextForReconciliation
		if matchCtx != nil {
			mc = &MatchContextForReconciliation{
				Team1PlayerIDs:    matchCtx.Team1PlayerIDs,
				Team2PlayerIDs:    matchCtx.Team2PlayerIDs,
				VenueID:           matchCtx.VenueID,
				FormatID:          matchCtx.FormatID,
				Team1OppositionID: matchCtx.Team1OppositionID,
				Team2OppositionID: matchCtx.Team2OppositionID,
				Temp:              matchCtx.Temp,
				Wind:              matchCtx.Wind,
				Rain:              matchCtx.Rain,
				Humidity:          matchCtx.Humidity,
				Cloud:             matchCtx.Cloud,
				Pressure:          matchCtx.Pressure,
				Viscosity:         matchCtx.Viscosity,
			}
		}
		resp, err := client.GenerateMatch(ctx, cutoff, format, playerIDs, features, mc)
		if err != nil {
			return nil, 0, 0, 0, "", err
		}
		players := make(map[int64]predictteam.PlayerPred, len(resp.Players))
		for _, p := range resp.Players {
			var balls, fours, sixes float64
			if p.Balls != nil {
				balls = *p.Balls
			}
			if p.Fours != nil {
				fours = *p.Fours
			}
			if p.Sixes != nil {
				sixes = *p.Sixes
			}
			players[p.PlayerID] = predictteam.PlayerPred{
				Runs:    p.Runs,
				Balls:   balls,
				Fours:   fours,
				Sixes:   sixes,
				Wickets: p.Wickets,
				Economy: p.Economy,
				Catches: p.Catches,
				RunOuts: p.RunOuts,
			}
		}
		in1, in2 := 0.0, 0.0
		if len(resp.Innings) >= 2 {
			in1, in2 = resp.Innings[0].Runs, resp.Innings[1].Runs
		}
		return players, in1, in2, resp.WinProbabilityTeam1, resp.ModelVersion, nil
	}
}

// predictTeamSelectionHandler handles POST /api/predict/team-selection
func (a *App) predictTeamSelectionHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, apiError{Code: "METHOD_NOT_ALLOWED", Message: "POST or GET required"})
		return
	}
	body, err := parsePredictTeamRequest(r)
	if err != nil {
		var retired retiredBodyFieldError
		if errors.As(err, &retired) {
			writeJSON(w, http.StatusBadRequest, apiError{
				Code:    retired.param.Code,
				Message: retired.param.Message,
				Hint:    retired.param.Hint,
			})
			return
		}
		writeJSON(w, http.StatusBadRequest, apiError{Code: "INVALID_JSON", Message: err.Error()})
		return
	}
	if body.Format == "" || body.Team1 == "" || body.Team2 == "" {
		writeJSON(
			w,
			http.StatusBadRequest,
			apiError{Code: "INVALID_PARAM", Message: "format, team1, team2 are required"},
		)
		return
	}
	if body.MatchDate == "" {
		writeJSON(
			w,
			http.StatusBadRequest,
			apiError{Code: "INVALID_PARAM", Message: "match_date is required (RFC3339 or YYYY-MM-DD)"},
		)
		return
	}
	matchDate, err := parseMatchDate(body.MatchDate)
	if err != nil {
		writeJSON(
			w,
			http.StatusBadRequest,
			apiError{Code: "INVALID_PARAM", Message: "match_date must be RFC3339 or YYYY-MM-DD: " + err.Error()},
		)
		return
	}
	input := buildPredictInput(body, matchDate)
	// Optional reconciled scorecard generator: when client requests reconciled or both scorecards, call ML generate-match.
	var reconciledGen predictteam.GenerateMatchFunc
	if input.UseReconciledScorecard || input.IncludeBothScorecards {
		client := a.backtestMLClient
		if client == nil {
			client = NewBacktestMLClient()
		}
		reconciledGen = newReconciledGenerator(client)
	}
	if body.Simulate != nil && *body.Simulate {
		opts, err := buildSimulationOpts(body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, apiError{Code: "INVALID_PARAM", Message: err.Error()})
			return
		}
		result, sim, err := predictteam.PredictTeamsWithSimulation(
			r.Context(),
			input,
			mlPredictorAdapter{},
			opts,
			reconciledGen,
		)
		if err != nil {
			respondErr(w, err)
			return
		}
		out := map[string]any{
			"team1": result.Team1, "team2": result.Team2,
			"scorecard_summary": result.ScorecardSummary, "simulation": sim,
		}
		if result.ScorecardSummaryReconciled != nil {
			out["scorecard_summary_reconciled"] = result.ScorecardSummaryReconciled
		}
		writeJSON(w, http.StatusOK, out)
		return
	}
	result, err := predictteam.PredictTeams(r.Context(), input, mlPredictorAdapter{}, reconciledGen)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func parseMatchDate(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}
	return time.Time{}, errors.New("invalid date format")
}

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
) (map[int64]predictteam.PlayerPred, error) {
	preds, err := mlBacktestPredictFunc(ctx, cutoff, format, playerIDs, features)
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

// predictTeamSelectionHandler handles POST /api/predict/team-selection
func (a *App) predictTeamSelectionHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, apiError{Code: "METHOD_NOT_ALLOWED", Message: "POST or GET required"})
		return
	}
	var body struct {
		Format             string `json:"format"`
		Team1              string `json:"team1"`
		Team2              string `json:"team2"`
		Venue              string `json:"venue"`
		MatchDate          string `json:"match_date"` // RFC3339 or YYYY-MM-DD
		SeasonID           *int64 `json:"season_id"`
		UseUnifiedModel    *bool  `json:"use_unified_model,omitempty"`
		Simulate           *bool  `json:"simulate,omitempty"`             // run Monte Carlo for win prob and outcome distributions
		SimulationTopK     int    `json:"simulation_top_k,omitempty"`     // top XIs per team (default 50)
		SimulationSamples  int    `json:"simulation_samples,omitempty"`   // samples per matchup (default 500)
		SimulationMaxPairs int    `json:"simulation_max_pairs,omitempty"` // cap on matchup pairs (0 = no cap)
		Weather            *struct {
			Temp     float64 `json:"temp"`
			Humidity float64 `json:"humidity"`
			Wind     float64 `json:"wind"`
			Rain     float64 `json:"rain"`
			Cloud    float64 `json:"cloud"`
			Pressure float64 `json:"pressure"`
		} `json:"weather"`
		ExtraTeam1    []int64 `json:"extra_team1"`
		ExtraTeam2    []int64 `json:"extra_team2"`
		MinBowlers    int     `json:"min_bowlers"`
		RequireKeeper *bool   `json:"require_keeper"`
	}
	if r.Method == http.MethodPost && r.Header.Get("Content-Type") == "application/json" {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, apiError{Code: "INVALID_JSON", Message: err.Error()})
			return
		}
	} else {
		q := r.URL.Query()
		body.Format = strings.TrimSpace(q.Get("format"))
		body.Team1 = strings.TrimSpace(q.Get("team1"))
		body.Team2 = strings.TrimSpace(q.Get("team2"))
		body.Venue = strings.TrimSpace(q.Get("venue"))
		body.MatchDate = strings.TrimSpace(q.Get("match_date"))
		if s := q.Get("season_id"); s != "" {
			if id, err := strconv.ParseInt(s, 10, 64); err == nil {
				body.SeasonID = &id
			}
		}
		if s := q.Get("min_bowlers"); s != "" {
			if n, err := strconv.Atoi(s); err == nil && n > 0 {
				body.MinBowlers = n
			}
		}
		if s := q.Get("require_keeper"); s != "" {
			v := strings.EqualFold(s, "true") || s == "1"
			body.RequireKeeper = &v
		}
		if s := q.Get("use_unified_model"); s == "1" || strings.EqualFold(s, "true") {
			t := true
			body.UseUnifiedModel = &t
		}
		if strings.EqualFold(strings.TrimSpace(q.Get("model")), "unified") {
			t := true
			body.UseUnifiedModel = &t
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
	}

	if body.Format == "" || body.Team1 == "" || body.Team2 == "" {
		writeJSON(w, http.StatusBadRequest, apiError{
			Code:    "INVALID_PARAM",
			Message: "format, team1, team2 are required",
		})
		return
	}
	if body.MatchDate == "" {
		writeJSON(w, http.StatusBadRequest, apiError{
			Code:    "INVALID_PARAM",
			Message: "match_date is required (RFC3339 or YYYY-MM-DD)",
		})
		return
	}
	matchDate, err := parseMatchDate(body.MatchDate)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, apiError{
			Code:    "INVALID_PARAM",
			Message: "match_date must be RFC3339 or YYYY-MM-DD: " + err.Error(),
		})
		return
	}

	input := predictteam.Input{
		Format:          body.Format,
		Team1:           body.Team1,
		Team2:           body.Team2,
		Venue:           body.Venue,
		MatchDate:       matchDate,
		SeasonID:        body.SeasonID,
		ExtraTeam1:      body.ExtraTeam1,
		ExtraTeam2:      body.ExtraTeam2,
		MinBowlers:      body.MinBowlers,
		RequireKeeper:   true,
		UseUnifiedModel: body.UseUnifiedModel != nil && *body.UseUnifiedModel,
	}
	if body.Weather != nil {
		input.Weather = &predictteam.WeatherInput{
			Temp:     body.Weather.Temp,
			Humidity: body.Weather.Humidity,
			Wind:     body.Weather.Wind,
			Rain:     body.Weather.Rain,
			Cloud:    body.Weather.Cloud,
			Pressure: body.Weather.Pressure,
		}
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

	simulate := body.Simulate != nil && *body.Simulate
	if simulate {
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
			writeJSON(
				w,
				http.StatusBadRequest,
				apiError{
					Code:    "INVALID_PARAM",
					Message: "simulation would exceed max samples (reduce simulation_top_k, simulation_samples, or simulation_max_pairs)",
				},
			)
			return
		}
		result, sim, err := predictteam.PredictTeamsWithSimulation(r.Context(), input, mlPredictorAdapter{}, opts)
		if err != nil {
			respondErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"team1":             result.Team1,
			"team2":             result.Team2,
			"scorecard_summary": result.ScorecardSummary,
			"simulation":        sim,
		})
		return
	}
	result, err := predictteam.PredictTeams(r.Context(), input, mlPredictorAdapter{})
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

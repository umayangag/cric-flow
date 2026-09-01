package predictteam

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// XISimulator is the ml-service /simulate contract (L2-C): the match drawn from the
// performance model's forecasts for two elevens, so the scorecard, the innings totals and
// their ranges come from one set of draws instead of being rescaled toward each other.
type XISimulator interface {
	SimulateMatchXI(ctx context.Context, req XISimulationRequest) (*XISimulationResult, error)
}

// XISimulationRequest is the Go-side payload for POST /simulate.
type XISimulationRequest struct {
	Format         string
	Team1PlayerIDs []int64
	Team2PlayerIDs []int64
	Team1ID        int64
	Team2ID        int64
	VenueID        int64
	// Team1BatsFirst is nil before the toss, when both batting orders are drawn.
	Team1BatsFirst *bool
	// AsOf asks for ratings as they stood strictly before this date (backtests); zero
	// means the serving state through today.
	AsOf time.Time
	// Samples is the draw count; zero lets the ML service use its default (2000).
	Samples int
	Seed    int
}

// XISimulatedRange is a 10-50-90 summary of one simulated quantity.
type XISimulatedRange struct {
	P10    float64 `json:"p10"`
	Median float64 `json:"median"`
	P90    float64 `json:"p90"`
}

// XISimulatedPlayer is one player's draws summarised: ranges, the median-band scorecard
// line (what the scorecard shows; the lines sum to the side total by construction) and
// the player's contribution to the side total's spread.
type XISimulatedPlayer struct {
	PlayerID              int64
	Runs                  XISimulatedRange
	BallsFaced            XISimulatedRange
	Wickets               XISimulatedRange
	RunsConceded          XISimulatedRange
	ScorecardRuns         float64
	ScorecardBalls        float64
	ScorecardWickets      float64
	ScorecardRunsConceded float64
	ScorecardBallsBowled  float64
	SpreadShare           float64
	SpreadRuns            float64
}

// XISimulatedSide is one side's simulated innings.
type XISimulatedSide struct {
	Total           XISimulatedRange
	TotalMean       float64
	TotalScorecard  float64 // the median-band total the scorecard lines and extras sum to
	ExtrasScorecard float64
	Players         []XISimulatedPlayer
}

// XISimulationResult is the Go-side response from POST /simulate.
type XISimulationResult struct {
	Samples                      int
	TossMarginalised             bool
	Team1                        XISimulatedSide
	Team2                        XISimulatedSide
	SimulatedTeam1WinProbability float64
	DisplayTeam1WinProbability   float64
	// HeadlineTeam1WinProbability is the one to show, per the plan's E2 rule; HeadlineSource
	// says which model it came from ("display" or "simulator").
	HeadlineTeam1WinProbability float64
	HeadlineSource              string
}

// XISimulationSummary is what the xi path adds to the team-selection response: the
// innings totals with their 10-90 ranges, the simulated win probability beside the
// displayed one, and which model the displayed probability came from.
type XISimulationSummary struct {
	Samples                      int              `json:"samples"`
	TossMarginalised             bool             `json:"toss_marginalised"`
	Innings1                     XISimulatedRange `json:"innings1"`
	Innings2                     XISimulatedRange `json:"innings2"`
	SimulatedTeam1WinProbability float64          `json:"simulated_team1_win_probability"`
	WinProbabilitySource         string           `json:"win_probability_source"`
}

// SelectionExplanation is the L3 "why this XI and why this total": each selected player's
// marginal value (P(win) lost if replaced by an average player, from /xi/optimize) and each
// player's share of the side total's spread (from the simulator).
type SelectionExplanation struct {
	MarginalValues      map[int64]float64 `json:"marginal_values,omitempty"`
	SpreadContributions map[int64]float64 `json:"spread_contributions,omitempty"`
}

// ValueRange is a 10-90 range shown beside a scorecard point.
type ValueRange struct {
	P10 float64 `json:"p10"`
	P90 float64 `json:"p90"`
}

// simulatedFormats are the formats with an innings length; the simulator runs for these
// only (TEST stays on the greedy path, plan H-17). Mirrors ml.xi.simulator.SIMULATED_FORMATS.
var simulatedFormats = map[string]bool{"T20": true, "T20I": true, "ODI": true}

// formatHasInningsLength reports whether the simulator runs for the format.
func formatHasInningsLength(format string) bool {
	return simulatedFormats[strings.TrimSpace(strings.ToUpper(format))]
}

// xiScorecardInputs is what the simulator needs to describe the selected fixture.
type xiScorecardInputs struct {
	format               string
	team1IDs, team2IDs   []int64
	team1ID, team2ID     int64
	venueID              int64
	asOf                 time.Time
	team1Code, team2Code string
	marginalValues       map[int64]float64
}

// applyXISimulation replaces the scorecard totals and per-player points with the
// simulator's, on the xi path only. It returns false -- and leaves result and summary
// untouched -- when the predictor cannot simulate, the format has no innings length, or
// the call failed, so the caller keeps the windowed-form scorecard.
func applyXISimulation(
	ctx context.Context,
	predictor MLPredictor,
	in xiScorecardInputs,
	result *Result,
	summary *ScorecardSummary,
) bool {
	simulator, ok := predictor.(XISimulator)
	if !ok || !selectionUsesXIWinModel() || !formatHasInningsLength(in.format) {
		return false
	}
	sim, err := simulator.SimulateMatchXI(ctx, XISimulationRequest{
		Format:         strings.TrimSpace(strings.ToUpper(in.format)),
		Team1PlayerIDs: in.team1IDs,
		Team2PlayerIDs: in.team2IDs,
		Team1ID:        in.team1ID,
		Team2ID:        in.team2ID,
		VenueID:        in.venueID,
		AsOf:           in.asOf,
	})
	if err != nil {
		slog.WarnContext(ctx, "xi simulation failed, keeping the windowed-form scorecard", slog.Any("err", err))
		return false
	}
	if len(sim.Team1.Players) == 0 || len(sim.Team2.Players) == 0 {
		slog.WarnContext(ctx, "xi simulation returned no players, keeping the windowed-form scorecard")
		return false
	}
	spread := make(map[int64]float64, len(sim.Team1.Players)+len(sim.Team2.Players))
	applySimulatedSide(result.Team1, sim.Team1, spread)
	applySimulatedSide(result.Team2, sim.Team2, spread)
	summary.Innings1Total = sim.Team1.TotalScorecard
	summary.Innings2Total = sim.Team2.TotalScorecard
	summary.ExtrasInnings1 = sim.Team1.ExtrasScorecard
	summary.ExtrasInnings2 = sim.Team2.ExtrasScorecard
	summary.Team1WinProbability = sim.HeadlineTeam1WinProbability
	if sim.HeadlineTeam1WinProbability >= 0.5 {
		summary.PredictedWinner = in.team1Code
	} else {
		summary.PredictedWinner = in.team2Code
	}
	result.XISimulation = &XISimulationSummary{
		Samples:                      sim.Samples,
		TossMarginalised:             sim.TossMarginalised,
		Innings1:                     sim.Team1.Total,
		Innings2:                     sim.Team2.Total,
		SimulatedTeam1WinProbability: sim.SimulatedTeam1WinProbability,
		WinProbabilitySource:         sim.HeadlineSource,
	}
	result.Explanation = &SelectionExplanation{MarginalValues: in.marginalValues, SpreadContributions: spread}
	slog.InfoContext(ctx, "xi simulation applied to the scorecard",
		slog.String("format", in.format),
		slog.Int("samples", sim.Samples),
		slog.Float64("innings1", sim.Team1.TotalScorecard),
		slog.Float64("innings2", sim.Team2.TotalScorecard),
		slog.Float64("p_display", sim.DisplayTeam1WinProbability),
		slog.Float64("p_simulated", sim.SimulatedTeam1WinProbability),
		slog.String("headline", sim.HeadlineSource))
	return true
}

// applySimulatedSide overwrites the selected players' points with their median-band
// scorecard lines and attaches the 10-90 ranges; players the simulator did not return
// (it always returns the eleven it was sent) keep their previous values and are logged.
func applySimulatedSide(players []SelectedPlayer, side XISimulatedSide, spread map[int64]float64) {
	byID := make(map[int64]XISimulatedPlayer, len(side.Players))
	for _, p := range side.Players {
		byID[p.PlayerID] = p
		spread[p.PlayerID] = p.SpreadShare
	}
	for i := range players {
		sp, ok := byID[players[i].PlayerID]
		if !ok {
			slog.Warn("xi simulation: selected player missing from the simulator's side",
				slog.Int64("player_id", players[i].PlayerID))
			continue
		}
		players[i].Runs = sp.ScorecardRuns
		players[i].Balls = sp.ScorecardBalls
		players[i].Wickets = sp.ScorecardWickets
		players[i].Economy = economy(sp.ScorecardRunsConceded, sp.ScorecardBallsBowled)
		players[i].RunsRange = &ValueRange{P10: sp.Runs.P10, P90: sp.Runs.P90}
		players[i].WicketsRange = &ValueRange{P10: sp.Wickets.P10, P90: sp.Wickets.P90}
	}
}

// economy is runs conceded per over; zero when the player did not bowl.
func economy(runsConceded, ballsBowled float64) float64 {
	if ballsBowled <= 0 {
		return 0
	}
	return runsConceded / (ballsBowled / 6.0)
}

// String renders the inputs for logs without the id lists.
func (in xiScorecardInputs) String() string {
	return fmt.Sprintf(
		"%s %s v %s (venue %d, as of %s)",
		in.format,
		in.team1Code,
		in.team2Code,
		in.venueID,
		in.asOf.Format("2006-01-02"),
	)
}

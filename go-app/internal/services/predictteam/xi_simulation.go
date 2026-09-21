package predictteam

import (
	"context"
	"fmt"
	"log/slog"
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
	Format          string
	Team1PlayerKeys []string
	Team2PlayerKeys []string
	Team1ID         int64
	Team2ID         int64
	VenueID         int64
	// Team1BatsFirst is nil before the toss, when both batting orders are drawn.
	Team1BatsFirst *bool
	// AsOf asks for ratings as they stood strictly before this date (backtests); zero
	// means the serving state through today.
	AsOf time.Time
	// MatchDate is the day the fixture is played, which every date-dependent feature in
	// the rows the draws come from is read at (SERVE-04).
	MatchDate time.Time
	// Samples is the draw count; zero lets the ML service use its default (2000).
	Samples int
	Seed    int
}

// XISimulatedRange is a 10-50-90 summary of one simulated quantity.
type XISimulatedRange struct {
	P10    float64
	Median float64
	P90    float64
}

// XISimulatedPlayer is one player's draws summarised: ranges, the median-band scorecard
// line (what the scorecard shows; the lines sum to the side total by construction) and
// the player's contribution to the side total's spread.
type XISimulatedPlayer struct {
	PlayerKey             string
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
}

// XISimulatedSide is one side's simulated innings.
type XISimulatedSide struct {
	Total           XISimulatedRange
	TotalScorecard  float64 // the median-band total the scorecard lines and extras sum to
	ExtrasScorecard float64
	Players         []XISimulatedPlayer
}

// XISimulationResult is the Go-side response from POST /simulate.
type XISimulationResult struct {
	Samples                      int
	TossMarginalised             bool
	SharedFactor                 bool
	Team1                        XISimulatedSide
	Team2                        XISimulatedSide
	SimulatedTeam1WinProbability float64
	DisplayTeam1WinProbability   float64
	// HeadlineTeam1WinProbability is the one to show, per E2's rule; HeadlineSource says
	// which model it came from ("display" or "simulator").
	HeadlineTeam1WinProbability float64
	HeadlineSource              string
	// Served is the rating state the draws were made from.
	Served ServedRatings
}

// Where a displayed win probability can come from. E2 chose per format on the folds; the
// simulator's answer is reported beside the display model's, never blended with it.
const (
	winProbabilitySourceDisplay   = "display"
	winProbabilitySourceSimulator = "simulator"
)

// WinProbabilitySources returns every value `win_probability.source` may carry (H-24).
//
// The Lab names the source beside the headline and opens its explainer from the name, so a
// value the UI cannot name would be a probability shown with no model behind it. Declared
// here, published in contracts/ops-console.contract.json, and asserted from the frontend
// (P1-4). ml-service is not a side of this vocabulary: it answers both models and go-app
// decides which is the headline.
func WinProbabilitySources() []string {
	return []string{winProbabilitySourceDisplay, winProbabilitySourceSimulator}
}

// simulatedFormats are the formats with an innings length; the simulator runs for these
// only (TEST has no innings to draw, plan H-17). Mirrors ml.xi.simulator.SIMULATED_FORMATS.
//
// The order is the one a caller offers them in, so the list and the lookup are one
// declaration: a format added to the slice is a format the lookup accepts.
var simulatedFormats = []string{"T20", "T20I", "ODI"}

// SimulatedFormatCodes returns the formats the simulator serves, for a caller that has to
// offer them rather than test one.
func SimulatedFormatCodes() []string { return append([]string(nil), simulatedFormats...) }

// Forecast sources, mirroring predictteam.ForecastSummary.Source.
const (
	forecastSourceSimulator = "simulator"
	forecastSourceQuantiles = "performance_quantiles"
)

// ForecastSources returns every value `forecast.source` may carry (H-24): the same
// contract as WinProbabilitySources, for the model behind the per-player numbers.
func ForecastSources() []string {
	return []string{forecastSourceSimulator, forecastSourceQuantiles}
}

// FormatHasInningsLength reports whether the simulator runs for the format.
func FormatHasInningsLength(format string) bool {
	normalized := NormalizeFormat(format)
	for _, code := range simulatedFormats {
		if code == normalized {
			return true
		}
	}
	return false
}

// applyXISimulation fills the scorecard, the per-player points and their ranges from one
// set of draws, and takes the headline probability the simulator reports (E2's rule).
//
// A failure here is a failure of the request: there is no second scorecard to fall back to,
// and answering with an XI and no numbers would be a silently narrower response.
func applyXISimulation(
	ctx context.Context,
	simulator XISimulator,
	fix fixture,
	xi1, xi2 []string,
	result *Result,
) error {
	sim, err := simulator.SimulateMatchXI(ctx, XISimulationRequest{
		Format:          fix.format,
		Team1PlayerKeys: xi1,
		Team2PlayerKeys: xi2,
		Team1ID:         fix.team1.ClubID,
		Team2ID:         fix.team2.ClubID,
		VenueID:         fix.venueID,
		Team1BatsFirst:  fix.team1BatsFirst,
		AsOf:            fix.asOf,
		MatchDate:       fix.matchDate,
	})
	if err != nil {
		return fmt.Errorf("simulate match: %w", err)
	}
	if len(sim.Team1.Players) == 0 || len(sim.Team2.Players) == 0 {
		return fmt.Errorf("simulate match: the simulator returned no players")
	}
	// The toss the draws used has to be the toss that was asked for. A known toss answered
	// by marginalised draws -- or the reverse -- is the request being silently changed, and
	// `toss_marginalised` is the one field that can catch it (§8.7).
	if sim.TossMarginalised != (fix.team1BatsFirst == nil) {
		return fmt.Errorf(
			"simulate match: the toss was %s but the simulator reports toss_marginalised=%t",
			tossDescription(fix.team1BatsFirst), sim.TossMarginalised)
	}
	// The headline is E2's choice, made on the folds and served as a constant. An
	// unrecognised source would put a number on screen with no honest label for it.
	if sim.HeadlineSource != winProbabilitySourceDisplay && sim.HeadlineSource != winProbabilitySourceSimulator {
		return fmt.Errorf("simulate match: unknown win-probability source %q", sim.HeadlineSource)
	}
	// The draws have to come from the rating state the XIs were chosen from, or the
	// scorecard describes a different run than the selection beside it (P1-5).
	if err := result.Adopt(sim.Served); err != nil {
		return fmt.Errorf("simulate match: %w", err)
	}

	if err := applySimulatedSide(result.Team1, sim.Team1); err != nil {
		return fmt.Errorf("simulate match: %w", err)
	}
	if err := applySimulatedSide(result.Team2, sim.Team2); err != nil {
		return fmt.Errorf("simulate match: %w", err)
	}
	result.Forecast = ForecastSummary{Source: forecastSourceSimulator}
	result.Toss = tossApplied(fix.team1BatsFirst)
	result.Scorecard = &Scorecard{
		Samples:          sim.Samples,
		TossMarginalised: sim.TossMarginalised,
		SharedFactor:     sim.SharedFactor,
		Team1Innings:     inningsTotal(sim.Team1),
		Team2Innings:     inningsTotal(sim.Team2),
	}
	simulated := sim.SimulatedTeam1WinProbability
	result.WinProbability = WinProbabilitySummary{
		Team1:           sim.HeadlineTeam1WinProbability,
		Source:          sim.HeadlineSource,
		Simulated:       &simulated,
		PredictedWinner: winnerFrom(sim.HeadlineTeam1WinProbability, fix.team1, fix.team2),
	}
	slog.InfoContext(ctx, "xi simulation applied to the scorecard",
		slog.String("format", fix.format),
		slog.Int("samples", sim.Samples),
		slog.Float64("team1_innings", sim.Team1.TotalScorecard),
		slog.Float64("team2_innings", sim.Team2.TotalScorecard),
		slog.Float64("p_display", sim.DisplayTeam1WinProbability),
		slog.Float64("p_simulated", simulated),
		slog.String("headline", sim.HeadlineSource),
		slog.String("toss", tossDescription(fix.team1BatsFirst)))
	return nil
}

// tossDescription names the batting order for a log line or an error message.
func tossDescription(team1BatsFirst *bool) string {
	if team1BatsFirst == nil {
		return "unknown"
	}
	if *team1BatsFirst {
		return "team1 bats first"
	}
	return "team2 bats first"
}

func inningsTotal(side XISimulatedSide) InningsTotal {
	return InningsTotal{
		Total:  side.TotalScorecard,
		Extras: side.ExtrasScorecard,
		P10:    side.Total.P10,
		Median: side.Total.Median,
		P90:    side.Total.P90,
	}
}

// applySimulatedSide writes the selected players' median-band scorecard lines, their 10-90
// ranges and their share of the innings total's spread.
//
// A player the simulator did not return is an error, not a warning. The simulator always
// returns the eleven it was sent, so this cannot happen without something being wrong --
// and the old behaviour, leaving the row at zeros, put "0 runs off 0 balls" on screen
// indistinguishable from a genuine forecast (§8.7: a substitution has to be visible, and
// substituting zeros for a forecast cannot be made visible).
func applySimulatedSide(players []SelectedPlayer, side XISimulatedSide) error {
	byKey := make(map[string]XISimulatedPlayer, len(side.Players))
	for _, p := range side.Players {
		byKey[p.PlayerKey] = p
	}
	for i := range players {
		sp, ok := byKey[players[i].PlayerKey]
		if !ok {
			return fmt.Errorf("the simulator returned no line for selected player %q", players[i].PlayerKey)
		}
		spread := sp.SpreadShare
		players[i].Runs = sp.ScorecardRuns
		players[i].Balls = sp.ScorecardBalls
		players[i].Wickets = sp.ScorecardWickets
		players[i].RunsConceded = sp.ScorecardRunsConceded
		players[i].Economy = economy(sp.ScorecardRunsConceded, sp.ScorecardBallsBowled)
		players[i].RunsRange = &ValueRange{P10: sp.Runs.P10, P90: sp.Runs.P90}
		players[i].BallsRange = &ValueRange{P10: sp.BallsFaced.P10, P90: sp.BallsFaced.P90}
		players[i].WicketsRange = &ValueRange{P10: sp.Wickets.P10, P90: sp.Wickets.P90}
		players[i].RunsConcededRange = &ValueRange{P10: sp.RunsConceded.P10, P90: sp.RunsConceded.P90}
		players[i].SpreadShare = &spread
	}
	return nil
}

// economy is runs conceded per over; zero when the player did not bowl.
func economy(runsConceded, ballsBowled float64) float64 {
	if ballsBowled <= 0 {
		return 0
	}
	return runsConceded / (ballsBowled / 6.0)
}

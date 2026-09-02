package predictteam

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// XIPerformancePredictor is the ml-service /performance/predict contract (L2-B): each
// player's distribution of runs, balls faced, wickets and runs conceded, for two elevens.
//
// It is what a format with no innings length gets instead of the simulator: the same
// forecasts the simulator would have drawn from, reported directly. There is no innings to
// draw, so there is no total and no median-band scorecard — and the response says so by
// carrying no scorecard rather than by inventing one.
type XIPerformancePredictor interface {
	PredictPerformance(ctx context.Context, req XIPerformanceRequest) (*XIPerformanceResult, error)
}

// XIPerformanceRequest is the Go-side payload for POST /performance/predict.
type XIPerformanceRequest struct {
	Format          string
	Team1PlayerKeys []string
	Team2PlayerKeys []string
	Team1ID         int64
	Team2ID         int64
	VenueID         int64
	AsOf            time.Time
}

// XIPerformancePlayer is one player's forecast: the median of each target with its 10-90
// interval, and the expected wickets.
type XIPerformancePlayer struct {
	PlayerKey    string
	Runs         XISimulatedRange
	BallsFaced   XISimulatedRange
	RunsConceded XISimulatedRange
	Wickets      float64
}

// XIPerformanceResult is the Go-side response from POST /performance/predict.
type XIPerformanceResult struct {
	InningsMarginalised bool
	Players             []XIPerformancePlayer
}

// applyPerformanceForecast writes each selected player's median and 10-90 range straight
// from L2-B. No totals: summing eleven medians is not an innings, and the thing that used
// to produce one here was the extras model plus a rescale, both deleted in P-5.
func applyPerformanceForecast(
	ctx context.Context,
	predictor XIPerformancePredictor,
	fix fixture,
	xi1, xi2 []string,
	result *Result,
) error {
	forecast, err := predictor.PredictPerformance(ctx, XIPerformanceRequest{
		Format:          fix.format,
		Team1PlayerKeys: xi1,
		Team2PlayerKeys: xi2,
		Team1ID:         fix.team1ID,
		Team2ID:         fix.team2ID,
		VenueID:         fix.venueID,
		AsOf:            fix.asOf,
	})
	if err != nil {
		return fmt.Errorf("performance forecast: %w", err)
	}
	byKey := make(map[string]XIPerformancePlayer, len(forecast.Players))
	for _, p := range forecast.Players {
		byKey[p.PlayerKey] = p
	}
	if err := applyForecastToSide(result.Team1, byKey); err != nil {
		return fmt.Errorf("performance forecast: %w", err)
	}
	if err := applyForecastToSide(result.Team2, byKey); err != nil {
		return fmt.Errorf("performance forecast: %w", err)
	}
	// §8.7: the substitution is named on the wire, not only in this log line. The caller
	// asked for a match forecast and got per-player distributions instead, because this
	// format has no innings length for the simulator to draw.
	result.Forecast = ForecastSummary{
		Source: forecastSourceQuantiles,
		Note: "This format has no innings length, so there is no simulated match: the " +
			"per-player numbers are the performance model's own quantiles, and there is no total.",
	}
	slog.InfoContext(ctx, "performance forecast applied",
		slog.String("format", fix.format),
		slog.Int("players", len(forecast.Players)),
		slog.Bool("innings_marginalised", forecast.InningsMarginalised))
	return nil
}

// applyForecastToSide writes each player's median and 10-90 range. A player the response
// does not carry is an error for the same reason it is in the simulator path: a row left
// at zeros reads as a forecast of nothing rather than as a missing forecast.
func applyForecastToSide(players []SelectedPlayer, byKey map[string]XIPerformancePlayer) error {
	for i := range players {
		p, ok := byKey[players[i].PlayerKey]
		if !ok {
			return fmt.Errorf("no forecast for selected player %q", players[i].PlayerKey)
		}
		players[i].Runs = p.Runs.Median
		players[i].Balls = p.BallsFaced.Median
		players[i].Wickets = p.Wickets
		players[i].RunsConceded = p.RunsConceded.Median
		players[i].RunsRange = &ValueRange{P10: p.Runs.P10, P90: p.Runs.P90}
		players[i].BallsRange = &ValueRange{P10: p.BallsFaced.P10, P90: p.BallsFaced.P90}
		players[i].RunsConcededRange = &ValueRange{P10: p.RunsConceded.P10, P90: p.RunsConceded.P90}
	}
	return nil
}

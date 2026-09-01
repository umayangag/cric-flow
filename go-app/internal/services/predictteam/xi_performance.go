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
	Format         string
	Team1PlayerIDs []int64
	Team2PlayerIDs []int64
	Team1ID        int64
	Team2ID        int64
	VenueID        int64
	AsOf           time.Time
}

// XIPerformancePlayer is one player's forecast: the median of each target with its 10-90
// interval, and the expected wickets.
type XIPerformancePlayer struct {
	PlayerID     int64
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
	xi1, xi2 []int64,
	result *Result,
) error {
	forecast, err := predictor.PredictPerformance(ctx, XIPerformanceRequest{
		Format:         fix.format,
		Team1PlayerIDs: xi1,
		Team2PlayerIDs: xi2,
		Team1ID:        fix.team1ID,
		Team2ID:        fix.team2ID,
		VenueID:        fix.venueID,
		AsOf:           fix.asOf,
	})
	if err != nil {
		return fmt.Errorf("performance forecast: %w", err)
	}
	byID := make(map[int64]XIPerformancePlayer, len(forecast.Players))
	for _, p := range forecast.Players {
		byID[p.PlayerID] = p
	}
	applyForecastToSide(result.Team1, byID)
	applyForecastToSide(result.Team2, byID)
	slog.InfoContext(ctx, "performance forecast applied",
		slog.String("format", fix.format),
		slog.Int("players", len(forecast.Players)),
		slog.Bool("innings_marginalised", forecast.InningsMarginalised))
	return nil
}

func applyForecastToSide(players []SelectedPlayer, byID map[int64]XIPerformancePlayer) {
	for i := range players {
		p, ok := byID[players[i].PlayerID]
		if !ok {
			slog.Warn("performance forecast: selected player missing from the response",
				slog.Int64("player_id", players[i].PlayerID))
			continue
		}
		players[i].Runs = p.Runs.Median
		players[i].Balls = p.BallsFaced.Median
		players[i].Wickets = p.Wickets
		players[i].RunsConceded = p.RunsConceded.Median
		players[i].RunsRange = &ValueRange{P10: p.Runs.P10, P90: p.Runs.P90}
		players[i].BallsRange = &ValueRange{P10: p.BallsFaced.P10, P90: p.BallsFaced.P90}
		players[i].RunsConcededRange = &ValueRange{P10: p.RunsConceded.P10, P90: p.RunsConceded.P90}
	}
}

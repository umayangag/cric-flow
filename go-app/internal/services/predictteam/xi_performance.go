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
	// Team1BatsFirst is nil before the toss, when the model averages both batting orders.
	// The Lab's quantiles path leaves it nil — a format with no innings length has no toss
	// — and the auction's projection sets it where the operator asks a toss-known question.
	Team1BatsFirst *bool
	AsOf           time.Time
}

// XIPerformancePlayer is one player's forecast: the median of each target with its 10-90
// interval, and the wicket count's distribution.
//
// The wickets are an expectation and three probabilities and never a range. L2-B predicts
// wickets as a count distribution rather than through quantile heads, so there is no q10 or
// q90 on this path and deriving one from the probabilities would be an interval the model
// never produced (P1-4's rule, applied to the one target that has no interval).
type XIPerformancePlayer struct {
	PlayerKey string
	// Side is 1 for team1 and 2 for team2, as ml-service reported it. It is half the
	// identity of a row: the response is one flat list for both elevens, so a registry id
	// alone names a row only while no player is on both sides -- which is now enforced
	// (GO-04) and was, before that, an assumption that quietly gave one side the other's
	// forecast.
	Side         int
	Runs         XISimulatedRange
	BallsFaced   XISimulatedRange
	RunsConceded XISimulatedRange
	Wickets      float64
	// WicketsP0, WicketsP1 and WicketsP2Plus are P(0), P(1) and P(2 or more).
	WicketsP0     float64
	WicketsP1     float64
	WicketsP2Plus float64
}

// XIPerformanceResult is the Go-side response from POST /performance/predict.
type XIPerformanceResult struct {
	InningsMarginalised bool
	Players             []XIPerformancePlayer
	// The ground as the model read it, off the rows it consumed (P3-2). The performance
	// model reads a ground through these two columns and through nothing else — the
	// ground's scoring level was gated and recorded as a null (A-1), and
	// `FIXTURE_CONTEXT_FAMILIES_KEPT` is empty — so a caller comparing two grounds is
	// comparing what the toss does at each.
	VenueBatFirstRate float64
	VenueMatches      float64
	// VenueNeutral is true where the served state has no matches at the ground and the
	// rows read at the prior. Carried so a surface can say so rather than presenting a
	// substitution nobody can see (§8.7).
	VenueNeutral bool
	// Served is the rating state the forecasts were made from.
	Served ServedRatings
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
		Team1ID:         fix.team1.ClubID,
		Team2ID:         fix.team2.ClubID,
		VenueID:         fix.venueID,
		AsOf:            fix.asOf,
	})
	if err != nil {
		return fmt.Errorf("performance forecast: %w", err)
	}
	if err := result.Adopt(forecast.Served); err != nil {
		return fmt.Errorf("performance forecast: %w", err)
	}
	byKey := forecastsBySidePlayer(forecast.Players)
	if err := applyForecastToSide(result.Team1, Team1Side, byKey); err != nil {
		return fmt.Errorf("performance forecast: %w", err)
	}
	if err := applyForecastToSide(result.Team2, Team2Side, byKey); err != nil {
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

// The two sides as /performance/predict and /simulate number them.
const (
	Team1Side = 1
	Team2Side = 2
)

// sidePlayerKey identifies one forecast row: which side it is for and which player.
//
// The registry id alone would not. `/performance/predict` answers both elevens in one flat
// list, and a map keyed by id alone silently keeps whichever row came last for a player who
// appeared twice (GO-04).
type sidePlayerKey struct {
	side      int
	playerKey string
}

// forecastsBySidePlayer indexes the response's flat player list by side and registry id.
func forecastsBySidePlayer(players []XIPerformancePlayer) map[sidePlayerKey]XIPerformancePlayer {
	byKey := make(map[sidePlayerKey]XIPerformancePlayer, len(players))
	for _, p := range players {
		byKey[sidePlayerKey{side: p.Side, playerKey: p.PlayerKey}] = p
	}
	return byKey
}

// applyForecastToSide writes each player's median and 10-90 range. A player the response
// does not carry is an error for the same reason it is in the simulator path: a row left
// at zeros reads as a forecast of nothing rather than as a missing forecast.
func applyForecastToSide(
	players []SelectedPlayer,
	side int,
	byKey map[sidePlayerKey]XIPerformancePlayer,
) error {
	for i := range players {
		p, ok := byKey[sidePlayerKey{side: side, playerKey: players[i].PlayerKey}]
		if !ok {
			return fmt.Errorf("no forecast for selected player %q on side %d", players[i].PlayerKey, side)
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

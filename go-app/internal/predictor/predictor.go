// Package predictor defines types and helpers for computing team performance and predictions.
package predictor

import (
	"github.com/umayangag/cric-info-scrapers/go-app/internal/config"
)

// PlayerPrediction aggregates player features and predicted metrics.
type PlayerPrediction struct {
	PlayerName         string
	RunsScored         float64
	BallsFaced         float64
	FoursScored        float64
	SixesScored        float64
	BattingPosition    float64
	StrikeRate         float64
	RunsConceded       float64
	Deliveries         float64
	WicketsTaken       float64
	Econ               float64
	WinningProbability float64 // Added this field
}

// Team summarizes team-level aggregates computed from player predictions.
type Team struct {
	Players            []PlayerPrediction
	TotalScore         float64
	TotalWickets       float64
	TotalBalls         float64
	Target             float64
	Extras             float64
	MatchNumber        int64
	WinningProbability float64
}

// CalculateOverallPerformanceWithConfig is a pure variant that accepts configuration and predicted extras.
// predictedExtras must be supplied from a model or historical average (e.g. db.GetAverageExtrasForFormat); no default constant is used.
func CalculateOverallPerformanceWithConfig(cfg *config.Config, players []PlayerPrediction, matchID int64, predictedExtras float64) Team {
	if cfg == nil {
		cfg = &config.Config{}
	}

	// Guard against division by zero; return a zero-valued team with predicted extras.
	if len(players) == 0 {
		return Team{
			Players:      players,
			TotalScore:   predictedExtras,
			TotalWickets: 10,
			TotalBalls:   0,
			Target:       0,
			Extras:       predictedExtras,
			MatchNumber:  matchID,
		}
	}

	magicNumber := float64(cfg.Predictor.TeamSize) / float64(len(players))

	var totalRunsScored, totalBallsFaced, totalRunsConceded, totalDeliveries, totalWicketsTaken float64
	for _, p := range players {
		totalRunsScored += p.RunsScored
		totalBallsFaced += p.BallsFaced
		totalRunsConceded += p.RunsConceded
		totalDeliveries += p.Deliveries
		totalWicketsTaken += p.WicketsTaken
	}

	totalScore := totalRunsScored*magicNumber + predictedExtras
	target := totalRunsConceded * magicNumber
	totalBalls := totalBallsFaced * magicNumber

	team := Team{
		Players:      players,
		TotalScore:   totalScore,
		TotalWickets: 10,
		TotalBalls:   totalBalls,
		Target:       target,
		Extras:       predictedExtras,
		MatchNumber:  matchID,
	}

	return team
}

package predictor

import (
	"github.com/umayangag/cric-info-scrapers/go-app/internal/config"
)

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

func CalculateOverallPerformance(players []PlayerPrediction, matchID int64) Team {
	cfg := config.Load()

	magicNumber := float64(cfg.Predictor.TeamSize) / float64(len(players))
	extras := cfg.Predictor.DefaultExtras

	var totalRunsScored, totalBallsFaced, totalRunsConceded, totalDeliveries, totalWicketsTaken float64
	for _, p := range players {
		totalRunsScored += p.RunsScored
		totalBallsFaced += p.BallsFaced
		totalRunsConceded += p.RunsConceded
		totalDeliveries += p.Deliveries
		totalWicketsTaken += p.WicketsTaken
	}

	totalScore := totalRunsScored*magicNumber + extras
	target := totalRunsConceded * magicNumber
	totalBalls := totalBallsFaced * magicNumber

	team := Team{
		Players:      players,
		TotalScore:   totalScore,
		TotalWickets: 10,
		TotalBalls:   totalBalls,
		Target:       target,
		Extras:       extras,
		MatchNumber:  matchID,
	}

	return team
}

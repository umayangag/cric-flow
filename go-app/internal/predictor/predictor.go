// Package predictor defines types and helpers for computing team performance and predictions.
package predictor

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

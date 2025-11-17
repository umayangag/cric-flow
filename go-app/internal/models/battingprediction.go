package models

// BattingPrediction mirrors ml-service response.
type BattingPrediction struct {
	RunsScored      float32 `json:"runs_scored"`
	BallsFaced      float32 `json:"balls_faced"`
	FoursScored     float32 `json:"fours_scored"`
	SixesScored     float32 `json:"sixes_scored"`
	BattingPosition float32 `json:"batting_position"`
	StrikeRate      float32 `json:"strike_rate"`
}

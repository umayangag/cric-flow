package models

// BowlingPrediction mirrors ml-service response.
type BowlingPrediction struct {
	RunsConceded float32 `json:"runs_conceded"`
	Deliveries   float32 `json:"deliveries"`
	WicketsTaken float32 `json:"wickets_taken"`
	Econ         float32 `json:"econ"`
}

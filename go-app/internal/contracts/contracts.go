// Package contracts contains types used by the API.
package contracts

// BattingFeatures aligns with ml-service BattingFeatures schema.
// Keep names/types in sync with ml-service/app/main.py.
type BattingFeatures struct {
	BattingConsistency float32 `json:"batting_consistency"`
	BattingForm        float32 `json:"batting_form"`
	BattingTemp        int     `json:"batting_temp"`
	BattingWind        int     `json:"batting_wind"`
	BattingRain        int     `json:"batting_rain"`
	BattingHumidity    int     `json:"batting_humidity"`
	BattingCloud       int     `json:"batting_cloud"`
	BattingPressure    int     `json:"batting_pressure"`
	BattingViscosity   int     `json:"batting_viscosity"`
	BattingInning      int     `json:"batting_inning"`
	BattingSession     int     `json:"batting_session"`
	Toss               int     `json:"toss"`
	Venue              float32 `json:"venue"`
	Opposition         float32 `json:"opposition"`
	Season             int     `json:"season"`
	PlayerName         string  `json:"player_name"`
	Format             string  `json:"format,omitempty"`
}

// BowlingFeatures aligns with ml-service BowlingFeatures schema.
type BowlingFeatures struct {
	BowlingConsistency float32 `json:"bowling_consistency"`
	BowlingForm        float32 `json:"bowling_form"`
	BowlingTemp        int     `json:"bowling_temp"`
	BowlingWind        int     `json:"bowling_wind"`
	BowlingRain        int     `json:"bowling_rain"`
	BowlingHumidity    int     `json:"bowling_humidity"`
	BowlingCloud       int     `json:"bowling_cloud"`
	BowlingPressure    int     `json:"bowling_pressure"`
	BowlingViscosity   int     `json:"bowling_viscosity"`
	BattingInning      int     `json:"batting_inning"`
	BowlingSession     int     `json:"bowling_session"`
	Toss               int     `json:"toss"`
	BowlingVenue       float32 `json:"bowling_venue"`
	BowlingOpposition  float32 `json:"bowling_opposition"`
	Season             int     `json:"season"`
	PlayerName         string  `json:"player_name"`
	Format             string  `json:"format,omitempty"`
}

// BattingPrediction mirrors ml-service response.
type BattingPrediction struct {
	RunsScored      float32 `json:"runs_scored"`
	BallsFaced      float32 `json:"balls_faced"`
	FoursScored     float32 `json:"fours_scored"`
	SixesScored     float32 `json:"sixes_scored"`
	BattingPosition float32 `json:"batting_position"`
	StrikeRate      float32 `json:"strike_rate"`
}

// BowlingPrediction mirrors ml-service response.
type BowlingPrediction struct {
	RunsConceded float32 `json:"runs_conceded"`
	Deliveries   float32 `json:"deliveries"`
	WicketsTaken float32 `json:"wickets_taken"`
	Econ         float32 `json:"econ"`
}

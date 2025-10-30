package contracts

// BattingFeatures mirrors the input_batting_columns from the Python prototype.
type BattingFeatures struct {
	BattingConsistency float32 `json:"batting_consistency"`
	BattingForm        float32 `json:"batting_form"`
	BattingTemp        int32   `json:"batting_temp"`
	BattingWind        int32   `json:"batting_wind"`
	BattingRain        int32   `json:"batting_rain"`
	BattingHumidity    int32   `json:"batting_humidity"`
	BattingCloud       int32   `json:"batting_cloud"`
	BattingPressure    int32   `json:"batting_pressure"`
	BattingViscosity   int32   `json:"batting_viscosity"`
	BattingInning      int32   `json:"batting_inning"`
	BattingSession     int32   `json:"batting_session"`
	Toss               int32   `json:"toss"`
	Venue              float32 `json:"venue"`
	Opposition         float32 `json:"opposition"`
	Season             int32   `json:"season"`
	PlayerName         string  `json:"player_name"`
}

// BowlingFeatures mirrors the input_bowling_columns from the Python prototype.
type BowlingFeatures struct {
	BowlingConsistency float32 `json:"bowling_consistency"`
	BowlingForm        float32 `json:"bowling_form"`
	BowlingTemp        int32   `json:"bowling_temp"`
	BowlingWind        int32   `json:"bowling_wind"`
	BowlingRain        int32   `json:"bowling_rain"`
	BowlingHumidity    int32   `json:"bowling_humidity"`
	BowlingCloud       int32   `json:"bowling_cloud"`
	BowlingPressure    int32   `json:"bowling_pressure"`
	BowlingViscosity   int32   `json:"bowling_viscosity"`
	BattingInning      int32   `json:"batting_inning"`
	BowlingSession     int32   `json:"bowling_session"`
	Toss               int32   `json:"toss"`
	BowlingVenue       float32 `json:"bowling_venue"`
	BowlingOpposition  float32 `json:"bowling_opposition"`
	Season             int32   `json:"season"`
	PlayerName         string  `json:"player_name"`
}

// BattingPrediction mirrors output_batting_columns + derived.
type BattingPrediction struct {
	RunsScored     float32 `json:"runs_scored"`
	BallsFaced     float32 `json:"balls_faced"`
	FoursScored    float32 `json:"fours_scored"`
	SixesScored    float32 `json:"sixes_scored"`
	BattingPos     float32 `json:"batting_position"`
	StrikeRate     float32 `json:"strike_rate"`
	Contribution   float32 `json:"batting_contribution"`
	PlayerName     string  `json:"player_name"`
}

// BowlingPrediction mirrors output_bowling_columns + derived.
type BowlingPrediction struct {
	RunsConceded   float32 `json:"runs_conceded"`
	Deliveries     float32 `json:"deliveries"`
	WicketsTaken   float32 `json:"wickets_taken"`
	Econ           float32 `json:"econ"`
	Contribution   float32 `json:"bowling_contribution"`
	PlayerName     string  `json:"player_name"`
}

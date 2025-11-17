package models

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

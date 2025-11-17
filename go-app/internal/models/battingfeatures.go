package models

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

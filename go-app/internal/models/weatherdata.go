package models

// WeatherData represents a weather snapshot for a match and session.
type WeatherData struct {
	MatchID   int64
	Session   string // e.g., "batting" or "bowling"
	Temp      int
	Wind      int
	Rain      int
	Humidity  int
	Cloud     int
	Pressure  int
	Viscosity string // e.g., dry/humid/windy; service may normalize
}

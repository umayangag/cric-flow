package models

import "time"

// Forecast captures the minimal weather attributes we care about for a match
// and an innings session. This shape is intentionally small to keep the
// interface stable while we evaluate providers.
//
// All times are in UTC; callers are responsible for timezone conversions.
// Values may be zero when unknown; providers should document semantics.
type Forecast struct {
	MatchID   int64
	Innings   int // 1 or 2 (batting/bowling innings)
	Timestamp time.Time
	// Core features (extend as needed)
	TemperatureC float64
	HumidityPct  float64
	WindKph      float64
	PrecipMM     float64
}

package features

import (
	"math"
	"time"
)

const twoPi = 2.0 * math.Pi

// TemporalFeatures computes cyclical temporal features from a match date.
// Returns month_sin, month_cos, day_of_week_sin, day_of_week_cos.
// These replace raw match_date_unix and season_id to prevent temporal leakage.
func TemporalFeatures(t time.Time) (monthSin, monthCos, dowSin, dowCos float64) {
	month := float64(t.Month()) // 1–12
	dow := float64(t.Weekday()) // 0=Sun in Go; convert to 0=Mon to match Python
	// Python: Monday=0 … Sunday=6.  Go: Sunday=0 … Saturday=6.
	// Convert: Go Sunday(0) → Python Sunday(6), Go Monday(1) → Python Monday(0), etc.
	dow = math.Mod(dow+6, 7)

	monthSin = math.Sin(twoPi * month / 12.0)
	monthCos = math.Cos(twoPi * month / 12.0)
	dowSin = math.Sin(twoPi * dow / 7.0)
	dowCos = math.Cos(twoPi * dow / 7.0)
	return
}

// TemporalFeaturesFromUnix computes cyclical temporal features from a unix timestamp.
// A zero timestamp is treated as missing data and yields neutral (0,0,0,0) features,
// consistent with the Python temporal pipeline.
func TemporalFeaturesFromUnix(unixSec float64) (monthSin, monthCos, dowSin, dowCos float64) {
	if unixSec == 0 {
		return 0, 0, 0, 0
	}
	t := time.Unix(int64(unixSec), 0).UTC()
	return TemporalFeatures(t)
}

// TemporalFeatureMap returns temporal features as a map keyed by canonical column names.
// Suitable for merging into a feature vector map.
func TemporalFeatureMap(t time.Time) map[string]float64 {
	ms, mc, ds, dc := TemporalFeatures(t)
	return map[string]float64{
		"month_sin":       ms,
		"month_cos":       mc,
		"day_of_week_sin": ds,
		"day_of_week_cos": dc,
	}
}

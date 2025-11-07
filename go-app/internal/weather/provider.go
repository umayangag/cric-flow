// Package weather provides interfaces and helpers to ingest weather data
// for cricket matches into the database. This is a scaffold aligned with the HLD
// and will be extended with real providers and orchestration.
package weather

import (
	"context"
	"time"
)

// Forecast captures the minimal weather attributes we care about for a match
// and an innings session. This shape is intentionally small to keep the
// interface stable while we evaluate providers.
//
// All times are in UTC; callers are responsible for timezone conversions.
// Values may be zero when unknown; providers should document semantics.
type Forecast struct {
	MatchID   int64
	Innings   int    // 1 or 2 (batting/bowling innings)
	Timestamp time.Time
	// Core features (extend as needed)
	TemperatureC float64
	HumidityPct  float64
	WindKph      float64
	PrecipMM     float64
}

// Provider defines a source that can return historical or forecast weather
// observations for a given match and innings.
type Provider interface {
	Name() string
	GetForecast(ctx context.Context, matchID int64) ([]Forecast, error)
}

// Upserter abstracts persistence; implemented in internal/db. We define a
// minimal contract here to decouple ingestion orchestration from the DB layer.
type Upserter interface {
	UpsertForecasts(ctx context.Context, f []Forecast) error
}

// Ingest orchestrates provider → normalize → persist.
func Ingest(ctx context.Context, p Provider, u Upserter, matchID int64) error {
	if p == nil || u == nil {
		return ErrInvalidArgs
	}
	forecasts, err := p.GetForecast(ctx, matchID)
	if err != nil {
		return err
	}
	return u.UpsertForecasts(ctx, forecasts)
}

// Errors
var (
	ErrInvalidArgs = Err("invalid arguments")
)

type Err string

func (e Err) Error() string { return string(e) }

package db

import (
	"context"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/wx"
)

// WeatherRepo abstracts persistence for weather records.
// Keep minimal to simplify unit testing.
//
//go:generate mockery --name WeatherRepo --output internal/mocks --case underscore
type WeatherRepo interface {
	// UpsertWeather stores or updates a single weather record for a match/session.
	UpsertWeather(ctx context.Context, r wx.Record) error
}

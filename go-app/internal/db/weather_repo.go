package db

import (
	"context"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/models"
)

// WeatherRepo abstracts persistence for weather records.
// Keep minimal to simplify unit testing.
type WeatherRepo interface {
	// UpsertWeather stores or updates a single weather record for a match/session.
	UpsertWeather(ctx context.Context, r models.WeatherData) error
}

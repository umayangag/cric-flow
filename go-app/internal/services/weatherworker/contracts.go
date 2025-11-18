package weatherworker

import (
	"context"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/models"
)

// Provider defines a source that can return historical or forecast weather
// observations for a given match and innings.
type Provider interface {
	Fetch(ctx context.Context, matchID int64) ([]models.WeatherData, error)
}

// Repository defines a destination for weather observations.
type Repository interface {
	UpsertWeather(ctx context.Context, r models.WeatherData) error
}

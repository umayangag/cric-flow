package dummy

import (
	"context"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/models"
)

// Provider is a simple in-process weather provider for smoke/testing flows.
// It returns two synthetic records per match (batting/bowling sessions) with
// deterministic values.
//
// NOTE: This adapter is intended for wiring in cmd during refactors; production
// providers can be added under internal/adapters/weather/<name> implementing the
// same interface and possibly backed by HTTP.
type Provider struct{}

func New() *Provider { return &Provider{} }

func (p *Provider) Fetch(_ context.Context, matchID int64) ([]models.WeatherData, error) {
	return []models.WeatherData{
		{
			MatchID:   matchID,
			Session:   "batting",
			Temp:      25,
			Wind:      5,
			Rain:      0,
			Humidity:  40,
			Cloud:     10,
			Pressure:  1010,
			Viscosity: "dry",
		},
		{
			MatchID:   matchID,
			Session:   "bowling",
			Temp:      24,
			Wind:      7,
			Rain:      0,
			Humidity:  45,
			Cloud:     20,
			Pressure:  1012,
			Viscosity: "humid",
		},
	}, nil
}

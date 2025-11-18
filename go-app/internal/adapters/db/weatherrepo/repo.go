package weatherrepo

import (
	"context"

	appdb "github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/models"
)

// Repository implements db.WeatherRepo with a thin SQL upsert into weather_data.
// It assumes a unique key on (match_id, session). This keeps adapter tiny and
// allows the service to remain fully unit-testable.
// No unit tests here to avoid DB dependency; rely on smoke runs.

type Repository struct{}

func New() *Repository { return &Repository{} }

func (r *Repository) UpsertWeather(ctx context.Context, rec models.WeatherData) error {
	if appdb.Pool == nil {
		if _, err := appdb.Connect(ctx); err != nil {
			return err
		}
	}
	// Minimal upsert; columns mirror those used in exporters' queries.
	_, err := appdb.Pool.Exec(ctx, `
		INSERT INTO weather_data (
			match_id, session, temp, wind, rain, humidity, cloud, pressure, viscosity
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (match_id, session) DO UPDATE SET
			temp = EXCLUDED.temp,
			wind = EXCLUDED.wind,
			rain = EXCLUDED.rain,
			humidity = EXCLUDED.humidity,
			cloud = EXCLUDED.cloud,
			pressure = EXCLUDED.pressure,
			viscosity = EXCLUDED.viscosity
	`, rec.MatchID, rec.Session, rec.Temp, rec.Wind, rec.Rain, rec.Humidity, rec.Cloud, rec.Pressure, rec.Viscosity)
	return err
}

package db

import (
	"context"
	"errors"
)

// Weather represents a weather_data row.
// Nullable numeric/text fields use pointers; nil means unknown/no update on upsert when keeping existing.
type Weather struct {
	ID        int64   // not used on upsert
	MatchID   int64   // required
	Session   string  // required, composite key with MatchID
	Temp      *int
	Feels     *int
	Wind      *int
	Gust      *int
	Rain      *int
	Humidity  *int
	Cloud     *int
	Pressure  *int
	Viscosity *string
}

// UpsertWeather inserts or updates weather_data by (match_id, session).
func UpsertWeather(ctx context.Context, w *Weather) error {
	if Pool == nil { return errors.New("db pool not initialized") }
	// On conflict update only provided fields; if nil, keep existing using COALESCE pattern.
	_, err := Pool.Exec(ctx, `INSERT INTO weather_data(
		match_id, session, temp, feels, wind, gust, rain, humidity, cloud, pressure, viscosity)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		ON CONFLICT (match_id, session) DO UPDATE SET
			temp = COALESCE(EXCLUDED.temp, weather_data.temp),
			feels = COALESCE(EXCLUDED.feels, weather_data.feels),
			wind = COALESCE(EXCLUDED.wind, weather_data.wind),
			gust = COALESCE(EXCLUDED.gust, weather_data.gust),
			rain = COALESCE(EXCLUDED.rain, weather_data.rain),
			humidity = COALESCE(EXCLUDED.humidity, weather_data.humidity),
			cloud = COALESCE(EXCLUDED.cloud, weather_data.cloud),
			pressure = COALESCE(EXCLUDED.pressure, weather_data.pressure),
			viscosity = COALESCE(EXCLUDED.viscosity, weather_data.viscosity)
	`, w.MatchID, w.Session, w.Temp, w.Feels, w.Wind, w.Gust, w.Rain, w.Humidity, w.Cloud, w.Pressure, w.Viscosity)
	return err
}

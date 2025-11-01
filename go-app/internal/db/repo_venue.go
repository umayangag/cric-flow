package db

import (
	"context"
	"errors"
)

// Venue represents a row in the venue table with normalized/geocoded fields.
type Venue struct {
	ID             int64
	VenueName      string
	NormalizedName string
	DisplayName    string
	City           *string
	Country        *string
	Latitude       *float64
	Longitude      *float64
	Timezone       *string
	Source         *string
	Confidence     *float32
}

// LookupVenueByNormalizedName fetches a venue by normalized_name.
func LookupVenueByNormalizedName(ctx context.Context, norm string) (*Venue, error) {
	if Pool == nil {
		return nil, errors.New("db pool not initialized")
	}
	row := Pool.QueryRow(ctx, `SELECT id, venue_name, COALESCE(normalized_name,''), COALESCE(display_name,''), city, country,
		latitude, longitude, timezone, source, confidence
		FROM venue WHERE normalized_name = $1`, norm)
	var v Venue
	var city, country, tz, src *string
	var lat, lon *float64
	var conf *float32
	if err := row.Scan(&v.ID, &v.VenueName, &v.NormalizedName, &v.DisplayName, &city, &country, &lat, &lon, &tz, &src, &conf); err != nil {
		return nil, err
	}
	v.City = city
	v.Country = country
	v.Latitude = lat
	v.Longitude = lon
	v.Timezone = tz
	v.Source = src
	v.Confidence = conf
	return &v, nil
}

// UpsertVenueGeocode updates geocode fields for a venue identified by normalized_name,
// creating a new venue row if needed (using display_name as venue_name).
func UpsertVenueGeocode(ctx context.Context, norm, display string, city, country *string, lat, lon float64, timezone string, source string, confidence float32) (int64, error) {
	if Pool == nil {
		return 0, errors.New("db pool not initialized")
	}
	var id int64
	// Ensure a row exists keyed by normalized_name; set venue_name/display_name if missing.
	err := Pool.QueryRow(ctx, `INSERT INTO venue(venue_name, normalized_name, display_name, city, country, latitude, longitude, timezone, source, confidence)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT (normalized_name) DO UPDATE SET
			display_name = COALESCE(EXCLUDED.display_name, venue.display_name),
			city = COALESCE(EXCLUDED.city, venue.city),
			country = COALESCE(EXCLUDED.country, venue.country),
			latitude = COALESCE(EXCLUDED.latitude, venue.latitude),
			longitude = COALESCE(EXCLUDED.longitude, venue.longitude),
			timezone = COALESCE(EXCLUDED.timezone, venue.timezone),
			source = EXCLUDED.source,
			confidence = EXCLUDED.confidence
		RETURNING id`, display, norm, display, city, country, lat, lon, timezone, source, confidence).Scan(&id)
	return id, err
}

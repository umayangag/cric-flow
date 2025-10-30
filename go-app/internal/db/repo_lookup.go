package db

import (
	"context"
	"errors"
)

// GetOrCreateVenue returns venue.id for a given venue_name, creating it if necessary.
func GetOrCreateVenue(ctx context.Context, name string) (int64, error) {
	if Pool == nil { return 0, errors.New("db pool not initialized") }
	var id int64
	err := Pool.QueryRow(ctx, `INSERT INTO venue(venue_name) VALUES($1)
		ON CONFLICT (venue_name) DO UPDATE SET venue_name = EXCLUDED.venue_name
		RETURNING id`, name).Scan(&id)
	return id, err
}

// GetOrCreateOpposition returns opposition.id for a given opposition_name, creating it if necessary.
func GetOrCreateOpposition(ctx context.Context, name string) (int64, error) {
	if Pool == nil { return 0, errors.New("db pool not initialized") }
	var id int64
	err := Pool.QueryRow(ctx, `INSERT INTO opposition(opposition_name) VALUES($1)
		ON CONFLICT (opposition_name) DO UPDATE SET opposition_name = EXCLUDED.opposition_name
		RETURNING id`, name).Scan(&id)
	return id, err
}

// GetOrCreateSeason returns season.id for a given season_name, creating it if necessary.
func GetOrCreateSeason(ctx context.Context, name string) (int64, error) {
	if Pool == nil { return 0, errors.New("db pool not initialized") }
	var id int64
	err := Pool.QueryRow(ctx, `INSERT INTO season(season_name) VALUES($1)
		ON CONFLICT (season_name) DO UPDATE SET season_name = EXCLUDED.season_name
		RETURNING id`, name).Scan(&id)
	return id, err
}

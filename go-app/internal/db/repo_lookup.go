package db

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
)

// FindVenueIDByName returns the id of the venue with exactly this name, and whether the
// database holds one.
//
// It reads and never writes, which is the whole point of it existing beside
// GetOrCreateVenue: creating a venue is an import-time act, where the name comes from a
// match file that is about to reference it. A request naming a ground is not that. Asking
// the get-or-create for it inserted a venue on every typo, and returned an id with no
// history behind it as though it had been found (GO-08).
//
// The match is on `venue_name` -- the same key GetOrCreateVenue conflicts on, and the same
// string `/api/options/venues` offers -- and is deliberately exact. Folding spellings
// together is venue identity, which is IMPORT-08 and DATA-02's subject, not this one's.
func FindVenueIDByName(ctx context.Context, name string) (int64, bool, error) {
	if Pool == nil {
		return 0, false, errors.New("db pool not initialized")
	}
	var id int64
	err := Pool.QueryRow(ctx, `SELECT id FROM venue WHERE venue_name = $1`, name).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		slog.Error("db.FindVenueIDByName failed", slog.String("venue", name), slog.Any("err", err))
		return 0, false, fmt.Errorf("look up venue %q: %w", name, err)
	}
	return id, true, nil
}

// GetOrCreateVenue returns venue.id for a given venue_name, creating it if necessary. It
// belongs to the import path, which is the only caller entitled to bring a venue into
// existence; a request that merely names one reads it with FindVenueIDByName.
func GetOrCreateVenue(ctx context.Context, name string) (int64, error) {
	if Pool == nil {
		return 0, errors.New("db pool not initialized")
	}
	var id int64
	err := Pool.QueryRow(ctx, `INSERT INTO venue(venue_name) VALUES($1)
		ON CONFLICT (venue_name) DO UPDATE SET venue_name = EXCLUDED.venue_name
		RETURNING id`, name).Scan(&id)
	return id, err
}

// GetOrCreateOpposition returns opposition.id for a team, creating it if necessary.
//
// A team is (name, gender): 130 of the 394 names in the dataset are used by both a men's
// and a women's side, and one integer standing for both is a model input that means
// different things in different rows. Gender comes from info.gender, which every match
// file carries, so an empty gender here is a caller that has lost track of its match
// rather than a team whose gender is unknown -- and the NOT NULL column will say so.
func GetOrCreateOpposition(ctx context.Context, name, gender string) (int64, error) {
	if Pool == nil {
		return 0, errors.New("db pool not initialized")
	}
	var id int64
	err := Pool.QueryRow(ctx, `INSERT INTO opposition(opposition_name, gender) VALUES($1, $2)
		ON CONFLICT (opposition_name, gender) DO UPDATE SET opposition_name = EXCLUDED.opposition_name
		RETURNING id`, name, gender).Scan(&id)
	return id, err
}

// GetOrCreateSeason returns season.id for a given season_name, creating it if necessary.
func GetOrCreateSeason(ctx context.Context, name string) (int64, error) {
	if Pool == nil {
		return 0, errors.New("db pool not initialized")
	}
	var id int64
	err := Pool.QueryRow(ctx, `INSERT INTO season(season_name) VALUES($1)
		ON CONFLICT (season_name) DO UPDATE SET season_name = EXCLUDED.season_name
		RETURNING id`, name).Scan(&id)
	return id, err
}

// GetOrCreateMatchFormat returns match_format.id for a given code, creating it if necessary.
func GetOrCreateMatchFormat(ctx context.Context, code string) (int64, error) {
	if Pool == nil {
		return 0, errors.New("db pool not initialized")
	}
	var id int64
	err := Pool.QueryRow(ctx, `INSERT INTO match_format(code, name) VALUES($1, $1)
		ON CONFLICT (code) DO UPDATE SET code = EXCLUDED.code
		RETURNING id`, code).Scan(&id)
	return id, err
}

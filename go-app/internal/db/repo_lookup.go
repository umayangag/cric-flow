package db

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/umayangag/cric-flow/go-app/internal/venues"
)

// FindVenueIDByName returns the id of the venue this name identifies, and whether the
// database holds one.
//
// It reads and never writes, which is the whole point of it existing beside
// GetOrCreateVenue: creating a venue is an import-time act, where the name comes from a
// match file that is about to reference it. A request naming a ground is not that. Asking
// the get-or-create for it inserted a venue on every typo, and returned an id with no
// history behind it as though it had been found (GO-08). That split is unchanged here --
// this function still only reads, and the importer is still the only caller that may
// create.
//
// What changed is the key. The match is on `normalized_name`, the folded identity key both
// this function and GetOrCreateVenue compute with venues.NormalizeName, so the two agree on
// what counts as the same ground (IMPORT-08). "M.Chinnaswamy Stadium" and "M Chinnaswamy
// Stadium" are one venue; "County Ground, Bristol" and "County Ground, Derby" remain two.
// A name that folds to nothing -- empty, or punctuation only -- identifies no venue and is
// reported as not held rather than sent to the database.
func FindVenueIDByName(ctx context.Context, name string) (int64, bool, error) {
	if Pool == nil {
		return 0, false, errors.New("db pool not initialized")
	}
	normalized := venues.NormalizeName(name)
	if normalized == "" {
		return 0, false, nil
	}
	var id int64
	err := Pool.QueryRow(ctx, `SELECT id FROM venue WHERE normalized_name = $1`, normalized).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		slog.Error("db.FindVenueIDByName failed", slog.String("venue", name), slog.Any("err", err))
		return 0, false, fmt.Errorf("look up venue %q: %w", name, err)
	}
	return id, true, nil
}

// GetOrCreateVenue returns venue.id for a ground named by a match file, creating it if the
// database does not hold it yet. It belongs to the import path, which is the only caller
// entitled to bring a venue into existence; a request that merely names one reads it with
// FindVenueIDByName.
//
// The row's identity is `normalized_name`, not the spelling this particular file used: the
// conflict target is the folded key, so the second spelling of a ground returns the first
// spelling's id instead of creating a rival venue with its own familiarity history
// (IMPORT-08). `venue_name` keeps the spelling that created the row, which is the string
// `/api/options/venues` offers and the one FindVenueIDByName will fold back to this key.
//
// `city` is what the match file said the ground is in, and it is filled only while the
// column is still empty: the first city the archive names beside a ground wins, and no
// later file rewrites it. An empty city writes nothing.
func GetOrCreateVenue(ctx context.Context, name, city string) (int64, error) {
	if Pool == nil {
		return 0, errors.New("db pool not initialized")
	}
	normalized := venues.NormalizeName(name)
	if normalized == "" {
		return 0, fmt.Errorf("venue name %q has no identity once folded", name)
	}
	var id int64
	err := Pool.QueryRow(ctx, `INSERT INTO venue(venue_name, normalized_name, city)
		VALUES($1, $2, NULLIF($3, ''))
		ON CONFLICT (normalized_name) DO UPDATE
		SET city = COALESCE(NULLIF(venue.city, ''), NULLIF(EXCLUDED.city, ''))
		RETURNING id`, name, normalized, city).Scan(&id)
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

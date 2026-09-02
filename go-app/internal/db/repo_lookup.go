package db

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
)

// GetOrCreateVenue returns venue.id for a given venue_name, creating it if necessary.
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

// TeamRename is one club's change of name: both rows are (name, gender). See
// internal/teamlineage.
type TeamRename struct {
	FromName string
	ToName   string
	Gender   string
}

// ApplyTeamLineage points every superseded team row at the club's current row.
//
// Both rows have to exist for a rename to mean anything, and a dataset that stops at the
// boundary legitimately has only the old one. That is reported rather than treated as an
// error: the mapping describes cricket, not this particular import.
//
// Returns the number of rows it *changed*, not the number linked: a row already pointing at
// the right club is left alone, so a second run over an unchanged dataset returns zero and
// that is the healthy answer, not a missing link.
func ApplyTeamLineage(ctx context.Context, renames []TeamRename) (int, error) {
	if Pool == nil {
		return 0, errors.New("db pool not initialized")
	}
	linked := 0
	for _, rename := range renames {
		tag, err := Pool.Exec(ctx, `
			UPDATE opposition predecessor
			SET canonical_id = successor.id
			FROM opposition successor
			WHERE predecessor.opposition_name = $1
			  AND successor.opposition_name = $2
			  AND predecessor.gender = $3
			  AND successor.gender = $3
			  AND predecessor.id <> successor.id
			  AND predecessor.canonical_id IS DISTINCT FROM successor.id`,
			rename.FromName, rename.ToName, rename.Gender)
		if err != nil {
			slog.Error("apply team lineage failed",
				slog.String("from", rename.FromName),
				slog.String("to", rename.ToName),
				slog.String("gender", rename.Gender),
				slog.Any("err", err))
			return linked, fmt.Errorf("link %q to %q (%s): %w", rename.FromName, rename.ToName, rename.Gender, err)
		}
		if tag.RowsAffected() == 0 {
			slog.Info("team lineage: nothing to link, one side of the rename is not in this dataset",
				slog.String("from", rename.FromName),
				slog.String("to", rename.ToName),
				slog.String("gender", rename.Gender))
			continue
		}
		linked += int(tag.RowsAffected())
	}
	return linked, nil
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

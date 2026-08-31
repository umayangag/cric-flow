package db

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
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

// ErrOppositionNotFound reports that no team with the given name has played the format.
var ErrOppositionNotFound = errors.New("no opposition found for team name")

// FindOppositionIDForFormat resolves a team *name* to one opposition id for a request
// that does not carry a gender.
//
// The serving surfaces take a team name and a format and nothing else, so after the
// gender split a name alone can mean two teams. Rather than invent a gender or return
// both, this picks the side that has actually played the format, most recently -- which
// is the side a prediction request for that format is asking about -- and logs when the
// name was ambiguous, because that log is the record of a request that could not say
// what it meant. The real fix is for the caller to carry the gender; that arrives with
// the L2 serving path (P-5), which takes ids rather than names.
func FindOppositionIDForFormat(ctx context.Context, name, formatCode string) (int64, error) {
	if Pool == nil {
		return 0, errors.New("db pool not initialized")
	}
	rows, err := Pool.Query(ctx, `
		SELECT o.id, o.gender, max(m.match_date) AS last_played
		FROM opposition o
		JOIN match_inning mi ON mi.batting_team_opposition_id = o.id OR mi.bowling_team_opposition_id = o.id
		JOIN match m ON m.match_id = mi.match_id
		JOIN match_format mf ON mf.id = m.format_id
		WHERE o.opposition_name = $1 AND mf.code = $2
		GROUP BY o.id, o.gender
		ORDER BY last_played DESC, o.id`, name, formatCode)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	type candidate struct {
		id     int64
		gender string
	}
	var candidates []candidate
	for rows.Next() {
		var c candidate
		var lastPlayed time.Time
		if err := rows.Scan(&c.id, &c.gender, &lastPlayed); err != nil {
			return 0, err
		}
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(candidates) == 0 {
		return 0, fmt.Errorf("%w: %q in %s", ErrOppositionNotFound, name, formatCode)
	}
	if len(candidates) > 1 {
		genders := make([]string, 0, len(candidates))
		for _, c := range candidates {
			genders = append(genders, c.gender)
		}
		slog.Warn("team name matches more than one side; using the most recently active",
			slog.String("team", name),
			slog.String("format", formatCode),
			slog.String("genders", strings.Join(genders, ", ")),
			slog.String("chosen_gender", candidates[0].gender))
	}
	return candidates[0].id, nil
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

package db

import (
	"context"
	"errors"
	"strings"
)

// getUniqueStringsWithParams runs a query that returns a single string column and returns distinct values.
// Use for option lists (teams, formats, opponents). Pass query args after the query.
func getUniqueStringsWithParams(ctx context.Context, query string, args ...any) ([]string, error) {
	if defaultDB == nil {
		return nil, errors.New("db pool not initialized")
	}
	rows, err := Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		results = append(results, s)
	}
	return results, rows.Err()
}

// GetUniqueFormats returns a list of unique match format codes from the match_format table.
func GetUniqueFormats(ctx context.Context) ([]string, error) {
	return getUniqueStringsWithParams(ctx, "SELECT code FROM match_format ORDER BY code")
}

// Team option lists are not here. A team is (name, gender), so a list of names is not a
// list of teams: the three name-keyed queries that used to live here fed a picker that
// could not say which side it meant (D-10). They are replaced by the side queries in
// repo_team_side.go, which return the club id every downstream read is keyed by.

// GetVenuesByQuery returns the venue names that match the query, for the picker.
// Query must be at least 3 characters; otherwise returns nil, nil (no error, no results).
// Matching is case-insensitive (ILIKE) with the pattern %q%.
//
// It offers `venue_name` and nothing else, because that is the string the caller will send
// back and every string this list offers must resolve. It used to offer
// the trimmed `display_name` and fell back to `venue_name` only when it was blank, while
// venue resolution matched `venue_name`: a venue with a display name would have been listed
// under one string and refused under it with VENUE_NOT_FOUND. Nothing has ever written
// `venue.display_name`, so the mismatch never fired -- it sat waiting for the first writer,
// which is the shape of bug `normalized_name` already was (IMPORT-08).
func GetVenuesByQuery(ctx context.Context, q string) ([]string, error) {
	q = strings.TrimSpace(q)
	if len(q) < 3 {
		return nil, nil
	}
	pattern := "%" + q + "%"
	query := `
		SELECT DISTINCT venue_name AS name
		FROM venue
		WHERE venue_name ILIKE $1
		ORDER BY name
		LIMIT 50
	`
	return getUniqueStringsWithParams(ctx, query, pattern)
}

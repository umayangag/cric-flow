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
// could not say which side it meant (D-11). They are replaced by the side queries in
// repo_team_side.go, which return the club id every downstream read is keyed by.

// GetVenuesByQuery returns venue names (display_name or venue_name) that match the query.
// Query must be at least 3 characters; otherwise returns nil, nil (no error, no results).
// Matching is case-insensitive (ILIKE) with the pattern %q%.
func GetVenuesByQuery(ctx context.Context, q string) ([]string, error) {
	q = strings.TrimSpace(q)
	if len(q) < 3 {
		return nil, nil
	}
	pattern := "%" + q + "%"
	query := `
		SELECT DISTINCT COALESCE(NULLIF(trim(display_name), ''), venue_name) AS name
		FROM venue
		WHERE (COALESCE(display_name, venue_name) ILIKE $1 OR venue_name ILIKE $1)
		ORDER BY name
		LIMIT 50
	`
	return getUniqueStringsWithParams(ctx, query, pattern)
}

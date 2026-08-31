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

// GetUniqueTeams returns a list of unique team names from the opposition table.
//
// DISTINCT because a team is (name, gender) and 130 names hold two sides; this list feeds
// a name-only picker, so it lists each name once.
func GetUniqueTeams(ctx context.Context) ([]string, error) {
	return getUniqueStringsWithParams(ctx, "SELECT DISTINCT opposition_name FROM opposition ORDER BY opposition_name")
}

// GetUniqueFormats returns a list of unique match format codes from the match_format table.
func GetUniqueFormats(ctx context.Context) ([]string, error) {
	return getUniqueStringsWithParams(ctx, "SELECT code FROM match_format ORDER BY code")
}

// GetTeamsByFormat returns a list of unique team names that played in the given format.
// Collects teams from both batting and bowling positions (UNION), so teams that only
// appeared as bowling side are included, matching buildPlayedMatchesFiltersQuery.
func GetTeamsByFormat(ctx context.Context, format string) ([]string, error) {
	query := `
		SELECT DISTINCT team_name
		FROM (
			SELECT o.opposition_name AS team_name
			FROM match_inning mi
			JOIN match m ON m.match_id = mi.match_id
			JOIN match_format mf ON mf.id = m.format_id
			JOIN opposition o ON o.id = mi.batting_team_opposition_id
			WHERE mf.code = $1 AND o.opposition_name IS NOT NULL AND o.opposition_name != ''
			UNION
			SELECT o.opposition_name AS team_name
			FROM match_inning mi
			JOIN match m ON m.match_id = mi.match_id
			JOIN match_format mf ON mf.id = m.format_id
			JOIN opposition o ON o.id = mi.bowling_team_opposition_id
			WHERE mf.code = $1 AND o.opposition_name IS NOT NULL AND o.opposition_name != ''
		) t
		ORDER BY team_name
	`
	return getUniqueStringsWithParams(ctx, query, format)
}

// GetOpponentsByFormatAndTeam returns team names that played against the given team in the given format.
// For each inning, batting_team and bowling_team are the two sides; the opponent is the other team.
func GetOpponentsByFormatAndTeam(ctx context.Context, format, teamName string) ([]string, error) {
	query := `
		SELECT DISTINCT
		  CASE WHEN o_bat.opposition_name = $2 THEN o_bowl.opposition_name ELSE o_bat.opposition_name END AS opponent
		FROM match_inning mi
		JOIN match m ON m.match_id = mi.match_id
		JOIN match_format mf ON mf.id = m.format_id
		JOIN opposition o_bat ON o_bat.id = mi.batting_team_opposition_id
		JOIN opposition o_bowl ON o_bowl.id = mi.bowling_team_opposition_id
		WHERE mf.code = $1
		  AND (o_bat.opposition_name = $2 OR o_bowl.opposition_name = $2)
		  AND o_bat.opposition_name IS NOT NULL AND o_bat.opposition_name != ''
		  AND o_bowl.opposition_name IS NOT NULL AND o_bowl.opposition_name != ''
		ORDER BY opponent
	`
	return getUniqueStringsWithParams(ctx, query, format, teamName)
}

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

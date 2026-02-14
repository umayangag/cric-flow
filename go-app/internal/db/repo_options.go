package db

import (
	"context"
	"errors"
)

// GetUniqueTeams returns a list of unique team names from the opposition table.
func GetUniqueTeams(ctx context.Context) ([]string, error) {
	return getUniqueStrings(ctx, "SELECT opposition_name FROM opposition ORDER BY opposition_name")
}

// GetUniqueFormats returns a list of unique match format codes from the match_format table.
func GetUniqueFormats(ctx context.Context) ([]string, error) {
	return getUniqueStrings(ctx, "SELECT code FROM match_format ORDER BY code")
}

// GetTeamsByFormat returns a list of unique team names that played in the given format.
func GetTeamsByFormat(ctx context.Context, format string) ([]string, error) {
	query := `
		SELECT DISTINCT o.opposition_name
		FROM match_details md
		JOIN match_format mf ON mf.id = md.format_id
		JOIN opposition o ON o.id = md.opposition_id
		WHERE mf.code = $1 AND o.opposition_name IS NOT NULL AND o.opposition_name != ''
		ORDER BY o.opposition_name
	`
	if defaultDB == nil {
		return nil, errors.New("db pool not initialized")
	}
	rows, err := Query(ctx, query, format)
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

// GetOpponentsByFormatAndTeam returns a list of unique team names that played against the given team in the given format.
// The match_details table has multiple rows per match (one per inning), all sharing the same match_number.
// Each row has a different opposition_id, so the two distinct opposition_ids represent the two teams.
// Strategy: Find all match_numbers where the given team appears, then return all other teams from those matches.
func GetOpponentsByFormatAndTeam(ctx context.Context, format, teamName string) ([]string, error) {
	query := `
		SELECT DISTINCT o2.opposition_name
		FROM match_details md1
		JOIN match_format mf ON mf.id = md1.format_id
		JOIN opposition o1 ON o1.id = md1.opposition_id
		JOIN match_details md2 ON md2.match_number = md1.match_number AND md2.opposition_id != md1.opposition_id
		JOIN opposition o2 ON o2.id = md2.opposition_id
		WHERE mf.code = $1
		  AND o1.opposition_name = $2
		  AND o2.opposition_name IS NOT NULL
		  AND o2.opposition_name != ''
		ORDER BY o2.opposition_name
	`
	if defaultDB == nil {
		return nil, errors.New("db pool not initialized")
	}
	rows, err := Query(ctx, query, format, teamName)
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

func getUniqueStrings(ctx context.Context, query string) ([]string, error) {
	if defaultDB == nil {
		return nil, errors.New("db pool not initialized")
	}
	rows, err := Query(ctx, query)
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
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

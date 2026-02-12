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

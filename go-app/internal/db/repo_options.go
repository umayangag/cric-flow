package db

import (
	"context"
	"errors"
)

// GetUniqueTeams returns a list of unique team names from the opposition table.
func GetUniqueTeams(ctx context.Context) ([]string, error) {
	if Pool == nil {
		return nil, errors.New("db pool not initialized")
	}
	rows, err := Pool.Query(ctx, "SELECT opposition_name FROM opposition ORDER BY opposition_name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var teams []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		teams = append(teams, name)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return teams, nil
}

// GetUniqueFormats returns a list of unique match format codes from the match_format table.
func GetUniqueFormats(ctx context.Context) ([]string, error) {
	if Pool == nil {
		return nil, errors.New("db pool not initialized")
	}
	rows, err := Pool.Query(ctx, "SELECT code FROM match_format ORDER BY code")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var formats []string
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, err
		}
		formats = append(formats, code)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return formats, nil
}

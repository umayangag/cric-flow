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

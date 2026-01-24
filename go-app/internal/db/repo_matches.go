package db

import (
    "context"
    "database/sql"
    "errors"
    "time"
    "github.com/umayangag/cric-info-scrapers/go-app/internal/formats"
)

// MatchRow represents a minimal match listing row for API responses.
type MatchRow struct {
	MatchID    int64
	Date       time.Time
	FormatCode sql.NullString
	Teams      [2]string
}

// ListMatches returns matches within a season strictly after the given cutoff date,
// optionally filtered by match format code.
// Note: SQL joins assume presence of match_details (date, season_id, match_id, format_id),
// team_match (match_id, team_id), team (id, name), and match_format (id, code).
func ListMatches(ctx context.Context, season int, after time.Time, format string) ([]MatchRow, error) {
	if Pool == nil {
		return nil, errors.New("db pool not initialized")
	}

	// Base query aggregates two distinct team names per match and orders by date asc.
	// If schema differs, adjust accordingly in future iterations.
	q := `
        WITH teams AS (
            SELECT tm.match_id, array_agg(DISTINCT t.name ORDER BY t.name) AS team_names
            FROM team_match tm
            JOIN team t ON t.id = tm.team_id
            GROUP BY tm.match_id
        )
        SELECT md.match_id,
               md.date,
               COALESCE(mf.code, '') AS format_code,
               teams.team_names
        FROM match_details md
        JOIN season s ON s.id = md.season_id
        LEFT JOIN match_format mf ON mf.id = md.format_id
        JOIN teams ON teams.match_id = md.match_id
        WHERE s.id = $1 AND md.date > $2
    `
 args := []any{season, after}
 if format != "" {
     canon := formats.CanonicalizeCode(format)
     q += " AND mf.code = $3"
     args = append(args, canon)
 }
	q += " ORDER BY md.date ASC"

	rows, err := Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]MatchRow, 0)
	for rows.Next() {
		var (
			matchID  int64
			date     time.Time
			fcode    sql.NullString
			teamsArr []string
		)
		if err := rows.Scan(&matchID, &date, &fcode, &teamsArr); err != nil {
			return nil, err
		}
		if len(teamsArr) != 2 {
			// Skip incomplete matches silently; caller expects exactly 2 teams
			continue
		}
		out = append(out, MatchRow{
			MatchID:    matchID,
			Date:       date,
			FormatCode: fcode,
			Teams:      [2]string{teamsArr[0], teamsArr[1]},
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

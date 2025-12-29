package db

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// BacktestCandidate represents a played match candidate for backtesting.
type BacktestCandidate struct {
	MatchID    int64
	StableID   sql.NullString
	Date       time.Time
	Venue      sql.NullString
	Season     sql.NullString
	FormatCode sql.NullString
	Team1      string
	Team2      string
	WinnerTeam sql.NullString
}

// ListPlayedMatchesByFormatAndTeams returns already-played matches filtered by
// format code and two team names. Teams are order-insensitive; results are ordered by date ASC.
// A match is considered "played" if its date is strictly before NOW().
// Note: Adjust schema/table names if they drift; this query expects:
//   - match_details(match_id, date, season_id, venue_id, format_id, stable_id)
//   - season(id, name)
//   - venue(id, name)
//   - match_format(id, code)
//   - team_match(match_id, team_id, result)
//   - team(id, name)
func ListPlayedMatchesByFormatAndTeams(
	ctx context.Context,
	formatCode string,
	team1 string,
	team2 string,
) ([]BacktestCandidate, error) {
	if Pool == nil {
		return nil, errors.New("db pool not initialized")
	}

	q := `
        WITH tm AS (
            SELECT tm.match_id,
                   MIN(t.name) AS team_a,
                   MAX(t.name) AS team_b,
                   MAX(CASE WHEN tm.result IN ('W','WIN','1','TRUE','T') THEN t.name ELSE NULL END) AS winner
            FROM team_match tm
            JOIN team t ON t.id = tm.team_id
            GROUP BY tm.match_id
        )
        SELECT md.match_id,
               COALESCE(md.stable_id, '') AS stable_id,
               md.date,
               COALESCE(v.name, '') AS venue_name,
               COALESCE(s.name, '') AS season_name,
               COALESCE(mf.code, '') AS format_code,
               tm.team_a,
               tm.team_b,
               COALESCE(tm.winner, '') AS winner
        FROM match_details md
        JOIN tm ON tm.match_id = md.match_id
        LEFT JOIN venue v ON v.id = md.venue_id
        LEFT JOIN season s ON s.id = md.season_id
        LEFT JOIN match_format mf ON mf.id = md.format_id
        WHERE md.date < NOW()
          AND mf.code = $1
          AND ((tm.team_a = $2 AND tm.team_b = $3) OR (tm.team_a = $3 AND tm.team_b = $2))
        ORDER BY md.date ASC
    `

	rows, err := Pool.Query(ctx, q, formatCode, team1, team2)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]BacktestCandidate, 0)
	for rows.Next() {
		var c BacktestCandidate
		if err := rows.Scan(
			&c.MatchID,
			&c.StableID,
			&c.Date,
			&c.Venue,
			&c.Season,
			&c.FormatCode,
			&c.Team1,
			&c.Team2,
			&c.WinnerTeam,
		); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

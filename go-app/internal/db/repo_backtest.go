package db

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/db/scanx"
)

// BacktestCandidate represents a played match candidate for backtesting.
type BacktestCandidate struct {
	MatchID    int64
	StableID   sql.NullString
	MatchDate  time.Time
	Venue      sql.NullString
	Season     sql.NullString
	FormatCode sql.NullString
	Team1      string
	Team2      string
	WinnerTeam sql.NullString
}

// buildPlayedMatchesFiltersQuery builds the SQL and argument list for querying
// already played matches with optional filters. Uses match and match_inning;
// both teams come from MIN/MAX(opposition_name) across innings per match.
func buildPlayedMatchesFiltersQuery(
	formatCode string,
	team1 string,
	team2 string,
	start time.Time,
	end time.Time,
	order string,
	limit int,
) (string, []any) {
	sb := strings.Builder{}
	sb.WriteString(`
        WITH match_teams AS (
            SELECT match_id,
                   MIN(team_name) AS team_a,
                   MAX(team_name) AS team_b
            FROM (
                SELECT mi.match_id, o.opposition_name AS team_name
                FROM match_inning mi
                JOIN opposition o ON o.id = mi.batting_team_opposition_id
                UNION
                SELECT mi.match_id, o.opposition_name AS team_name
                FROM match_inning mi
                JOIN opposition o ON o.id = mi.bowling_team_opposition_id
            ) t
            WHERE team_name IS NOT NULL AND team_name != ''
            GROUP BY match_id
        ),
        match_winner AS (
            SELECT m.match_id, o.opposition_name AS winner
            FROM match m
            JOIN opposition o ON o.id = m.outcome_winner_opposition_id
            WHERE m.outcome_winner_opposition_id IS NOT NULL
        )
        SELECT m.match_id,
               CAST(m.match_id AS TEXT) AS stable_id,
               m.match_date,
               COALESCE(v.display_name, v.venue_name, '') AS venue_name,
               COALESCE(s.season_name, '') AS season_name,
               COALESCE(mf.code, '') AS format_code,
               mt.team_a,
               mt.team_b,
               COALESCE(mw.winner, '') AS winner
        FROM match m
        JOIN match_teams mt ON mt.match_id = m.match_id
        LEFT JOIN match_winner mw ON mw.match_id = m.match_id
        LEFT JOIN venue v ON v.id = m.venue_id
        LEFT JOIN season s ON s.id = m.season_id
        LEFT JOIN match_format mf ON mf.id = m.format_id
        WHERE m.match_date < NOW()`)

	args := []any{}
	idx := 1

	if formatCode != "" {
		sb.WriteString(" AND mf.code = $")
		sb.WriteString(strconv.Itoa(idx))
		args = append(args, formatCode)
		idx++
	}
	if !start.IsZero() {
		sb.WriteString(" AND m.match_date >= $")
		sb.WriteString(strconv.Itoa(idx))
		args = append(args, start)
		idx++
	}
	if !end.IsZero() {
		sb.WriteString(" AND m.match_date <= $")
		sb.WriteString(strconv.Itoa(idx))
		args = append(args, end)
		idx++
	}
	if team1 != "" && team2 != "" {
		sb.WriteString(" AND ((mt.team_a = $")
		sb.WriteString(strconv.Itoa(idx))
		args = append(args, team1)
		idx++
		sb.WriteString(" AND mt.team_b = $")
		sb.WriteString(strconv.Itoa(idx))
		args = append(args, team2)
		idx++
		sb.WriteString(") OR (mt.team_a = $")
		sb.WriteString(strconv.Itoa(idx))
		args = append(args, team2)
		idx++
		sb.WriteString(" AND mt.team_b = $")
		sb.WriteString(strconv.Itoa(idx))
		args = append(args, team1)
		sb.WriteString("))")
	} else if team1 != "" || team2 != "" {
		// Single-team filter (order-insensitive)
		team := team1
		if team == "" {
			team = team2
		}
		sb.WriteString(" AND (mt.team_a = $")
		sb.WriteString(strconv.Itoa(idx))
		args = append(args, team)
		idx++
		sb.WriteString(" OR mt.team_b = $")
		sb.WriteString(strconv.Itoa(idx))
		args = append(args, team)
		sb.WriteString(")")
	}

	// Ordering
	if strings.ToLower(order) == "desc" {
		sb.WriteString(" ORDER BY m.match_date DESC")
	} else {
		sb.WriteString(" ORDER BY m.match_date ASC")
	}
	if limit > 0 {
		sb.WriteString(" LIMIT $")
		sb.WriteString(strconv.Itoa(idx))
		args = append(args, limit)
	}
	return sb.String(), args
}

// scanBacktestCandidate populates a BacktestCandidate from the current row.
func scanBacktestCandidate(rows scanx.Scanner, c *BacktestCandidate) error {
	return rows.Scan(
		&c.MatchID,
		&c.StableID,
		&c.MatchDate,
		&c.Venue,
		&c.Season,
		&c.FormatCode,
		&c.Team1,
		&c.Team2,
		&c.WinnerTeam,
	)
}

// ListPlayedMatchesByFormatAndTeams returns already-played matches filtered by
// format code and two team names. Teams are order-insensitive; results are ordered by date ASC.
// A match is considered "played" if its date is strictly before NOW().
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
        WITH match_teams AS (
            SELECT match_id,
                   MIN(team_name) AS team_a,
                   MAX(team_name) AS team_b
            FROM (
                SELECT mi.match_id, o.opposition_name AS team_name
                FROM match_inning mi
                JOIN opposition o ON o.id = mi.batting_team_opposition_id
                UNION
                SELECT mi.match_id, o.opposition_name AS team_name
                FROM match_inning mi
                JOIN opposition o ON o.id = mi.bowling_team_opposition_id
            ) t
            WHERE team_name IS NOT NULL AND team_name != ''
            GROUP BY match_id
        ),
        match_winner AS (
            SELECT m.match_id, o.opposition_name AS winner
            FROM match m
            JOIN opposition o ON o.id = m.outcome_winner_opposition_id
            WHERE m.outcome_winner_opposition_id IS NOT NULL
        )
        SELECT m.match_id,
               CAST(m.match_id AS TEXT) AS stable_id,
               m.match_date,
               COALESCE(v.display_name, v.venue_name, '') AS venue_name,
               COALESCE(s.season_name, '') AS season_name,
               COALESCE(mf.code, '') AS format_code,
               mt.team_a,
               mt.team_b,
               COALESCE(mw.winner, '') AS winner
        FROM match m
        JOIN match_teams mt ON mt.match_id = m.match_id
        LEFT JOIN match_winner mw ON mw.match_id = m.match_id
        LEFT JOIN venue v ON v.id = m.venue_id
        LEFT JOIN season s ON s.id = m.season_id
        LEFT JOIN match_format mf ON mf.id = m.format_id
        WHERE m.match_date < NOW()
          AND mf.code = $1
          AND ((mt.team_a = $2 AND mt.team_b = $3) OR (mt.team_a = $3 AND mt.team_b = $2))
        ORDER BY m.match_date ASC
    `

	rows, err := Pool.Query(ctx, q, formatCode, team1, team2)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]BacktestCandidate, 0)
	for rows.Next() {
		var c BacktestCandidate
		if err := scanBacktestCandidate(rows, &c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// ListPlayedMatchesByFilters returns already-played matches filtered by optional
// format, date range, and team codes. Results are ordered by date asc/desc and
// can be limited.
func ListPlayedMatchesByFilters(
	ctx context.Context,
	formatCode string,
	team1 string,
	team2 string,
	start time.Time,
	end time.Time,
	order string,
	limit int,
) ([]BacktestCandidate, error) {
	if Pool == nil {
		return nil, errors.New("db pool not initialized")
	}
	q, args := buildPlayedMatchesFiltersQuery(formatCode, team1, team2, start, end, order, limit)
	rows, err := Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]BacktestCandidate, 0)
	for rows.Next() {
		var c BacktestCandidate
		if err := scanBacktestCandidate(rows, &c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

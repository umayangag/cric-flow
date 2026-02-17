package db

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db/scanx"
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

// MatchAfterItem holds match_id and match_date for walk-forward (matches after a cutoff).
type MatchAfterItem struct {
	MatchID   int64
	MatchDate time.Time
}

// ListMatchIDsAfter returns match_id and match_date for matches strictly after the given cutoff,
// for the given format IDs (e.g. from GetFormatIDsForTrainingBucket). Order: match_date ASC.
// limit caps the number of matches (e.g. walk-forward window size).
func ListMatchIDsAfter(ctx context.Context, formatIDs []int64, after time.Time, limit int) ([]MatchAfterItem, error) {
	if Pool == nil {
		return nil, errors.New("db pool not initialized")
	}
	if limit <= 0 {
		limit = 100
	}
	q := `SELECT m.match_id, m.match_date
		FROM match m
		WHERE m.format_id = ANY($1::bigint[]) AND m.match_date > $2
		ORDER BY m.match_date ASC
		LIMIT $3`
	rows, err := Pool.Query(ctx, q, formatIDs, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MatchAfterItem
	for rows.Next() {
		var item MatchAfterItem
		if err := rows.Scan(&item.MatchID, &item.MatchDate); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// --- Match prediction aggregates cache ---

// MatchPredictionAggregates stores cached match-level prediction outputs.
// Only a minimal subset is currently needed by the server for accuracy metrics.
type MatchPredictionAggregates struct {
	MatchID             int64
	Format              string
	Team1Code           string
	Team2Code           string
	PredictedWinnerCode sql.NullString
	PredictedTotalRuns  sql.NullFloat64
	ModelVersion        sql.NullString
	CutoffAt            time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// GetMatchPredictionAggregates fetches a cached aggregates record for the match.
func GetMatchPredictionAggregates(ctx context.Context, matchID int64) (MatchPredictionAggregates, error) {
	if Pool == nil {
		return MatchPredictionAggregates{}, errors.New("db pool not initialized")
	}
	const q = `
        SELECT match_id, format, team1_code, team2_code,
               predicted_winner_code, predicted_total_runs, model_version,
               cutoff_at, created_at, updated_at
        FROM match_prediction_aggregates
        WHERE match_id = $1`
	var row MatchPredictionAggregates
	err := Pool.QueryRow(ctx, q, matchID).Scan(
		&row.MatchID,
		&row.Format,
		&row.Team1Code,
		&row.Team2Code,
		&row.PredictedWinnerCode,
		&row.PredictedTotalRuns,
		&row.ModelVersion,
		&row.CutoffAt,
		&row.CreatedAt,
		&row.UpdatedAt,
	)
	if err != nil {
		return MatchPredictionAggregates{}, err
	}
	return row, nil
}

// UpsertMatchPredictionAggregates inserts or updates a cached aggregates record for the match.
func UpsertMatchPredictionAggregates(ctx context.Context, row MatchPredictionAggregates) error {
	if Pool == nil {
		return errors.New("db pool not initialized")
	}
	const q = `
        INSERT INTO match_prediction_aggregates (
            match_id, format, team1_code, team2_code,
            predicted_winner_code, predicted_total_runs, model_version,
            cutoff_at
        ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
        ON CONFLICT (match_id) DO UPDATE SET
            format = EXCLUDED.format,
            team1_code = EXCLUDED.team1_code,
            team2_code = EXCLUDED.team2_code,
            predicted_winner_code = EXCLUDED.predicted_winner_code,
            predicted_total_runs = EXCLUDED.predicted_total_runs,
            model_version = EXCLUDED.model_version,
            cutoff_at = EXCLUDED.cutoff_at`
	_, err := Pool.Exec(ctx, q,
		row.MatchID,
		row.Format,
		row.Team1Code,
		row.Team2Code,
		row.PredictedWinnerCode,
		row.PredictedTotalRuns,
		row.ModelVersion,
		row.CutoffAt,
	)
	return err
}

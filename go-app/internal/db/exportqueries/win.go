package exportqueries

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// WinTrainingRows returns match-level rows for win prediction: format_id, venue_id, team1_opposition_id, team2_opposition_id, toss_winner_opposition_id, team1_wins (0/1).
// team1 = batting in inning 1, team2 = bowling in inning 1. team1_wins = 1 if outcome_winner_opposition_id == team1 else 0.
func WinTrainingRows(ctx context.Context, cutoff time.Time) ([][]string, error) {
	return winTrainingRowsImpl(ctx, cutoff, nil)
}

// WinTrainingRowsWithFormat returns win rows filtered by format code.
func WinTrainingRowsWithFormat(ctx context.Context, format string, cutoff time.Time) ([][]string, error) {
	formatIDs, err := db.GetGlobalCache().GetFormatIDsForTrainingBucket(ctx, format)
	if err != nil {
		return nil, err
	}
	return winTrainingRowsImpl(ctx, cutoff, formatIDs)
}

func winTrainingRowsImpl(ctx context.Context, cutoff time.Time, formatIDs []int64) ([][]string, error) {
	q := `SELECT m.match_id, m.format_id, COALESCE(m.venue_id, 0),
		mi.batting_team_opposition_id AS team1_opposition_id,
		mi.bowling_team_opposition_id AS team2_opposition_id,
		COALESCE(m.toss_winner_opposition_id, 0),
		CASE WHEN m.outcome_winner_opposition_id IS NULL THEN 0 WHEN m.outcome_winner_opposition_id = mi.batting_team_opposition_id THEN 1 ELSE 0 END AS team1_wins,
		COALESCE(mf.code, '') AS format_code
		FROM match m
		JOIN match_inning mi ON mi.match_id = m.match_id AND mi.inning_number = 1
		LEFT JOIN match_format mf ON m.format_id = mf.id
		WHERE m.match_date < $1`
	args := []any{cutoff}
	if formatIDs != nil {
		q = strings.Replace(
			q,
			"WHERE m.match_date < $1",
			"WHERE m.format_id = ANY($1::bigint[]) AND m.match_date < $2",
			1,
		)
		args = []any{formatIDs, cutoff}
	}
	rows, err := db.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	headers := []string{
		"match_id",
		"format_id",
		"venue_id",
		"team1_opposition_id",
		"team2_opposition_id",
		"toss_winner_opposition_id",
		"team1_wins",
		"format_code",
	}
	out := make([][]string, 0, 256)
	out = append(out, headers)
	for rows.Next() {
		var matchID, formatID, venueID, team1, team2, tossWinner int64
		var team1Wins int
		var formatCode string
		if err := rows.Scan(&matchID, &formatID, &venueID, &team1, &team2, &tossWinner, &team1Wins, &formatCode); err != nil {
			return nil, err
		}
		out = append(out, []string{
			strconv.FormatInt(matchID, 10),
			strconv.FormatInt(formatID, 10),
			strconv.FormatInt(venueID, 10),
			strconv.FormatInt(team1, 10),
			strconv.FormatInt(team2, 10),
			strconv.FormatInt(tossWinner, 10),
			strconv.Itoa(team1Wins),
			formatCode,
		})
	}
	return out, rows.Err()
}

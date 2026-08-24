package db

import (
	"context"
	"errors"
	"strconv"
	"time"
)

// MatchLite is a minimal projection to iterate matches chronologically.
type MatchLite struct {
	ID           int64
	MatchID      int64
	MatchDate    time.Time
	FormatID     int64
	OppositionID int64
	VenueID      int64
}

// ListPlayersWithHistoryBefore returns distinct player IDs who have batting or bowling
// records in the given format strictly before the cutoff date.
func ListPlayersWithHistoryBefore(ctx context.Context, formatID int64, cutoff time.Time) ([]int64, error) {
	if Pool == nil {
		return nil, errors.New("db pool not initialized")
	}
	rows, err := Pool.Query(ctx, `
		SELECT DISTINCT player_id
		FROM (
		  SELECT b.player_id
		  FROM batting_data b
		  JOIN match_inning mi ON mi.match_id = b.match_id AND mi.inning_number = b.inning_number
		  JOIN match m ON m.match_id = b.match_id
		  WHERE m.format_id = $1 AND m.match_date < $2
		  UNION
		  SELECT w.player_id
		  FROM bowling_data w
		  JOIN match_inning mi ON mi.match_id = w.match_id AND mi.inning_number = w.inning_number
		  JOIN match m ON m.match_id = w.match_id
		  WHERE m.format_id = $1 AND m.match_date < $2
		) t
		ORDER BY player_id ASC`, formatID, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ListMatchesByFormatDatePage returns a page of matches (keyset pagination) for replay.
// Use afterCursor=nil for the first page; then pass the last match of the previous page.
// pageSize 0 means no limit (returns all, same as ListMatchesByFormatDate before chunking).
func ListMatchesByFormatDatePage(
	ctx context.Context,
	formatID int64,
	from, to *time.Time,
	pageSize int,
	afterCursor *MatchLite,
) ([]MatchLite, error) {
	if Pool == nil {
		return nil, errors.New("db pool not initialized")
	}
	q := `SELECT match_id AS id, match_id, match_date, format_id, 0 AS opposition_id, COALESCE(venue_id,0)
		FROM match
		WHERE format_id = $1`
	args := []any{formatID}
	argNum := 2
	if from != nil {
		q += ` AND match_date >= $` + strconv.Itoa(argNum)
		args = append(args, *from)
		argNum++
	}
	if to != nil {
		q += ` AND match_date <= $` + strconv.Itoa(argNum)
		args = append(args, *to)
		argNum++
	}
	if afterCursor != nil {
		q += ` AND (match_date, match_id) > ($` + strconv.Itoa(argNum) + `, $` + strconv.Itoa(argNum+1) + `)`
		args = append(args, afterCursor.MatchDate, afterCursor.MatchID)
		argNum += 2
	}
	q += ` ORDER BY match_date ASC, match_id ASC`
	if pageSize > 0 {
		q += ` LIMIT $` + strconv.Itoa(argNum)
		args = append(args, pageSize)
	}
	rows, err := Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []MatchLite
	for rows.Next() {
		var m MatchLite
		if err := rows.Scan(&m.ID, &m.MatchID, &m.MatchDate, &m.FormatID, &m.OppositionID, &m.VenueID); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	return res, rows.Err()
}

// ListPlayersInMatch returns distinct player_ids appearing in batting or bowling for a match.
func ListPlayersInMatch(ctx context.Context, matchID int64) ([]int64, error) {
	if Pool == nil {
		return nil, errors.New("db pool not initialized")
	}
	rows, err := Pool.Query(ctx, `
		SELECT DISTINCT player_id FROM batting_data WHERE match_id = $1
		UNION
		SELECT DISTINCT player_id FROM bowling_data WHERE match_id = $1
	`, matchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// InnVal is a minimal row for historical performance values with date.
type InnVal struct {
	MatchDate time.Time
	Value     float64
}

// ListBattingBefore returns batting values (runs as Value) for a player strictly before cutoff date.
func ListBattingBefore(
	ctx context.Context,
	playerID int64,
	cutoff time.Time,
	formatID int64,
	oppID, venueID *int64,
) ([]InnVal, error) {
	if Pool == nil {
		return nil, errors.New("db pool not initialized")
	}
	q := `SELECT m.match_date, COALESCE(b.runs,0)::float8
		FROM batting_data b
		JOIN match_inning mi ON mi.match_id = b.match_id AND mi.inning_number = b.inning_number
		JOIN match m ON m.match_id = b.match_id
		WHERE b.player_id = $1 AND m.match_date < $2 AND m.format_id = $3`
	args := []any{playerID, cutoff, formatID}
	if oppID != nil {
		q += ` AND mi.bowling_team_opposition_id = $4`
		args = append(args, *oppID)
	}
	if venueID != nil {
		if oppID != nil {
			q += ` AND m.venue_id = $5`
		} else {
			q += ` AND m.venue_id = $4`
		}
		args = append(args, *venueID)
	}
	q += ` ORDER BY m.match_date ASC, b.id ASC`
	rows, err := Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []InnVal
	for rows.Next() {
		var iv InnVal
		if err := rows.Scan(&iv.MatchDate, &iv.Value); err != nil {
			return nil, err
		}
		res = append(res, iv)
	}
	return res, rows.Err()
}

// ListBowlingBefore returns bowling values (wickets as Value) for a player strictly before cutoff date.
func ListBowlingBefore(
	ctx context.Context,
	playerID int64,
	cutoff time.Time,
	formatID int64,
	oppID, venueID *int64,
) ([]InnVal, error) {
	if Pool == nil {
		return nil, errors.New("db pool not initialized")
	}
	q := `SELECT m.match_date, COALESCE(w.wickets,0)::float8
		FROM bowling_data w
		JOIN match_inning mi ON mi.match_id = w.match_id AND mi.inning_number = w.inning_number
		JOIN match m ON m.match_id = w.match_id
		WHERE w.player_id = $1 AND m.match_date < $2 AND m.format_id = $3`
	args := []any{playerID, cutoff, formatID}
	if oppID != nil {
		q += ` AND mi.batting_team_opposition_id = $4`
		args = append(args, *oppID)
	}
	if venueID != nil {
		if oppID != nil {
			q += ` AND m.venue_id = $5`
		} else {
			q += ` AND m.venue_id = $4`
		}
		args = append(args, *venueID)
	}
	q += ` ORDER BY m.match_date ASC, w.id ASC`
	rows, err := Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []InnVal
	for rows.Next() {
		var iv InnVal
		if err := rows.Scan(&iv.MatchDate, &iv.Value); err != nil {
			return nil, err
		}
		res = append(res, iv)
	}
	return res, rows.Err()
}

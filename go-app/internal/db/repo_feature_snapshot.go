package db

import (
	"context"
	"errors"
	"sort"
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

// ListMatchesByFormatDate returns matches filtered by format and date range inclusive, ordered by date asc, id asc.
func ListMatchesByFormatDate(ctx context.Context, formatID int64, from, to *time.Time) ([]MatchLite, error) {
	if Pool == nil {
		return nil, errors.New("db pool not initialized")
	}
	q := `SELECT match_id AS id, match_id, match_date, format_id, 0 AS opposition_id, COALESCE(venue_id,0)
		FROM match
		WHERE format_id = $1`
	args := []any{formatID}
	if from != nil {
		q += ` AND match_date >= $2`
		args = append(args, *from)
	}
	if to != nil {
		if len(args) == 1 {
			q += ` AND match_date <= $2`
		} else {
			q += ` AND match_date <= $3`
		}
		args = append(args, *to)
	}
	q += ` ORDER BY match_date ASC, id ASC`
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

// ListBattingBefore returns batting values (runs as Value) for a player strictly before cutoff date, filtered by optional format/opposition/venue.
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

// ListBowlingBefore returns bowling values (wickets as Value) for a player strictly before cutoff date, filtered by optional format/opposition/venue.
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

// ListFieldingBefore returns fielding involvements (catches + run_outs*1.5 + stumpings) per match
// for a player strictly before cutoff, in the given format.
func ListFieldingBefore(ctx context.Context, playerID int64, cutoff time.Time, formatID int64) ([]InnVal, error) {
	if Pool == nil {
		return nil, errors.New("db pool not initialized")
	}
	rows, err := Pool.Query(ctx, `SELECT m.match_date,
		(COALESCE(fd.catches,0) + COALESCE(fd.run_outs,0)*1.5 + COALESCE(fd.stumpings,0))::float8
		FROM fielding_data fd
		JOIN match m ON m.match_id = fd.match_id
		WHERE fd.player_id = $1 AND m.match_date < $2 AND m.format_id = $3
		ORDER BY m.match_date ASC`, playerID, cutoff, formatID)
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

// HistQueryKey identifies a history lookup for bulk fetch: PlayerID, Cutoff, FormatID, OppID (0=overall), VenueID (0=overall).
type HistQueryKey struct {
	P int64
	T time.Time
	F int64
	O int64 // 0 = no filter
	V int64 // 0 = no filter
}

type battingBulkRow struct {
	playerID   int64
	formatID   int64
	matchDate  time.Time
	value      float64
	opposition int64
	venue      int64
}

// ListBattingBeforeBulk fetches batting history for multiple keys in one query to avoid N+1.
func ListBattingBeforeBulk(ctx context.Context, keys []HistQueryKey) (map[HistQueryKey][]InnVal, error) {
	if Pool == nil {
		return nil, errors.New("db pool not initialized")
	}
	if len(keys) == 0 {
		return map[HistQueryKey][]InnVal{}, nil
	}
	seenPF := make(map[struct{ P, F int64 }]struct{})
	var maxCutoff time.Time
	for _, k := range keys {
		seenPF[struct{ P, F int64 }{k.P, k.F}] = struct{}{}
		if k.T.After(maxCutoff) {
			maxCutoff = k.T
		}
	}
	var pids, fids []int64
	for pf := range seenPF {
		pids = append(pids, pf.P)
		fids = append(fids, pf.F)
	}
	q := `SELECT b.player_id, m.format_id, m.match_date, COALESCE(b.runs,0)::float8,
		COALESCE(mi.bowling_team_opposition_id, 0),
		COALESCE(m.venue_id, 0)
		FROM batting_data b
		JOIN match_inning mi ON mi.match_id = b.match_id AND mi.inning_number = b.inning_number
		JOIN match m ON m.match_id = b.match_id
		WHERE (b.player_id, m.format_id) IN (SELECT * FROM unnest($1::bigint[], $2::bigint[]))
		AND m.match_date < $3
		ORDER BY b.player_id, m.format_id, m.match_date ASC, b.id ASC`
	rows, err := Pool.Query(ctx, q, pids, fids, maxCutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var all []battingBulkRow
	for rows.Next() {
		var r battingBulkRow
		if err := rows.Scan(&r.playerID, &r.formatID, &r.matchDate, &r.value, &r.opposition, &r.venue); err != nil {
			return nil, err
		}
		all = append(all, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make(map[HistQueryKey][]InnVal)
	for _, k := range keys {
		var list []InnVal
		for _, r := range all {
			if r.playerID != k.P || r.formatID != k.F || !r.matchDate.Before(k.T) {
				continue
			}
			if k.O != 0 && r.opposition != k.O {
				continue
			}
			if k.V != 0 && r.venue != k.V {
				continue
			}
			list = append(list, InnVal{MatchDate: r.matchDate, Value: r.value})
		}
		out[k] = list
	}
	return out, nil
}

type bowlingBulkRow struct {
	playerID   int64
	formatID   int64
	matchDate  time.Time
	value      float64
	opposition int64
	venue      int64
}

// ListBowlingBeforeBulk fetches bowling history for multiple keys in one query to avoid N+1.
func ListBowlingBeforeBulk(ctx context.Context, keys []HistQueryKey) (map[HistQueryKey][]InnVal, error) {
	if Pool == nil {
		return nil, errors.New("db pool not initialized")
	}
	if len(keys) == 0 {
		return map[HistQueryKey][]InnVal{}, nil
	}
	seenPF := make(map[struct{ P, F int64 }]struct{})
	var maxCutoff time.Time
	for _, k := range keys {
		seenPF[struct{ P, F int64 }{k.P, k.F}] = struct{}{}
		if k.T.After(maxCutoff) {
			maxCutoff = k.T
		}
	}
	var pfPairs []struct{ P, F int64 }
	for pf := range seenPF {
		pfPairs = append(pfPairs, pf)
	}
	sort.Slice(pfPairs, func(i, j int) bool {
		if pfPairs[i].P != pfPairs[j].P {
			return pfPairs[i].P < pfPairs[j].P
		}
		return pfPairs[i].F < pfPairs[j].F
	})
	pids := make([]int64, len(pfPairs))
	fids := make([]int64, len(pfPairs))
	for i, pf := range pfPairs {
		pids[i] = pf.P
		fids[i] = pf.F
	}
	q := `SELECT w.player_id, m.format_id, m.match_date, COALESCE(w.wickets,0)::float8,
		COALESCE(mi.batting_team_opposition_id, 0),
		COALESCE(m.venue_id, 0)
		FROM bowling_data w
		JOIN match_inning mi ON mi.match_id = w.match_id AND mi.inning_number = w.inning_number
		JOIN match m ON m.match_id = w.match_id
		WHERE (w.player_id, m.format_id) IN (SELECT * FROM unnest($1::bigint[], $2::bigint[]))
		AND m.match_date < $3
		ORDER BY w.player_id, m.format_id, m.match_date ASC, w.id ASC`
	rows, err := Pool.Query(ctx, q, pids, fids, maxCutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var all []bowlingBulkRow
	for rows.Next() {
		var r bowlingBulkRow
		if err := rows.Scan(&r.playerID, &r.formatID, &r.matchDate, &r.value, &r.opposition, &r.venue); err != nil {
			return nil, err
		}
		all = append(all, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make(map[HistQueryKey][]InnVal)
	for _, k := range keys {
		var list []InnVal
		for _, r := range all {
			if r.playerID != k.P || r.formatID != k.F || !r.matchDate.Before(k.T) {
				continue
			}
			if k.O != 0 && r.opposition != k.O {
				continue
			}
			if k.V != 0 && r.venue != k.V {
				continue
			}
			list = append(list, InnVal{MatchDate: r.matchDate, Value: r.value})
		}
		out[k] = list
	}
	return out, nil
}

// UpsertFeatureFormSnapshot inserts or updates a consolidated form snapshot row for the given scope.
func UpsertFeatureFormSnapshot(
	ctx context.Context,
	playerID int64,
	asOf time.Time,
	formatID int64,
	scope string, // 'overall' | 'venue' | 'opposition'
	scopeID *int64, // nil when scope == 'overall'
	battingValue float64,
	bowlingValue float64,
	alpha float64,
	nSamplesBat float64,
	nSamplesBowl float64,
	effectiveN float64,
	sourceVersion string,
) error {
	if Pool == nil {
		return errors.New("db pool not initialized")
	}
	_, err := Pool.Exec(
		ctx,
		`INSERT INTO feature_form_snapshots(
			player_id, as_of_date, format_id, scope, scope_id,
			batting_value, bowling_value, alpha, n_samples_bat, n_samples_bowl, effective_n, source_version
		) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		ON CONFLICT (player_id, as_of_date, format_id, scope, scope_id)
		DO UPDATE SET batting_value = EXCLUDED.batting_value,
			bowling_value = EXCLUDED.bowling_value,
			alpha = EXCLUDED.alpha,
			n_samples_bat = EXCLUDED.n_samples_bat,
			n_samples_bowl = EXCLUDED.n_samples_bowl,
			effective_n = EXCLUDED.effective_n,
			source_version = EXCLUDED.source_version`,
		playerID,
		asOf,
		formatID,
		scope,
		scopeID,
		battingValue,
		bowlingValue,
		alpha,
		nSamplesBat,
		nSamplesBowl,
		effectiveN,
		sourceVersion,
	)
	return err
}

// UpsertFeatureConsistencySnapshot inserts or updates a consolidated consistency snapshot row for the given scope.
func UpsertFeatureConsistencySnapshot(
	ctx context.Context,
	playerID int64,
	asOf time.Time,
	formatID int64,
	scope string, // 'overall' | 'venue' | 'opposition'
	scopeID *int64, // nil when scope == 'overall'
	battingValue float64,
	bowlingValue float64,
	windowN int,
	nSamplesBat int,
	nSamplesBowl int,
	sourceVersion string,
) error {
	if Pool == nil {
		return errors.New("db pool not initialized")
	}
	_, err := Pool.Exec(
		ctx,
		`INSERT INTO feature_consistency_snapshots(
			player_id, as_of_date, format_id, scope, scope_id,
			batting_value, bowling_value, window_n, n_samples_bat, n_samples_bowl, source_version
		) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		ON CONFLICT (player_id, as_of_date, format_id, scope, scope_id)
		DO UPDATE SET batting_value = EXCLUDED.batting_value,
			bowling_value = EXCLUDED.bowling_value,
			window_n = EXCLUDED.window_n,
			n_samples_bat = EXCLUDED.n_samples_bat,
			n_samples_bowl = EXCLUDED.n_samples_bowl,
			source_version = EXCLUDED.source_version`,
		playerID,
		asOf,
		formatID,
		scope,
		scopeID,
		battingValue,
		bowlingValue,
		windowN,
		nSamplesBat,
		nSamplesBowl,
		sourceVersion,
	)
	return err
}

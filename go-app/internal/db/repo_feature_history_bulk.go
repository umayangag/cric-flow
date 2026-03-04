package db

import (
	"context"
	"errors"
	"sort"
	"time"
)

// FieldingHistKey identifies a fielding history lookup: PlayerID, Cutoff, FormatID.
type FieldingHistKey struct {
	P int64
	T time.Time
	F int64
}

type fieldingBulkRow struct {
	playerID  int64
	formatID  int64
	matchDate time.Time
	value     float64
}

// ListFieldingBeforeBulk fetches fielding involvements for multiple (playerID, cutoff, formatID) keys in one query.
func ListFieldingBeforeBulk(ctx context.Context, keys []FieldingHistKey) (map[FieldingHistKey][]InnVal, error) {
	if Pool == nil {
		return nil, errors.New("db pool not initialized")
	}
	if len(keys) == 0 {
		return map[FieldingHistKey][]InnVal{}, nil
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
	rows, err := Pool.Query(ctx, `SELECT fd.player_id, m.format_id, m.match_date,
		(COALESCE(fd.catches,0) + COALESCE(fd.run_outs,0)*1.5 + COALESCE(fd.stumpings,0))::float8
		FROM fielding_data fd
		JOIN match m ON m.match_id = fd.match_id
		WHERE (fd.player_id, m.format_id) IN (SELECT * FROM unnest($1::bigint[], $2::bigint[]))
		AND m.match_date < $3
		ORDER BY fd.player_id, m.format_id, m.match_date ASC`, pids, fids, maxCutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var all []fieldingBulkRow
	for rows.Next() {
		var r fieldingBulkRow
		if err := rows.Scan(&r.playerID, &r.formatID, &r.matchDate, &r.value); err != nil {
			return nil, err
		}
		all = append(all, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make(map[FieldingHistKey][]InnVal)
	for _, k := range keys {
		var list []InnVal
		for _, r := range all {
			if r.playerID != k.P || r.formatID != k.F || !r.matchDate.Before(k.T) {
				continue
			}
			list = append(list, InnVal{MatchDate: r.matchDate, Value: r.value})
		}
		out[k] = list
	}
	return out, nil
}

// HistQueryKey identifies a history lookup for bulk fetch: PlayerID, Cutoff, FormatID, OppID (0=overall), VenueID (0=overall).
type HistQueryKey struct {
	P int64
	T time.Time
	F int64
	O int64
	V int64
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

package exportqueries

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// FieldingTrainingRows returns fielding export-shaped rows for all matches with match_date < cutoff.
// Used by the backtest training-data API. Form/consistency are computed on the fly from ListFieldingBeforeBulk.
func FieldingTrainingRows(ctx context.Context, cutoff time.Time) ([][]string, error) {
	return fieldingTrainingRowsImpl(ctx, cutoff, nil)
}

// FieldingTrainingRowsWithFormat returns fielding rows filtered by format code (T20, ODI, etc.).
func FieldingTrainingRowsWithFormat(ctx context.Context, format string, cutoff time.Time) ([][]string, error) {
	formatIDs, err := db.GetGlobalCache().GetFormatIDsForTrainingBucket(ctx, format)
	if err != nil {
		return nil, err
	}
	return fieldingTrainingRowsImpl(ctx, cutoff, formatIDs)
}

func fieldingTrainingRowsRawQuery(formatIDs []int64, cutoff time.Time) (q string, args []any) {
	// Opposition for fielding = batting team in the inning the player fielded in.
	// Weather: first innings aligns with batting session, second with bowling session.
	q = `SELECT
		m.match_date,
		fd.player_id,
		m.format_id,
		COALESCE(m.venue_id, 0),
		COALESCE(mi.batting_team_opposition_id, 0),
		fd.inning_number AS inning,
		CASE WHEN m.toss_decision IS NULL THEN 0 WHEN lower(m.toss_decision) LIKE '%bat%' THEN 1 ELSE 0 END AS toss,
		p.player_name,
		COALESCE(fd.catches,0), COALESCE(fd.run_outs,0), COALESCE(fd.stumpings,0),
		COALESCE(mf.code, '')
	FROM fielding_data fd
	LEFT JOIN player p ON fd.player_id = p.id
	LEFT JOIN match m ON m.match_id = fd.match_id
	LEFT JOIN match_format mf ON mf.id = m.format_id
	LEFT JOIN match_inning mi ON mi.match_id = fd.match_id AND mi.inning_number = fd.inning_number
	WHERE m.match_date < $1
	ORDER BY m.match_date ASC, fd.match_id, fd.inning_number, fd.player_id`
	args = []any{cutoff}
	if formatIDs != nil {
		q = strings.Replace(
			q,
			"WHERE m.match_date < $1",
			"WHERE m.format_id = ANY($1::bigint[]) AND m.match_date < $2",
			1,
		)
		args = []any{formatIDs, cutoff}
	}
	return q, args
}

type fieldingTrainingRowRaw struct {
	matchDate    time.Time
	playerID     int64
	formatID     int64
	venueID      int64
	oppositionID int64
	inning       string
	toss         string
	playerName   string
	catches      string
	runOuts      string
	stumpings    string
	formatCode   string
}

func fieldingTrainingRowsImpl(ctx context.Context, cutoff time.Time, formatIDs []int64) ([][]string, error) {
	q, args := fieldingTrainingRowsRawQuery(formatIDs, cutoff)
	rows, err := db.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var rawRows []fieldingTrainingRowRaw
	for rows.Next() {
		var r fieldingTrainingRowRaw
		err := rows.Scan(
			&r.matchDate, &r.playerID, &r.formatID, &r.venueID, &r.oppositionID,
			&r.inning, &r.toss, &r.playerName,
			&r.catches, &r.runOuts, &r.stumpings,
			&r.formatCode,
		)
		if err != nil {
			return nil, err
		}
		rawRows = append(rawRows, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	keys := make([]db.FieldingHistKey, 0, len(rawRows))
	seen := make(map[db.FieldingHistKey]struct{})
	for _, r := range rawRows {
		k := db.FieldingHistKey{P: r.playerID, T: r.matchDate, F: r.formatID}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		keys = append(keys, k)
	}
	bulkRes, err := db.ListFieldingBeforeBulk(ctx, keys)
	if err != nil {
		return nil, err
	}
	alpha, lastN, windowN, _, _, _ := GetFeatureExtractionParams()
	headers := FieldingTrainingHeaders()
	out := make([][]string, 0, len(rawRows)+1)
	out = append(out, headers)
	for _, r := range rawRows {
		k := db.FieldingHistKey{P: r.playerID, T: r.matchDate, F: r.formatID}
		mainHist := bulkRes[k]
		snap := computeFieldingSnapshotFromHistories(mainHist, r.matchDate, alpha, lastN, windowN)
		// Venue/opposition: no scope in fielding history; use 0 for now so model can still use context.
		venueStr := "0"
		oppStr := "0"
		if r.venueID != 0 {
			venueStr = strconv.FormatInt(r.venueID, 10)
		}
		if r.oppositionID != 0 {
			oppStr = strconv.FormatInt(r.oppositionID, 10)
		}
		row := []string{
			r.catches, r.runOuts, r.stumpings,
			floatToExport(snap.consistency), floatToExport(snap.form),
			r.inning, r.toss, venueStr, oppStr, cyclicalMonthSin(r.matchDate), cyclicalMonthCos(r.matchDate), cyclicalDowSin(r.matchDate), cyclicalDowCos(r.matchDate), r.playerName,
			strings.TrimSpace(strings.ToUpper(r.formatCode)),
			r.matchDate.Format("2006-01-02"),
		}
		out = append(out, row)
	}
	return out, nil
}

// fieldingHoldoutRawQuery returns SQL and args for raw fielding rows for the given match IDs.
func fieldingHoldoutRawQuery(matchIDs []int64) (string, []any) {
	q := `SELECT
		m.match_date,
		fd.player_id,
		m.format_id,
		COALESCE(m.venue_id, 0),
		COALESCE(mi.batting_team_opposition_id, 0),
		fd.inning_number AS inning,
		CASE WHEN m.toss_decision IS NULL THEN 0 WHEN lower(m.toss_decision) LIKE '%bat%' THEN 1 ELSE 0 END AS toss,
		p.player_name,
		COALESCE(fd.catches,0), COALESCE(fd.run_outs,0), COALESCE(fd.stumpings,0),
		COALESCE(mf.code, '')
	FROM fielding_data fd
	LEFT JOIN player p ON fd.player_id = p.id
	LEFT JOIN match m ON m.match_id = fd.match_id
	LEFT JOIN match_format mf ON mf.id = m.format_id
	LEFT JOIN match_inning mi ON mi.match_id = fd.match_id AND mi.inning_number = fd.inning_number
	WHERE m.match_id = ANY($1::bigint[])`
	return q, []any{matchIDs}
}

// fieldingHoldoutRowsImpl returns fielding export-shaped rows for the given match IDs with features at cutoff.
func fieldingHoldoutRowsImpl(ctx context.Context, matchIDs []int64, cutoff time.Time) ([][]string, error) {
	if len(matchIDs) == 0 {
		headers := FieldingHoldoutHeaders()
		return [][]string{headers}, nil
	}
	q, args := fieldingHoldoutRawQuery(matchIDs)
	rows, err := db.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var rawRows []fieldingTrainingRowRaw
	for rows.Next() {
		var r fieldingTrainingRowRaw
		if err := rows.Scan(
			&r.matchDate, &r.playerID, &r.formatID, &r.venueID, &r.oppositionID,
			&r.inning, &r.toss, &r.playerName,
			&r.catches, &r.runOuts, &r.stumpings,
			&r.formatCode,
		); err != nil {
			return nil, err
		}
		rawRows = append(rawRows, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	keys := make([]db.FieldingHistKey, 0, len(rawRows))
	seen := make(map[db.FieldingHistKey]struct{})
	for _, r := range rawRows {
		k := db.FieldingHistKey{P: r.playerID, T: cutoff, F: r.formatID}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		keys = append(keys, k)
	}
	bulkRes, err := db.ListFieldingBeforeBulk(ctx, keys)
	if err != nil {
		return nil, err
	}
	alpha, lastN, windowN, _, _, _ := GetFeatureExtractionParams()
	headers := FieldingHoldoutHeaders()
	out := make([][]string, 0, len(rawRows)+1)
	out = append(out, headers)
	for _, r := range rawRows {
		k := db.FieldingHistKey{P: r.playerID, T: cutoff, F: r.formatID}
		mainHist := bulkRes[k]
		snap := computeFieldingSnapshotFromHistories(mainHist, cutoff, alpha, lastN, windowN)
		venueStr := "0"
		oppStr := "0"
		if r.venueID != 0 {
			venueStr = strconv.FormatInt(r.venueID, 10)
		}
		if r.oppositionID != 0 {
			oppStr = strconv.FormatInt(r.oppositionID, 10)
		}
		row := []string{
			r.catches, r.runOuts, r.stumpings,
			floatToExport(snap.consistency), floatToExport(snap.form),
			r.inning, r.toss, venueStr, oppStr, cyclicalMonthSin(r.matchDate), cyclicalMonthCos(r.matchDate), cyclicalDowSin(r.matchDate), cyclicalDowCos(r.matchDate), r.playerName,
			strings.TrimSpace(strings.ToUpper(r.formatCode)),
			r.matchDate.Format("2006-01-02"),
		}
		out = append(out, row)
	}
	return out, nil
}

// FieldingHoldoutRows returns fielding export-shaped rows for the next limit matches after cutoff (walk-forward holdout).
func FieldingHoldoutRows(ctx context.Context, format string, cutoff time.Time, limit int) ([][]string, error) {
	formatIDs, err := db.GetGlobalCache().GetFormatIDsForTrainingBucket(ctx, format)
	if err != nil {
		return nil, err
	}
	matchList, err := db.ListMatchIDsAfter(ctx, formatIDs, cutoff, limit)
	if err != nil {
		return nil, err
	}
	matchIDs := make([]int64, 0, len(matchList))
	for _, it := range matchList {
		matchIDs = append(matchIDs, it.MatchID)
	}
	return fieldingHoldoutRowsImpl(ctx, matchIDs, cutoff)
}

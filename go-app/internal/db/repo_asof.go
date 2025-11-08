package db

import (
	"context"
	"errors"
	"time"
)

// MatchLite is a minimal projection to iterate matches chronologically.
type MatchLite struct {
	ID           int64
	MatchID      int64
	Date         time.Time
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
		  JOIN match_details md ON md.match_id = b.match_id
		  WHERE md.format_id = $1 AND md.date < $2
		  UNION
		  SELECT w.player_id
		  FROM bowling_data w
		  JOIN match_details md ON md.match_id = w.match_id
		  WHERE md.format_id = $1 AND md.date < $2
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
	q := `SELECT id, match_id, date, format_id, COALESCE(opposition_id,0), COALESCE(venue_id,0)
		FROM match_details
		WHERE format_id = $1`
	args := []any{formatID}
	if from != nil {
		q += ` AND date >= $2`
		args = append(args, *from)
	}
	if to != nil {
		if len(args) == 1 {
			q += ` AND date <= $2`
		} else {
			q += ` AND date <= $3`
		}
		args = append(args, *to)
	}
	q += ` ORDER BY date ASC, id ASC`
	rows, err := Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []MatchLite
	for rows.Next() {
		var m MatchLite
		if err := rows.Scan(&m.ID, &m.MatchID, &m.Date, &m.FormatID, &m.OppositionID, &m.VenueID); err != nil {
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
	Date  time.Time
	Value float64
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
	q := `SELECT md.date, COALESCE(b.runs,0)::float8
		FROM batting_data b
		JOIN match_details md ON md.match_id = b.match_id
		WHERE b.player_id = $1 AND md.date < $2 AND md.format_id = $3`
	args := []any{playerID, cutoff, formatID}
	if oppID != nil {
		q += ` AND md.opposition_id = $4`
		args = append(args, *oppID)
	}
	if venueID != nil {
		if oppID != nil {
			q += ` AND md.venue_id = $5`
		} else {
			q += ` AND md.venue_id = $4`
		}
		args = append(args, *venueID)
	}
	q += ` ORDER BY md.date ASC, b.id ASC`
	rows, err := Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []InnVal
	for rows.Next() {
		var iv InnVal
		if err := rows.Scan(&iv.Date, &iv.Value); err != nil {
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
	q := `SELECT md.date, COALESCE(w.wickets,0)::float8
		FROM bowling_data w
		JOIN match_details md ON md.match_id = w.match_id
		WHERE w.player_id = $1 AND md.date < $2 AND md.format_id = $3`
	args := []any{playerID, cutoff, formatID}
	if oppID != nil {
		q += ` AND md.opposition_id = $4`
		args = append(args, *oppID)
	}
	if venueID != nil {
		if oppID != nil {
			q += ` AND md.venue_id = $5`
		} else {
			q += ` AND md.venue_id = $4`
		}
		args = append(args, *venueID)
	}
	q += ` ORDER BY md.date ASC, w.id ASC`
	rows, err := Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []InnVal
	for rows.Next() {
		var iv InnVal
		if err := rows.Scan(&iv.Date, &iv.Value); err != nil {
			return nil, err
		}
		res = append(res, iv)
	}
	return res, rows.Err()
}

// UpsertPlayerFormAsOf inserts or updates a form snapshot.
func UpsertPlayerFormAsOf(ctx context.Context, playerID int64, asOf time.Time, formatID int64,
	batForm, bowlForm, nBat, nBowl float64, windowSpec string,
) error {
	if Pool == nil {
		return errors.New("db pool not initialized")
	}
	// Backward compatible no-op: write into the new consolidated table as overall scope
	scope := "overall"
	var scopeID *int64
	_, err := Pool.Exec(
		ctx,
		`INSERT INTO feature_form_snapshots(
			player_id, as_of_date, format_id, scope, scope_id,
			batting_value, bowling_value, alpha, n_samples_bat, n_samples_bowl, effective_n, source_version
		) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,'v1')
		ON CONFLICT (player_id, as_of_date, format_id, scope, scope_id)
		DO UPDATE SET batting_value = EXCLUDED.batting_value,
			bowling_value = EXCLUDED.bowling_value,
			alpha = EXCLUDED.alpha,
			n_samples_bat = EXCLUDED.n_samples_bat,
			n_samples_bowl = EXCLUDED.n_samples_bowl,
			effective_n = EXCLUDED.effective_n`,
		playerID,
		asOf,
		formatID,
		scope,
		scopeID,
		batForm,
		bowlForm,
		// Derive alpha from windowSpec if possible later; for now use 0 as placeholder when unknown
		0.0,
		nBat,
		nBowl,
		(nBat + nBowl),
	)
	return err
}

// UpsertPlayerConsistencyAsOf inserts or updates a consistency snapshot.
func UpsertPlayerConsistencyAsOf(ctx context.Context, playerID int64, asOf time.Time, formatID int64,
	batCons, bowlCons float64, nBat, nBowl int, windowSpec string,
) error {
	if Pool == nil {
		return errors.New("db pool not initialized")
	}
	_, err := Pool.Exec(
		ctx,
		`INSERT INTO player_consistency_asof(player_id, as_of_date, format_id, bat_consistency, bowl_consistency, n_samples_bat, n_samples_bowl, window_spec)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (player_id, as_of_date, format_id)
		DO UPDATE SET bat_consistency = EXCLUDED.bat_consistency,
			bowl_consistency = EXCLUDED.bowl_consistency,
			n_samples_bat = EXCLUDED.n_samples_bat,
			n_samples_bowl = EXCLUDED.n_samples_bowl,
			window_spec = EXCLUDED.window_spec`,
		playerID,
		asOf,
		formatID,
		batCons,
		bowlCons,
		nBat,
		nBowl,
		windowSpec,
	)
	return err
}

// UpsertPlayerVsOppAsOf inserts or updates a vs-opposition snapshot.
func UpsertPlayerVsOppAsOf(ctx context.Context, playerID int64, oppositionID int64, asOf time.Time, formatID int64,
	batValue, bowlValue float64, nSamples int, windowSpec string,
) error {
	if Pool == nil {
		return errors.New("db pool not initialized")
	}
	_, err := Pool.Exec(
		ctx,
		`INSERT INTO player_vs_opposition_asof(player_id, opposition_id, as_of_date, format_id, bat_value, bowl_value, n_samples, window_spec)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (player_id, opposition_id, as_of_date, format_id)
		DO UPDATE SET bat_value = EXCLUDED.bat_value,
			bowl_value = EXCLUDED.bowl_value,
			n_samples = EXCLUDED.n_samples,
			window_spec = EXCLUDED.window_spec`,
		playerID,
		oppositionID,
		asOf,
		formatID,
		batValue,
		bowlValue,
		nSamples,
		windowSpec,
	)
	return err
}

// UpsertPlayerAtVenueAsOf inserts or updates an at-venue snapshot.
func UpsertPlayerAtVenueAsOf(ctx context.Context, playerID int64, venueID int64, asOf time.Time, formatID int64,
	batValue, bowlValue float64, nSamples int, windowSpec string,
) error {
	if Pool == nil {
		return errors.New("db pool not initialized")
	}
	_, err := Pool.Exec(
		ctx,
		`INSERT INTO player_at_venue_asof(player_id, venue_id, as_of_date, format_id, bat_value, bowl_value, n_samples, window_spec)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (player_id, venue_id, as_of_date, format_id)
		DO UPDATE SET bat_value = EXCLUDED.bat_value,
			bowl_value = EXCLUDED.bowl_value,
			n_samples = EXCLUDED.n_samples,
			window_spec = EXCLUDED.window_spec`,
		playerID,
		venueID,
		asOf,
		formatID,
		batValue,
		bowlValue,
		nSamples,
		windowSpec,
	)
	return err
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

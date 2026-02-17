package db

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// MatchContext carries the minimal fields needed for feature building.
type MatchContext struct {
	Inning       sql.NullInt64
	Session      sql.NullInt64
	Toss         sql.NullInt64 // already encoded numeric in our exporters; default 0 if NULL
	VenueID      sql.NullInt64
	OppositionID sql.NullInt64
	SeasonID     sql.NullInt64
	MatchNumber  sql.NullInt64
}

// GetMatchContext fetches match context from match + first match_inning.
func GetMatchContext(ctx context.Context, matchID int64) (*MatchContext, error) {
	if Pool == nil {
		return nil, errors.New("db pool not initialized")
	}
	mc := &MatchContext{}
	err := Pool.QueryRow(ctx, `
		SELECT 
			mi.inning_number,
			0 AS session,
			CASE WHEN m.toss_decision IS NULL THEN 0 WHEN lower(m.toss_decision) = 'bat' THEN 1 ELSE 0 END,
			m.venue_id, mi.bowling_team_opposition_id, m.season_id, m.match_number
		FROM match m
		LEFT JOIN LATERAL (
			SELECT inning_number, bowling_team_opposition_id
			FROM match_inning WHERE match_id = m.match_id ORDER BY inning_number LIMIT 1
		) mi ON true
		WHERE m.match_id = $1
	`, matchID).Scan(&mc.Inning, &mc.Session, &mc.Toss, &mc.VenueID, &mc.OppositionID, &mc.SeasonID, &mc.MatchNumber)
	if err != nil {
		return nil, err
	}
	return mc, nil
}

// PlayerPoolRow represents a candidate player with consistency and flags.
type PlayerPoolRow struct {
	PlayerID           int64
	PlayerName         string
	IsWicketKeeper     int16
	BattingConsistency sql.NullFloat64
	BowlingConsistency sql.NullFloat64
}

// ListPlayerPoolConsistency returns non-retired players with any non-zero consistency for the format.
// Uses feature_consistency_snapshots (latest snapshot per player/format); seasonName is ignored.
func ListPlayerPoolConsistency(ctx context.Context, _ string, formatCode string) ([]PlayerPoolRow, error) {
	if Pool == nil {
		return nil, errors.New("db pool not initialized")
	}
	fid, err := GetOrCreateMatchFormat(ctx, formatCode)
	if err != nil {
		return nil, err
	}
	rows, err := Pool.Query(ctx, `
		SELECT p.id, p.player_name, p.is_wicket_keeper,
		       COALESCE(latest.batting_value,0)::real AS batting_consistency,
		       COALESCE(latest.bowling_value,0)::real AS bowling_consistency
		FROM player p
		LEFT JOIN LATERAL (
			SELECT batting_value, bowling_value
			FROM feature_consistency_snapshots
			WHERE player_id = p.id AND format_id = $1 AND scope = 'overall' AND scope_id IS NULL
			ORDER BY as_of_date DESC
			LIMIT 1
		) latest ON true
		WHERE p.is_retired = 0 AND (COALESCE(latest.batting_value,0) != 0 OR COALESCE(latest.bowling_value,0) != 0)
	`, fid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PlayerPoolRow
	for rows.Next() {
		var r PlayerPoolRow
		if err := rows.Scan(&r.PlayerID, &r.PlayerName, &r.IsWicketKeeper, &r.BattingConsistency, &r.BowlingConsistency); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetPlayerFormFmt returns batting and bowling form for player/format from feature_form_snapshots.
// Uses the latest snapshot (scope=overall); seasonID is ignored.
func GetPlayerFormFmt(ctx context.Context, playerID, _ int64, formatID int64) (bat float64, bowl float64, err error) {
	if Pool == nil {
		return 0, 0, errors.New("db pool not initialized")
	}
	var b, w sql.NullFloat64
	err = Pool.QueryRow(ctx, `
		SELECT batting_value, bowling_value
		FROM feature_form_snapshots
		WHERE player_id = $1 AND format_id = $2 AND scope = 'overall' AND scope_id IS NULL
		ORDER BY as_of_date DESC
		LIMIT 1
	`, playerID, formatID).Scan(&b, &w)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, 0, nil
		}
		return 0, 0, err
	}
	if b.Valid {
		bat = b.Float64
	}
	if w.Valid {
		bowl = w.Float64
	}
	return bat, bowl, nil
}

// GetPlayerVenueEffectFmt returns batting and bowling venue effect from feature_form_snapshots (scope=venue).
func GetPlayerVenueEffectFmt(
	ctx context.Context,
	playerID, venueID, formatID int64,
) (bat float64, bowl float64, err error) {
	if Pool == nil {
		return 0, 0, errors.New("db pool not initialized")
	}
	var b, w sql.NullFloat64
	err = Pool.QueryRow(ctx, `
		SELECT batting_value, bowling_value
		FROM feature_form_snapshots
		WHERE player_id = $1 AND format_id = $2 AND scope = 'venue' AND scope_id = $3
		ORDER BY as_of_date DESC
		LIMIT 1
	`, playerID, formatID, venueID).Scan(&b, &w)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, 0, nil
		}
		return 0, 0, err
	}
	if b.Valid {
		bat = b.Float64
	}
	if w.Valid {
		bowl = w.Float64
	}
	return bat, bowl, nil
}

// ListPlayerPoolByTeam returns players who have played for the given team (opposition name) in the format,
// with at least one batting or bowling record before the cutoff date. Uses feature snapshots for form/consistency
// when available. Optionally include extra player IDs (e.g. IPL auction players with no prior team history).
func ListPlayerPoolByTeam(
	ctx context.Context,
	formatCode string,
	teamName string,
	cutoff time.Time,
	extraPlayerIDs []int64,
) ([]PlayerPoolRow, error) {
	if Pool == nil {
		return nil, errors.New("db pool not initialized")
	}
	formatID, err := GetOrCreateMatchFormat(ctx, formatCode)
	if err != nil {
		return nil, err
	}
	oppID, err := GetOrCreateOpposition(ctx, teamName)
	if err != nil {
		return nil, err
	}
	cutoffDate := cutoff.Truncate(24 * time.Hour)

	// Players who have batted or bowled for this team (opposition) in matches before cutoff.
	// Join with feature_consistency_snapshots for latest consistency at or before cutoff so teamselect can identify bowlers.
	rows, err := Pool.Query(ctx, `
		SELECT DISTINCT p.id, p.player_name, p.is_wicket_keeper,
		       COALESCE(latest.batting_value, 0)::real AS batting_consistency,
		       COALESCE(latest.bowling_value, 0)::real AS bowling_consistency
		FROM player p
		LEFT JOIN LATERAL (
			SELECT batting_value, bowling_value
			FROM feature_consistency_snapshots
			WHERE player_id = p.id AND format_id = $1 AND scope = 'overall' AND scope_id IS NULL
			  AND as_of_date <= $2
			ORDER BY as_of_date DESC
			LIMIT 1
		) latest ON true
		WHERE p.is_retired = 0
		  AND (
		    EXISTS (
		      SELECT 1 FROM batting_data bd
		      JOIN match_inning mi ON mi.match_id = bd.match_id AND mi.inning_number = bd.inning_number
		      JOIN match m ON m.match_id = bd.match_id
		      WHERE bd.player_id = p.id AND m.format_id = $1 AND m.match_date < $2
		        AND mi.batting_team_opposition_id = $3
		    )
		    OR EXISTS (
		      SELECT 1 FROM bowling_data bw
		      JOIN match_inning mi ON mi.match_id = bw.match_id AND mi.inning_number = bw.inning_number
		      JOIN match m ON m.match_id = bw.match_id
		      WHERE bw.player_id = p.id AND m.format_id = $1 AND m.match_date < $2
		        AND mi.bowling_team_opposition_id = $3
		    )
		  )
		ORDER BY p.player_name
	`, formatID, cutoffDate, oppID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	seen := make(map[int64]bool)
	var out []PlayerPoolRow
	for rows.Next() {
		var r PlayerPoolRow
		if err := rows.Scan(&r.PlayerID, &r.PlayerName, &r.IsWicketKeeper, &r.BattingConsistency, &r.BowlingConsistency); err != nil {
			return nil, err
		}
		if seen[r.PlayerID] {
			continue
		}
		seen[r.PlayerID] = true
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Add extra player IDs (e.g. new auction players) if not already in pool
	for _, pid := range extraPlayerIDs {
		if seen[pid] {
			continue
		}
		var name string
		var keeper int16
		if err := Pool.QueryRow(ctx, `SELECT player_name, is_wicket_keeper FROM player WHERE id = $1`, pid).Scan(&name, &keeper); err != nil {
			if err == sql.ErrNoRows {
				continue
			}
			return nil, err
		}
		seen[pid] = true
		out = append(out, PlayerPoolRow{
			PlayerID:           pid,
			PlayerName:         name,
			IsWicketKeeper:     keeper,
			BattingConsistency: sql.NullFloat64{},
			BowlingConsistency: sql.NullFloat64{},
		})
	}

	return out, nil
}

// GetPlayerOppositionEffectFmt returns batting and bowling opposition effect from feature_form_snapshots (scope=opposition).
func GetPlayerOppositionEffectFmt(
	ctx context.Context,
	playerID, oppositionID, formatID int64,
) (bat float64, bowl float64, err error) {
	if Pool == nil {
		return 0, 0, errors.New("db pool not initialized")
	}
	var b, w sql.NullFloat64
	err = Pool.QueryRow(ctx, `
		SELECT batting_value, bowling_value
		FROM feature_form_snapshots
		WHERE player_id = $1 AND format_id = $2 AND scope = 'opposition' AND scope_id = $3
		ORDER BY as_of_date DESC
		LIMIT 1
	`, playerID, formatID, oppositionID).Scan(&b, &w)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, 0, nil
		}
		return 0, 0, err
	}
	if b.Valid {
		bat = b.Float64
	}
	if w.Valid {
		bowl = w.Float64
	}
	return bat, bowl, nil
}

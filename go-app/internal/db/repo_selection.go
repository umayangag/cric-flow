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
	PlayerID int64
	// ExternalID is the Cricsheet registry id, and it is what the ML service knows a player
	// by: the rating state has been keyed on it since P-1, on both sources. The database id
	// is this repo's own and means nothing to ml-service.
	ExternalID         string
	PlayerName         string
	IsWicketKeeper     int16
	BattingConsistency sql.NullFloat64
	BowlingConsistency sql.NullFloat64
}

// ListPlayerPoolConsistency returns non-retired players with any non-zero consistency for the format.
// Uses feature_raw_stats_snapshots (std_w10 as consistency; latest snapshot per player/format); seasonName is ignored.
func ListPlayerPoolConsistency(ctx context.Context, _ string, formatCode string) ([]PlayerPoolRow, error) {
	if Pool == nil {
		return nil, errors.New("db pool not initialized")
	}
	fid, err := GetOrCreateMatchFormat(ctx, formatCode)
	if err != nil {
		return nil, err
	}
	rows, err := Pool.Query(ctx, `
		SELECT p.id, COALESCE(p.external_id, ''), p.player_name, p.is_wicket_keeper,
		       COALESCE(latest.batting_std_w10,0)::real AS batting_consistency,
		       COALESCE(latest.bowling_std_w10,0)::real AS bowling_consistency
		FROM player p
		LEFT JOIN LATERAL (
			SELECT batting_std_w10, bowling_std_w10
			FROM feature_raw_stats_snapshots
			WHERE player_id = p.id AND format_id = $1 AND scope = 'overall' AND scope_id IS NULL
			ORDER BY as_of_date DESC
			LIMIT 1
		) latest ON true
		WHERE p.is_retired = 0 AND (COALESCE(latest.batting_std_w10,0) != 0 OR COALESCE(latest.bowling_std_w10,0) != 0)
	`, fid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PlayerPoolRow
	for rows.Next() {
		var r PlayerPoolRow
		if err := rows.Scan(&r.PlayerID, &r.ExternalID, &r.PlayerName, &r.IsWicketKeeper,
			&r.BattingConsistency, &r.BowlingConsistency); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListPlayerPoolByOpposition returns players who have played for the given team in the format,
// with at least one batting or bowling record before the cutoff date. Uses feature snapshots for form/consistency
// when available. Optionally include extra player IDs (e.g. IPL auction players with no prior team history).
//
// It takes an opposition id rather than a team name because a name is no longer one
// team: 130 of the 394 names in the dataset belong to both a men's and a women's side.
// Resolving the name is the caller's job (FindOppositionIDForFormat), which also stops
// this read path from creating a team row as a side effect of a prediction request.
//
// The id is a *club*: the pool spans every name the club has played under.
func ListPlayerPoolByOpposition(
	ctx context.Context,
	formatCode string,
	oppID int64,
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
	cutoffDate := cutoff.Truncate(24 * time.Hour)

	// Players who have batted or bowled for this team (opposition) in matches before cutoff.
	// batting_team_opposition_id = team that batted (batters in batting_data play for that team);
	// bowling_team_opposition_id = team that bowled (bowlers in bowling_data play for that team).
	// Use a CTE to get distinct player_ids via JOINs (avoids EXISTS per-row); then join with player and consistency.
	rows, err := Pool.Query(ctx, `
		WITH club AS (
		  -- Every opposition row belonging to the same club, so a rename does not halve
		  -- the pool: Royal Challengers Bengaluru's players include the ones who only ever
		  -- appear under Bangalore.
		  SELECT id FROM opposition WHERE COALESCE(canonical_id, id) = $3
		), eligible AS (
		  SELECT bd.player_id AS id
		  FROM batting_data bd
		  JOIN match_inning mi ON mi.match_id = bd.match_id AND mi.inning_number = bd.inning_number
		  JOIN match m ON m.match_id = bd.match_id
		  WHERE m.format_id = $1 AND m.match_date < $2
		    AND mi.batting_team_opposition_id IN (SELECT id FROM club)
		  UNION
		  SELECT bw.player_id
		  FROM bowling_data bw
		  JOIN match_inning mi ON mi.match_id = bw.match_id AND mi.inning_number = bw.inning_number
		  JOIN match m ON m.match_id = bw.match_id
		  WHERE m.format_id = $1 AND m.match_date < $2
		    AND mi.bowling_team_opposition_id IN (SELECT id FROM club)
		)
		SELECT p.id, COALESCE(p.external_id, ''), p.player_name, p.is_wicket_keeper,
		       COALESCE(latest.batting_std_w10, 0)::real AS batting_consistency,
		       COALESCE(latest.bowling_std_w10, 0)::real AS bowling_consistency
		FROM player p
		JOIN eligible e ON e.id = p.id
		LEFT JOIN LATERAL (
			SELECT batting_std_w10, bowling_std_w10
			FROM feature_raw_stats_snapshots
			WHERE player_id = p.id AND format_id = $1 AND scope = 'overall' AND scope_id IS NULL
			  AND as_of_date <= $2
			ORDER BY as_of_date DESC
			LIMIT 1
		) latest ON true
		WHERE p.is_retired = 0
		ORDER BY p.player_name, p.id
	`, formatID, cutoffDate, oppID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	seen := make(map[int64]bool)
	var out []PlayerPoolRow
	for rows.Next() {
		var r PlayerPoolRow
		if err := rows.Scan(&r.PlayerID, &r.ExternalID, &r.PlayerName, &r.IsWicketKeeper,
			&r.BattingConsistency, &r.BowlingConsistency); err != nil {
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
		var name, externalID string
		var keeper int16
		if err := Pool.QueryRow(ctx,
			`SELECT COALESCE(external_id, ''), player_name, is_wicket_keeper FROM player WHERE id = $1`,
			pid).Scan(&externalID, &name, &keeper); err != nil {
			if err == sql.ErrNoRows {
				continue
			}
			return nil, err
		}
		seen[pid] = true
		out = append(out, PlayerPoolRow{
			PlayerID:           pid,
			ExternalID:         externalID,
			PlayerName:         name,
			IsWicketKeeper:     keeper,
			BattingConsistency: sql.NullFloat64{},
			BowlingConsistency: sql.NullFloat64{},
		})
	}

	return out, nil
}

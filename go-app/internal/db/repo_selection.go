package db

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// PlayerPoolRow represents a candidate player with consistency and flags.
type PlayerPoolRow struct {
	PlayerID int64
	// ExternalID is the Cricsheet registry id, and it is what the ML service knows a player
	// by: the rating state has been keyed on it since P-1, on both sources. The database id
	// is this repo's own and means nothing to ml-service.
	ExternalID     string
	PlayerName     string
	IsWicketKeeper int16
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
	// A CTE gets the distinct player_ids via JOINs, avoiding EXISTS per row.
	//
	// The pool is who *may* play, and nothing more: it used to carry each player's windowed
	// consistency from `feature_raw_stats_snapshots`, for the score weights P-5 deleted. P-6
	// dropped that table, and the rating state ml-service holds is where a player's form
	// lives now -- go-app sends ids and lets the model read them.
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
		SELECT p.id, COALESCE(p.external_id, ''), p.player_name, p.is_wicket_keeper
		FROM player p
		JOIN eligible e ON e.id = p.id
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
		if err := rows.Scan(&r.PlayerID, &r.ExternalID, &r.PlayerName, &r.IsWicketKeeper); err != nil {
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
			PlayerID:       pid,
			ExternalID:     externalID,
			PlayerName:     name,
			IsWicketKeeper: keeper,
		})
	}

	return out, nil
}

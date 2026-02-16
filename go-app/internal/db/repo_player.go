package db

import (
	"context"
	"errors"
)

// Player represents the player table row
type Player struct {
	ID             int64
	Name           string
	IsWicketKeeper int16
	IsRetired      int16
}

// PlayerConsistency holds consistency values for a player/format (from feature_consistency_snapshots or legacy).
type PlayerConsistency struct {
	PlayerID           int64
	SeasonID           int64
	FormatID           int64
	BattingConsistency float32
	BowlingConsistency float32
}

// GetPlayerByID returns a player by their ID.
func GetPlayerByID(ctx context.Context, id int64) (*Player, error) {
	if Pool == nil {
		return nil, errors.New("db pool not initialized")
	}
	row := QueryRow(
		ctx,
		`SELECT id, player_name, is_wicket_keeper, is_retired FROM player WHERE id = $1`,
		id,
	)
	p := &Player{}
	if err := row.Scan(&p.ID, &p.Name, &p.IsWicketKeeper, &p.IsRetired); err != nil {
		return nil, err
	}
	return p, nil
}

// GetPlayerConsistency returns a player”'s consistency data for a given season and format.
func GetPlayerConsistency(
	ctx context.Context,
	playerID int64,
	seasonName, formatCode string,
) (*PlayerConsistency, error) {
	if Pool == nil {
		return nil, errors.New("db pool not initialized")
	}

	sid, err := GetOrCreateSeason(ctx, seasonName)
	if err != nil {
		return nil, err
	}

	fid, err := GetOrCreateMatchFormat(ctx, formatCode)
	if err != nil {
		return nil, err
	}

	// Read from feature_consistency_snapshots (latest per player/format); seasonID kept for API compatibility.
	row := Pool.QueryRow(ctx, `
		SELECT $1::bigint, $2::bigint, $3::bigint, batting_value::real, bowling_value::real
		FROM feature_consistency_snapshots
		WHERE player_id = $1 AND format_id = $3 AND scope = 'overall' AND scope_id IS NULL
		ORDER BY as_of_date DESC
		LIMIT 1
	`, playerID, sid, fid)

	pc := &PlayerConsistency{}
	if err := row.Scan(&pc.PlayerID, &pc.SeasonID, &pc.FormatID, &pc.BattingConsistency, &pc.BowlingConsistency); err != nil {
		return nil, err
	}
	return pc, nil
}

// GetByName returns a player by exact name match.
func GetByName(ctx context.Context, name string) (*Player, error) {
	if Pool == nil {
		return nil, errors.New("db pool not initialized")
	}
	row := Pool.QueryRow(
		ctx,
		`SELECT id, player_name, is_wicket_keeper, is_retired FROM player WHERE player_name = $1`,
		name,
	)
	p := &Player{}
	if err := row.Scan(&p.ID, &p.Name, &p.IsWicketKeeper, &p.IsRetired); err != nil {
		return nil, err
	}
	return p, nil
}

// GetOrCreateByName fetches a player id or creates a new row.
func GetOrCreateByName(ctx context.Context, name string) (int64, error) {
	if Pool == nil {
		return 0, errors.New("db pool not initialized")
	}
	// Try to insert, if conflict on unique name, return existing id
	var id int64
	err := Pool.QueryRow(ctx, `INSERT INTO player(player_name) VALUES($1)
		ON CONFLICT (player_name) DO UPDATE SET player_name = EXCLUDED.player_name
		RETURNING id`, name).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

// UpsertConsistencyFmt inserts or updates a player”'s consistency scores for a
// given season and match format.
func UpsertConsistencyFmt(ctx context.Context, data *PlayerConsistency) error {
	// Consistency is now in feature_consistency_snapshots (precompute-features runner).
	return nil
}

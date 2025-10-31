package db

import (
	"context"
	"errors"
)

// Player represents the player table row
type Player struct {
	ID                 int64
	Name               string
	IsWicketKeeper     int16
	IsRetired          int16
	BattingConsistency *float32
	BowlingConsistency *float32
}

// GetByName returns a player by exact name match.
func GetByName(ctx context.Context, name string) (*Player, error) {
	if Pool == nil {
		return nil, errors.New("db pool not initialized")
	}
	row := Pool.QueryRow(
		ctx,
		`SELECT id, player_name, is_wicket_keeper, is_retired, batting_consistency, bowling_consistency
		FROM player WHERE player_name = $1`,
		name,
	)
	p := &Player{}
	if err := row.Scan(&p.ID, &p.Name, &p.IsWicketKeeper, &p.IsRetired, &p.BattingConsistency, &p.BowlingConsistency); err != nil {
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

// UpsertConsistency updates batting/bowling consistency values by player id.
func UpsertConsistency(ctx context.Context, playerID int64, batting, bowling *float32) error {
	if Pool == nil {
		return errors.New("db pool not initialized")
	}
	_, err := Pool.Exec(ctx, `UPDATE player SET 
		batting_consistency = COALESCE($2, batting_consistency),
		bowling_consistency = COALESCE($3, bowling_consistency)
		WHERE id = $1`, playerID, batting, bowling)
	return err
}

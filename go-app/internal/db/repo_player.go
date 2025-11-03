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

// PlayerConsistency represents a row in the player_consistency_data_fmt table.
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
	row := Pool.QueryRow(
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

	row := Pool.QueryRow(ctx, `
		SELECT player_id, season_id, format_id, batting_consistency, bowling_consistency
		FROM player_consistency_data_fmt
		WHERE player_id = $1 AND season_id = $2 AND format_id = $3
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
	if Pool == nil {
		return errors.New("db pool not initialized")
	}
	_, err := Pool.Exec(ctx, `
		INSERT INTO player_consistency_data_fmt (
			player_id, season_id, format_id, batting_consistency, bowling_consistency
		) VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (player_id, season_id, format_id)
		DO UPDATE SET
			batting_consistency = EXCLUDED.batting_consistency,
			bowling_consistency = EXCLUDED.bowling_consistency
	`, data.PlayerID, data.SeasonID, data.FormatID, data.BattingConsistency, data.BowlingConsistency)
	return err
}

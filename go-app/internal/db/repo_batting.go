package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Batting represents a batting_data row.
type Batting struct {
	ID              int64
	MatchID         int64
	PlayerID        int64
	Description     *string
	Runs            *int
	Balls           *int
	Minutes         *int
	Fours           *int
	Sixes           *int
	StrikeRate      *float32
	BattingPosition *int
}

// UpsertBatting inserts or updates batting_data by (match_id, player_id).
func UpsertBatting(ctx context.Context, b *Batting) error {
	if Pool == nil {
		return errors.New("db pool not initialized")
	}
	_, err := Pool.Exec(ctx, `INSERT INTO batting_data(
		match_id, player_id, description, runs, balls, minutes, fours, sixes, strike_rate, batting_position)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT (match_id, player_id) DO UPDATE SET
			description = COALESCE(EXCLUDED.description, batting_data.description),
			runs = COALESCE(EXCLUDED.runs, batting_data.runs),
			balls = COALESCE(EXCLUDED.balls, batting_data.balls),
			minutes = COALESCE(EXCLUDED.minutes, batting_data.minutes),
			fours = COALESCE(EXCLUDED.fours, batting_data.fours),
			sixes = COALESCE(EXCLUDED.sixes, batting_data.sixes),
			strike_rate = COALESCE(EXCLUDED.strike_rate, batting_data.strike_rate),
			batting_position = COALESCE(EXCLUDED.batting_position, batting_data.batting_position)
	`, b.MatchID, b.PlayerID, b.Description, b.Runs, b.Balls, b.Minutes, b.Fours, b.Sixes, b.StrikeRate, b.BattingPosition)
	return err
}

// UpsertBattingBatch inserts or updates multiple batting_data rows using pgx.CopyFrom and a temporary table.
func UpsertBattingBatch(ctx context.Context, rows []Batting) error {
	if PoolAPI == nil {
		return errors.New("db pool not initialized")
	}
	if len(rows) == 0 {
		return nil
	}

	// Use a temporary table and then MERGE for an atomic and performant upsert.
	tx, err := PoolAPI.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// 1. Create a temporary table with the same structure.
	// ON COMMIT DROP ensures it is cleaned up when the transaction ends.
	err = tx.Exec(ctx, `CREATE TEMP TABLE batting_data_tmp (LIKE batting_data INCLUDING DEFAULTS) ON COMMIT DROP`)
	if err != nil {
		return fmt.Errorf("create temp table: %w", err)
	}

	// 2. Use CopyFrom to bulk insert into the temporary table.
	_, err = tx.CopyFrom(
		ctx,
		pgx.Identifier{"batting_data_tmp"},
		[]string{
			"match_id",
			"player_id",
			"description",
			"runs",
			"balls",
			"minutes",
			"fours",
			"sixes",
			"strike_rate",
			"batting_position",
		},
		pgx.CopyFromSlice(len(rows), func(i int) ([]any, error) {
			r := rows[i]
			return []any{
				r.MatchID,
				r.PlayerID,
				r.Description,
				r.Runs,
				r.Balls,
				r.Minutes,
				r.Fours,
				r.Sixes,
				r.StrikeRate,
				r.BattingPosition,
			}, nil
		}),
	)
	if err != nil {
		return fmt.Errorf("copy from: %w", err)
	}

	// 3. Merge the temporary table into the main table.
	err = tx.Exec(ctx, `
		INSERT INTO batting_data (match_id, player_id, description, runs, balls, minutes, fours, sixes, strike_rate, batting_position)
		SELECT match_id, player_id, description, runs, balls, minutes, fours, sixes, strike_rate, batting_position
		FROM batting_data_tmp
		ON CONFLICT (match_id, player_id) DO UPDATE SET
			description = COALESCE(EXCLUDED.description, batting_data.description),
			runs = COALESCE(EXCLUDED.runs, batting_data.runs),
			balls = COALESCE(EXCLUDED.balls, batting_data.balls),
			minutes = COALESCE(EXCLUDED.minutes, batting_data.minutes),
			fours = COALESCE(EXCLUDED.fours, batting_data.fours),
			sixes = COALESCE(EXCLUDED.sixes, batting_data.sixes),
			strike_rate = COALESCE(EXCLUDED.strike_rate, batting_data.strike_rate),
			batting_position = COALESCE(EXCLUDED.batting_position, batting_data.batting_position)
	`)
	if err != nil {
		return fmt.Errorf("merge temp table: %w", err)
	}

	return tx.Commit(ctx)
}

package db

import (
	"context"
	"errors"
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

// UpsertBattingBatch inserts or updates multiple batting_data rows.
func UpsertBattingBatch(ctx context.Context, rows []Batting) error {
	if Pool == nil {
		return errors.New("db pool not initialized")
	}
	if len(rows) == 0 {
		return nil
	}
	// Use a transaction for the batch
	tx, err := Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	for _, b := range rows {
		_, err := tx.Exec(ctx, `INSERT INTO batting_data(
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
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

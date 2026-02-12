package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Bowling represents a bowling_data row.
type Bowling struct {
	ID       int64
	MatchID  int64
	PlayerID int64
	Overs    *float32
	Balls    *int
	Maidens  *int
	Runs     *int
	Wickets  *int
	Dots     *int
	Fours    *int
	Sixes    *int
	Econ     *float32
	Wides    *int
	NoBalls  *int
}

// UpsertBowling inserts or updates bowling_data by (match_id, player_id).
func UpsertBowling(ctx context.Context, b *Bowling) error {
	if Pool == nil {
		return errors.New("db pool not initialized")
	}
	_, err := Pool.Exec(ctx, `INSERT INTO bowling_data(
		match_id, player_id, overs, balls, maidens, runs, wickets, dots, fours, sixes, econ, wides, no_balls)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		ON CONFLICT (match_id, player_id) DO UPDATE SET
			overs = COALESCE(EXCLUDED.overs, bowling_data.overs),
			balls = COALESCE(EXCLUDED.balls, bowling_data.balls),
			maidens = COALESCE(EXCLUDED.maidens, bowling_data.maidens),
			runs = COALESCE(EXCLUDED.runs, bowling_data.runs),
			wickets = COALESCE(EXCLUDED.wickets, bowling_data.wickets),
			dots = COALESCE(EXCLUDED.dots, bowling_data.dots),
			fours = COALESCE(EXCLUDED.fours, bowling_data.fours),
			sixes = COALESCE(EXCLUDED.sixes, bowling_data.sixes),
			econ = COALESCE(EXCLUDED.econ, bowling_data.econ),
			wides = COALESCE(EXCLUDED.wides, bowling_data.wides),
			no_balls = COALESCE(EXCLUDED.no_balls, bowling_data.no_balls)
	`, b.MatchID, b.PlayerID, b.Overs, b.Balls, b.Maidens, b.Runs, b.Wickets, b.Dots, b.Fours, b.Sixes, b.Econ, b.Wides, b.NoBalls)
	return err
}

// UpsertBowlingBatch inserts or updates multiple bowling_data rows using pgx.CopyFrom and a temporary table.
func UpsertBowlingBatch(ctx context.Context, rows []Bowling) error {
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
	defer tx.Rollback(ctx)

	// 1. Create a temporary table with the same structure.
	err = tx.Exec(ctx, `CREATE TEMP TABLE bowling_data_tmp (LIKE bowling_data INCLUDING DEFAULTS) ON COMMIT DROP`)
	if err != nil {
		return fmt.Errorf("create temp table: %w", err)
	}

	// 2. Use CopyFrom to bulk insert into the temporary table.
	_, err = tx.CopyFrom(ctx,
		pgx.Identifier{"bowling_data_tmp"},
		[]string{"match_id", "player_id", "overs", "balls", "maidens", "runs", "wickets", "dots", "fours", "sixes", "econ", "wides", "no_balls"},
		pgx.CopyFromSlice(len(rows), func(i int) ([]any, error) {
			r := rows[i]
			return []any{r.MatchID, r.PlayerID, r.Overs, r.Balls, r.Maidens, r.Runs, r.Wickets, r.Dots, r.Fours, r.Sixes, r.Econ, r.Wides, r.NoBalls}, nil
		}),
	)
	if err != nil {
		return fmt.Errorf("copy from: %w", err)
	}

	// 3. Merge the temporary table into the main table.
	err = tx.Exec(ctx, `
		INSERT INTO bowling_data (match_id, player_id, overs, balls, maidens, runs, wickets, dots, fours, sixes, econ, wides, no_balls)
		SELECT match_id, player_id, overs, balls, maidens, runs, wickets, dots, fours, sixes, econ, wides, no_balls
		FROM bowling_data_tmp
		ON CONFLICT (match_id, player_id) DO UPDATE SET
			overs = COALESCE(EXCLUDED.overs, bowling_data.overs),
			balls = COALESCE(EXCLUDED.balls, bowling_data.balls),
			maidens = COALESCE(EXCLUDED.maidens, bowling_data.maidens),
			runs = COALESCE(EXCLUDED.runs, bowling_data.runs),
			wickets = COALESCE(EXCLUDED.wickets, bowling_data.wickets),
			dots = COALESCE(EXCLUDED.dots, bowling_data.dots),
			fours = COALESCE(EXCLUDED.fours, bowling_data.fours),
			sixes = COALESCE(EXCLUDED.sixes, bowling_data.sixes),
			econ = COALESCE(EXCLUDED.econ, bowling_data.econ),
			wides = COALESCE(EXCLUDED.wides, bowling_data.wides),
			no_balls = COALESCE(EXCLUDED.no_balls, bowling_data.no_balls)
	`)
	if err != nil {
		return fmt.Errorf("merge temp table: %w", err)
	}

	return tx.Commit(ctx)
}

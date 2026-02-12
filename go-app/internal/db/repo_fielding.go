package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Fielding represents a fielding_data row.
type Fielding struct {
	ID                int64
	MatchID           int64
	PlayerID          int64
	Catches           *int
	RunOuts           *int
	DroppedCatches    *int
	MissedRunOuts     *int
	Stumpings         *int
	RunoutsDirectHits *int
}

// UpsertFielding inserts or updates fielding_data by (match_id, player_id).
func UpsertFielding(ctx context.Context, f *Fielding) error {
	if Pool == nil {
		return errors.New("db pool not initialized")
	}
	_, err := Pool.Exec(ctx, `INSERT INTO fielding_data(
		match_id, player_id, catches, run_outs, dropped_catches, missed_run_outs, stumpings, runouts_direct_hits)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (match_id, player_id) DO UPDATE SET
			catches = COALESCE(EXCLUDED.catches, fielding_data.catches),
			run_outs = COALESCE(EXCLUDED.run_outs, fielding_data.run_outs),
			dropped_catches = COALESCE(EXCLUDED.dropped_catches, fielding_data.dropped_catches),
			missed_run_outs = COALESCE(EXCLUDED.missed_run_outs, fielding_data.missed_run_outs),
			stumpings = COALESCE(EXCLUDED.stumpings, fielding_data.stumpings),
			runouts_direct_hits = COALESCE(EXCLUDED.runouts_direct_hits, fielding_data.runouts_direct_hits)
	`, f.MatchID, f.PlayerID, f.Catches, f.RunOuts, f.DroppedCatches, f.MissedRunOuts, f.Stumpings, f.RunoutsDirectHits)
	return err
}

// UpsertFieldingBatch inserts or updates multiple fielding_data rows using pgx.CopyFrom and a temporary table.
func UpsertFieldingBatch(ctx context.Context, rows []Fielding) error {
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
	err = tx.Exec(ctx, `CREATE TEMP TABLE fielding_data_tmp (LIKE fielding_data INCLUDING DEFAULTS) ON COMMIT DROP`)
	if err != nil {
		return fmt.Errorf("create temp table: %w", err)
	}

	// 2. Use CopyFrom to bulk insert into the temporary table.
	_, err = tx.CopyFrom(
		ctx,
		pgx.Identifier{"fielding_data_tmp"},
		[]string{
			"match_id",
			"player_id",
			"catches",
			"run_outs",
			"dropped_catches",
			"missed_run_outs",
			"stumpings",
			"runouts_direct_hits",
		},
		pgx.CopyFromSlice(len(rows), func(i int) ([]any, error) {
			r := rows[i]
			return []any{
				r.MatchID,
				r.PlayerID,
				r.Catches,
				r.RunOuts,
				r.DroppedCatches,
				r.MissedRunOuts,
				r.Stumpings,
				r.RunoutsDirectHits,
			}, nil
		}),
	)
	if err != nil {
		return fmt.Errorf("copy from: %w", err)
	}

	// 3. Merge the temporary table into the main table.
	err = tx.Exec(ctx, `
		INSERT INTO fielding_data (match_id, player_id, catches, run_outs, dropped_catches, missed_run_outs, stumpings, runouts_direct_hits)
		SELECT match_id, player_id, catches, run_outs, dropped_catches, missed_run_outs, stumpings, runouts_direct_hits
		FROM fielding_data_tmp
		ON CONFLICT (match_id, player_id) DO UPDATE SET
			catches = COALESCE(EXCLUDED.catches, fielding_data.catches),
			run_outs = COALESCE(EXCLUDED.run_outs, fielding_data.run_outs),
			dropped_catches = COALESCE(EXCLUDED.dropped_catches, fielding_data.dropped_catches),
			missed_run_outs = COALESCE(EXCLUDED.missed_run_outs, fielding_data.missed_run_outs),
			stumpings = COALESCE(EXCLUDED.stumpings, fielding_data.stumpings),
			runouts_direct_hits = COALESCE(EXCLUDED.runouts_direct_hits, fielding_data.runouts_direct_hits)
	`)
	if err != nil {
		return fmt.Errorf("merge temp table: %w", err)
	}

	return tx.Commit(ctx)
}

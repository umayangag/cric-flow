package db

import (
	"context"
	"errors"
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

// UpsertFieldingBatch inserts or updates multiple fielding_data rows.
func UpsertFieldingBatch(ctx context.Context, rows []Fielding) error {
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
	defer tx.Rollback(ctx)

	for _, f := range rows {
		_, err := tx.Exec(ctx, `INSERT INTO fielding_data(
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
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

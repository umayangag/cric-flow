package db

import (
	"context"
	"errors"
)

// Fielding represents a fielding_data row.
type Fielding struct {
	ID             int64
	MatchID        int64
	PlayerID       int64
	Catches        *int
	RunOuts        *int
	DroppedCatches *int
	MissedRunOuts  *int
}

// UpsertFielding inserts or updates fielding_data by (match_id, player_id).
func UpsertFielding(ctx context.Context, f *Fielding) error {
	if Pool == nil {
		return errors.New("db pool not initialized")
	}
	_, err := Pool.Exec(ctx, `INSERT INTO fielding_data(
		match_id, player_id, catches, run_outs, dropped_catches, missed_run_outs)
		VALUES($1,$2,$3,$4,$5,$6)
		ON CONFLICT (match_id, player_id) DO UPDATE SET
			catches = COALESCE(EXCLUDED.catches, fielding_data.catches),
			run_outs = COALESCE(EXCLUDED.run_outs, fielding_data.run_outs),
			dropped_catches = COALESCE(EXCLUDED.dropped_catches, fielding_data.dropped_catches),
			missed_run_outs = COALESCE(EXCLUDED.missed_run_outs, fielding_data.missed_run_outs)
	`, f.MatchID, f.PlayerID, f.Catches, f.RunOuts, f.DroppedCatches, f.MissedRunOuts)
	return err
}

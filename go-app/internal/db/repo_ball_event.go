package db

import (
	"context"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5"
)

// smallBatchThreshold determines when to switch from individual INSERTs to a bulk COPY.
// For very small batches, the overhead of creating a temp table and using COPY can be higher
// than simple INSERT statements. This value is a heuristic.
const smallBatchThreshold = 8

// BallEventRow mirrors columns for insertion into ball_event.
// Optional player references are pointers; nil means unresolved/unknown.
// fielder_ids is omitted in this version for simplicity (left NULL).
type BallEventRow struct {
	MatchID      int64
	Innings      int
	Over         int
	Ball         int
	BallSeq      int
	IsLegal      bool
	Phase        string
	StrikerID    *int64
	NonStrikerID *int64
	BowlerID     *int64
	RunsBatter   int
	RunsExtras   int
	RunsTotal    int
	ExtrasKind   *string
	WicketKind   *string
	PlayerOutID  *int64
}

// InsertBallEvents inserts rows idempotently using the natural primary key
// (match_id, innings, over, ball). On conflicts, it does nothing to remain safe
// for re-runs and backfills.
func InsertBallEvents(ctx context.Context, rows []BallEventRow) error {
	// Pool must be available for this function per requirement.
	if PoolAPI == nil {
		return errors.New("db pool not initialized")
	}
	if len(rows) == 0 {
		slog.Warn("[InsertBallEvents] no rows to insert")
		return nil
	}
	slog.Debug("inserting ball events", slog.Int("num_rows", len(rows)))

	if len(rows) <= smallBatchThreshold {
		for i := range rows {
			r := rows[i]
			slog.Debug("insert ball event", slog.Int("match_id", int(r.MatchID)), slog.Any("row", r))
			err := PoolAPI.Exec(ctx, `
                INSERT INTO ball_event(
                    match_id, innings, "over", ball, ball_seq, is_legal, phase,
                    striker_id, non_striker_id, bowler_id,
                    runs_batter, runs_extras, runs_total,
                    extras_kind, wicket_kind, player_out_id
                ) VALUES (
                    $1,$2,$3,$4,$5,$6,$7,
                    $8,$9,$10,
                    $11,$12,$13,
                    $14,$15,$16
                )
                ON CONFLICT (match_id, innings, "over", ball) DO NOTHING
            `,
				r.MatchID, r.Innings, r.Over, r.Ball, r.BallSeq, r.IsLegal, r.Phase,
				r.StrikerID, r.NonStrikerID, r.BowlerID,
				r.RunsBatter, r.RunsExtras, r.RunsTotal,
				r.ExtrasKind, r.WicketKind, r.PlayerOutID,
			)
			if err != nil {
				slog.Error("insert ball events failed", slog.Any("err", err))
				return err
			}
		}
		return nil
	}

	// Bulk path: COPY rows into a temporary staging table, then INSERT .. ON CONFLICT DO NOTHING.
	tx, err := PoolAPI.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		// Ensure rollback if not already committed.
		_ = tx.Rollback(ctx)
	}()

	// Create a temp table with the exact columns we intend to insert.
	// Using CTAS to inherit column types from ball_event while restricting to insert columns only.
	// DROP first and ON COMMIT DROP avoid reuse/corruption when connection is pooled.
	_ = tx.Exec(ctx, `DROP TABLE IF EXISTS ball_event_stage`)
	err = tx.Exec(ctx, `
        CREATE TEMP TABLE ball_event_stage ON COMMIT DROP AS
        SELECT 
            match_id::bigint,
            innings::int,
            "over"::int,
            ball::int,
            ball_seq::int,
            is_legal::boolean,
            phase::text,
            striker_id::bigint,
            non_striker_id::bigint,
            bowler_id::bigint,
            runs_batter::int,
            runs_extras::int,
            runs_total::int,
            extras_kind::text,
            wicket_kind::text,
            player_out_id::bigint
        FROM ball_event
        WITH NO DATA
    `)
	if err != nil {
		return err
	}

	// Build COPY rows.
	data := make([][]any, 0, len(rows))
	for i := range rows {
		r := rows[i]
		data = append(data, []any{
			r.MatchID,
			r.Innings,
			r.Over,
			r.Ball,
			r.BallSeq,
			r.IsLegal,
			r.Phase,
			r.StrikerID,
			r.NonStrikerID,
			r.BowlerID,
			r.RunsBatter,
			r.RunsExtras,
			r.RunsTotal,
			r.ExtrasKind,
			r.WicketKind,
			r.PlayerOutID,
		})
	}

	// Perform COPY INTO the staging table.
	_, err = tx.CopyFrom(
		ctx,
		pgx.Identifier{"ball_event_stage"},
		[]string{
			"match_id", "innings", "over", "ball", "ball_seq", "is_legal", "phase",
			"striker_id", "non_striker_id", "bowler_id",
			"runs_batter", "runs_extras", "runs_total",
			"extras_kind", "wicket_kind", "player_out_id",
		},
		pgx.CopyFromRows(data),
	)
	if err != nil {
		return err
	}

	// Insert into the real table with idempotency.
	err = tx.Exec(ctx, `
        INSERT INTO ball_event(
            match_id, innings, "over", ball, ball_seq, is_legal, phase,
            striker_id, non_striker_id, bowler_id,
            runs_batter, runs_extras, runs_total,
            extras_kind, wicket_kind, player_out_id
        )
        SELECT 
            match_id, innings, "over", ball, ball_seq, is_legal, phase,
            striker_id, non_striker_id, bowler_id,
            runs_batter, runs_extras, runs_total,
            extras_kind, wicket_kind, player_out_id
        FROM ball_event_stage
        ON CONFLICT (match_id, innings, "over", ball) DO NOTHING
    `)
	if err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}

// InsertBallEventsTx inserts ball_event rows using the given transaction.
func InsertBallEventsTx(ctx context.Context, tx CopyFromTx, rows []BallEventRow) error {
	if len(rows) == 0 {
		return nil
	}
	if len(rows) <= smallBatchThreshold {
		for i := range rows {
			r := rows[i]
			if err := tx.Exec(ctx, `
                INSERT INTO ball_event(
                    match_id, innings, "over", ball, ball_seq, is_legal, phase,
                    striker_id, non_striker_id, bowler_id,
                    runs_batter, runs_extras, runs_total,
                    extras_kind, wicket_kind, player_out_id
                ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
                ON CONFLICT (match_id, innings, "over", ball) DO NOTHING
            `,
				r.MatchID, r.Innings, r.Over, r.Ball, r.BallSeq, r.IsLegal, r.Phase,
				r.StrikerID, r.NonStrikerID, r.BowlerID,
				r.RunsBatter, r.RunsExtras, r.RunsTotal,
				r.ExtrasKind, r.WicketKind, r.PlayerOutID,
			); err != nil {
				return err
			}
		}
		return nil
	}
	_ = tx.Exec(ctx, `DROP TABLE IF EXISTS ball_event_stage`)
	err := tx.Exec(ctx, `
        CREATE TEMP TABLE ball_event_stage ON COMMIT DROP AS
        SELECT match_id::bigint, innings::int, "over"::int, ball::int, ball_seq::int,
            is_legal::boolean, phase::text, striker_id::bigint, non_striker_id::bigint,
            bowler_id::bigint, runs_batter::int, runs_extras::int, runs_total::int,
            extras_kind::text, wicket_kind::text, player_out_id::bigint
        FROM ball_event
        WITH NO DATA
    `)
	if err != nil {
		return err
	}
	data := make([][]any, 0, len(rows))
	for i := range rows {
		r := rows[i]
		data = append(data, []any{
			r.MatchID, r.Innings, r.Over, r.Ball, r.BallSeq, r.IsLegal, r.Phase,
			r.StrikerID, r.NonStrikerID, r.BowlerID,
			r.RunsBatter, r.RunsExtras, r.RunsTotal,
			r.ExtrasKind, r.WicketKind, r.PlayerOutID,
		})
	}
	_, err = tx.CopyFrom(ctx, pgx.Identifier{"ball_event_stage"},
		[]string{
			"match_id", "innings", "over", "ball", "ball_seq", "is_legal", "phase",
			"striker_id", "non_striker_id", "bowler_id",
			"runs_batter", "runs_extras", "runs_total",
			"extras_kind", "wicket_kind", "player_out_id",
		},
		pgx.CopyFromRows(data))
	if err != nil {
		return err
	}
	return tx.Exec(ctx, `
        INSERT INTO ball_event(match_id, innings, "over", ball, ball_seq, is_legal, phase,
            striker_id, non_striker_id, bowler_id, runs_batter, runs_extras, runs_total,
            extras_kind, wicket_kind, player_out_id)
        SELECT match_id, innings, "over", ball, ball_seq, is_legal, phase,
            striker_id, non_striker_id, bowler_id, runs_batter, runs_extras, runs_total,
            extras_kind, wicket_kind, player_out_id
        FROM ball_event_stage
        ON CONFLICT (match_id, innings, "over", ball) DO NOTHING
    `)
}

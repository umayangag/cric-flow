package db

import (
	"context"

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
	// The extras by kind, as the file records them. RunsExtras is their sum; ExtrasKind is
	// a one-word summary of them that loses the second kind on a delivery that has two.
	ExtrasWides   int
	ExtrasNoBalls int
	ExtrasByes    int
	ExtrasLegByes int
	ExtrasPenalty int
	ExtrasKind    *string
	WicketKind    *string
	PlayerOutID   *int64
}

// InsertBallEventsTx inserts ball_event rows using the given transaction.
//
// A plain insert, with no ON CONFLICT clause: the caller has already cleared the match
// through DeleteMatchFactsTx, so every row here is new. The clause this replaces was
// DO NOTHING, which made a re-import of an already-imported match a no-op -- the whole of
// IMPORT-03. Within a single file the primary key (match_id, innings, over, ball) cannot
// repeat unless the file lists the same over twice, and a file like that should fail the
// import loudly rather than lose a delivery silently.
func InsertBallEventsTx(ctx context.Context, tx CopyFromTx, rows []BallEventRow) error {
	if len(rows) == 0 {
		return nil
	}
	if len(rows) <= smallBatchThreshold {
		for i := range rows {
			r := rows[i]
			if err := tx.Exec(
				ctx, `
                INSERT INTO ball_event(
                    match_id, innings, "over", ball, ball_seq, is_legal, phase,
                    striker_id, non_striker_id, bowler_id,
                    runs_batter, runs_extras, runs_total,
                    extras_wides, extras_noballs, extras_byes, extras_legbyes, extras_penalty,
                    extras_kind, wicket_kind, player_out_id
                ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21)
            `,
				r.MatchID, r.Innings, r.Over, r.Ball, r.BallSeq, r.IsLegal, r.Phase,
				r.StrikerID, r.NonStrikerID, r.BowlerID,
				r.RunsBatter, r.RunsExtras, r.RunsTotal,
				r.ExtrasWides, r.ExtrasNoBalls, r.ExtrasByes, r.ExtrasLegByes, r.ExtrasPenalty,
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
            extras_wides::int, extras_noballs::int, extras_byes::int, extras_legbyes::int,
            extras_penalty::int, extras_kind::text, wicket_kind::text, player_out_id::bigint
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
			r.ExtrasWides, r.ExtrasNoBalls, r.ExtrasByes, r.ExtrasLegByes, r.ExtrasPenalty,
			r.ExtrasKind, r.WicketKind, r.PlayerOutID,
		})
	}
	_, err = tx.CopyFrom(ctx, pgx.Identifier{"ball_event_stage"},
		[]string{
			"match_id", "innings", "over", "ball", "ball_seq", "is_legal", "phase",
			"striker_id", "non_striker_id", "bowler_id",
			"runs_batter", "runs_extras", "runs_total",
			"extras_wides", "extras_noballs", "extras_byes", "extras_legbyes", "extras_penalty",
			"extras_kind", "wicket_kind", "player_out_id",
		},
		pgx.CopyFromRows(data))
	if err != nil {
		return err
	}
	return tx.Exec(ctx, `
        INSERT INTO ball_event(match_id, innings, "over", ball, ball_seq, is_legal, phase,
            striker_id, non_striker_id, bowler_id, runs_batter, runs_extras, runs_total,
            extras_wides, extras_noballs, extras_byes, extras_legbyes, extras_penalty,
            extras_kind, wicket_kind, player_out_id)
        SELECT match_id, innings, "over", ball, ball_seq, is_legal, phase,
            striker_id, non_striker_id, bowler_id, runs_batter, runs_extras, runs_total,
            extras_wides, extras_noballs, extras_byes, extras_legbyes, extras_penalty,
            extras_kind, wicket_kind, player_out_id
        FROM ball_event_stage
    `)
}

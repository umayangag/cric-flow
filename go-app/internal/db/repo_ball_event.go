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
//
// The delivery's wickets are not on the row: a delivery can carry more than one, and
// each is a BallEventWicketRow in ball_event_wicket (IMPORT-06).
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
}

// BallEventWicketRow is one wicket on one delivery, for insertion into ball_event_wicket.
// WicketNumber is the wicket's 1-based position among the delivery's wickets, in the
// order the file lists them; Kind is the vocabulary's spelling (configs/wicket_kinds.json).
type BallEventWicketRow struct {
	MatchID      int64
	Innings      int
	Over         int
	Ball         int
	WicketNumber int
	Kind         string
	PlayerOutID  *int64
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
                    extras_kind
                ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)
            `,
				r.MatchID, r.Innings, r.Over, r.Ball, r.BallSeq, r.IsLegal, r.Phase,
				r.StrikerID, r.NonStrikerID, r.BowlerID,
				r.RunsBatter, r.RunsExtras, r.RunsTotal,
				r.ExtrasWides, r.ExtrasNoBalls, r.ExtrasByes, r.ExtrasLegByes, r.ExtrasPenalty,
				r.ExtrasKind,
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
            extras_penalty::int, extras_kind::text
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
			r.ExtrasKind,
		})
	}
	_, err = tx.CopyFrom(ctx, pgx.Identifier{"ball_event_stage"},
		[]string{
			"match_id", "innings", "over", "ball", "ball_seq", "is_legal", "phase",
			"striker_id", "non_striker_id", "bowler_id",
			"runs_batter", "runs_extras", "runs_total",
			"extras_wides", "extras_noballs", "extras_byes", "extras_legbyes", "extras_penalty",
			"extras_kind",
		},
		pgx.CopyFromRows(data))
	if err != nil {
		return err
	}
	return tx.Exec(ctx, `
        INSERT INTO ball_event(match_id, innings, "over", ball, ball_seq, is_legal, phase,
            striker_id, non_striker_id, bowler_id, runs_batter, runs_extras, runs_total,
            extras_wides, extras_noballs, extras_byes, extras_legbyes, extras_penalty,
            extras_kind)
        SELECT match_id, innings, "over", ball, ball_seq, is_legal, phase,
            striker_id, non_striker_id, bowler_id, runs_batter, runs_extras, runs_total,
            extras_wides, extras_noballs, extras_byes, extras_legbyes, extras_penalty,
            extras_kind
        FROM ball_event_stage
    `)
}

// InsertBallEventWicketsTx inserts ball_event_wicket rows using the given transaction,
// after the delivery rows they reference. A match has a few dozen wickets at most, so
// these are plain inserts; like the delivery rows they carry no ON CONFLICT clause,
// because the match was cleared before anything was written.
func InsertBallEventWicketsTx(ctx context.Context, tx CopyFromTx, rows []BallEventWicketRow) error {
	for i := range rows {
		r := rows[i]
		if err := tx.Exec(
			ctx, `
            INSERT INTO ball_event_wicket(match_id, innings, "over", ball, wicket_number, kind, player_out_id)
            VALUES ($1,$2,$3,$4,$5,$6,$7)
        `,
			r.MatchID, r.Innings, r.Over, r.Ball, r.WicketNumber, r.Kind, r.PlayerOutID,
		); err != nil {
			return err
		}
	}
	return nil
}

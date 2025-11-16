package seqcalc

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

// endPressureCalc computes end-of-over pressure metrics for positions 5 and 6 (legal balls only).
type endPressureCalc struct{}

func NewEndPressureCalculator() Calculator { return endPressureCalc{} }
func (endPressureCalc) Name() Target       { return TargetEndPressure }

// evRowEP is the local projection for end pressure aggregation.
type evRowEP struct {
	MatchID     int64
	Innings     int
	Over        int
	BallSeq     int
	Phase       string
	IsLegal     bool
	BowlerID    sql.NullInt64
	RunsBatter  int
	RunsTotal   int
	PlayerOutID sql.NullInt64
	AsOf        time.Time
	FormatID    int
}

func (endPressureCalc) Compute(ctx context.Context, params Params, dryRun bool) error {
	if dryRun {
		return nil
	}
	formatIDs := mapFormatIDs(params.FormatCode)
	ev, err := queryEventsForEndPressure(ctx, formatIDs)
	if err != nil {
		return err
	}
	if len(ev) == 0 {
		return nil
	}
	rows := aggregateEndPressure(ev)
	if len(rows) == 0 {
		return nil
	}
	return db.UpsertOverEndPressure(ctx, rows)
}

func queryEventsForEndPressure(ctx context.Context, formatIDs []int) ([]evRowEP, error) {
	if db.Pool == nil {
		return nil, fmt.Errorf("db not connected")
	}
	place := ""
	args := []any{}
	for i, id := range formatIDs {
		if i > 0 {
			place += ","
		}
		place += fmt.Sprintf("$%d", i+1)
		args = append(args, id)
	}
	q := fmt.Sprintf(`
		SELECT
		  be.match_id, be.innings, be.over, be.ball_seq, be.phase,
		  be.is_legal,
		  be.bowler_id, be.runs_batter, be.runs_total, be.player_out_id,
		  COALESCE(md.match_date, md.date) AS match_date, md.format_id
		FROM ball_event be
		JOIN match_details md ON md.match_id = be.match_id
		WHERE md.format_id IN (%s)
		ORDER BY be.match_id, be.innings, be.ball_seq
	`, place)
	dr, err := db.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer dr.Close()
	var out []evRowEP
	for dr.Next() {
		var r evRowEP
		if err := dr.Scan(&r.MatchID, &r.Innings, &r.Over, &r.BallSeq, &r.Phase,
			&r.IsLegal,
			&r.BowlerID, &r.RunsBatter, &r.RunsTotal, &r.PlayerOutID,
			&r.AsOf, &r.FormatID); err != nil {
			return nil, err
		}
		if r.AsOf.IsZero() {
			continue
		}
		out = append(out, r)
	}
	if err := dr.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// aggregateEndPressure finds the 5th and 6th legal balls per over and aggregates counts per key.
func aggregateEndPressure(events []evRowEP) []db.OverEndPressureRow {
	// We count legal positions within each (match, innings, over, bowler).
	type overKey struct {
		match      int64
		inng, over int
		bow        int64
	}
	type aggKey struct {
		asof     string
		fmt, pos int
		pid      int64
		phase    string
	}

	// For each over, we need to know which events correspond to 5th and 6th legal deliveries.
	// We'll perform a single pass and increment a counter per overKey only for legal deliveries.
	legalPos := map[overKey]int{}

	// Totals per emitted key
	counts := map[aggKey]*db.OverEndPressureRow{}

	for _, e := range events {
		if !e.BowlerID.Valid {
			continue
		}
		ok := overKey{match: e.MatchID, inng: e.Innings, over: e.Over, bow: e.BowlerID.Int64}
		if e.IsLegal {
			legalPos[ok]++
			pos := legalPos[ok]
			if pos == 5 || pos == 6 {
				key := aggKey{asof: e.AsOf.Format("2006-01-02"), fmt: e.FormatID, pos: pos, pid: ok.bow, phase: e.Phase}
				row := counts[key]
				if row == nil {
					row = &db.OverEndPressureRow{
						AsOfDate: key.asof,
						FormatID: key.fmt,
						Scope:    "overall",
						PlayerID: key.pid,
						Phase:    key.phase,
						Position: key.pos,
					}
					counts[key] = row
				}
				row.Balls++
				if e.RunsBatter == 4 || e.RunsBatter == 6 {
					row.Boundaries++
				}
				if e.PlayerOutID.Valid {
					row.Wickets++
				}
			}
		}
	}
	// finalize and compute rates
	out := make([]db.OverEndPressureRow, 0, len(counts))
	for _, r := range counts {
		if r.Balls > 0 {
			r.BoundaryRate = float64(r.Boundaries) / float64(r.Balls)
			r.WicketRate = float64(r.Wickets) / float64(r.Balls)
			out = append(out, *r)
		}
	}
	return out
}

package seqcalc

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

// overPosCalc computes incidence of boundaries and wickets at ball 1 and ball 6 by bowler and phase.
type overPosCalc struct{}

func NewOverPosCalculator() Calculator { return overPosCalc{} }
func (overPosCalc) Name() Target       { return TargetOverPos }

// evRowOverPos is a local projection tailored for over position aggregation.
type evRowOverPos struct {
	MatchID     int64
	Innings     int
	Over        int
	Ball        int
	Phase       string
	IsLegal     bool
	BowlerID    sql.NullInt64
	RunsBatter  int
	PlayerOutID sql.NullInt64
	AsOf        sql.NullTime
	FormatID    int
}

// overPosRow is an internal aggregation output used by tests; Compute converts to DB rows.
type overPosRow struct {
	AsOfDate     string
	FormatID     int
	PlayerID     int64
	Phase        string
	Position     int // 1 or 6
	Balls        int
	Boundaries   int
	Wickets      int
	BoundaryRate float64
	WicketRate   float64
}

func (overPosCalc) Compute(ctx context.Context, params Params, dryRun bool) error {
	if dryRun {
		return nil
	}
	formatIDs := mapFormatIDs(params.FormatCode)
	rows, err := queryEventsForOverPos(ctx, formatIDs)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	agg := aggregateOverPos(rows)
	if len(agg) == 0 {
		return nil
	}
	// Convert to DB rows and upsert
	dbRows := make([]db.OverPosRow, 0, len(agg))
	for i := range agg {
		r := agg[i]
		dbRows = append(dbRows, db.OverPosRow{
			AsOfDate:     r.AsOfDate,
			FormatID:     r.FormatID,
			Scope:        "overall",
			ScopeID:      nil,
			PlayerID:     r.PlayerID,
			Phase:        r.Phase,
			Position:     r.Position,
			Balls:        r.Balls,
			Boundaries:   r.Boundaries,
			Wickets:      r.Wickets,
			BoundaryRate: r.BoundaryRate,
			WicketRate:   r.WicketRate,
		})
	}
	return db.UpsertOverPos(ctx, dbRows)
}

func queryEventsForOverPos(ctx context.Context, formatIDs []int) ([]evRowOverPos, error) {
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
		  be.match_id, be.innings, be.over, be.ball, be.phase,
		  be.is_legal,
		  be.bowler_id, be.runs_batter, be.player_out_id,
		  md.match_date, md.format_id
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
	var out []evRowOverPos
	for dr.Next() {
		var r evRowOverPos
		if err := dr.Scan(&r.MatchID, &r.Innings, &r.Over, &r.Ball, &r.Phase,
			&r.IsLegal,
			&r.BowlerID, &r.RunsBatter, &r.PlayerOutID,
			&r.AsOf, &r.FormatID); err != nil {
			return nil, err
		}
		if !r.AsOf.Valid {
			continue
		}
		out = append(out, r)
	}
	if err := dr.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// aggregateOverPos groups by as-of, format, player, phase, and position (1 or 6).
func aggregateOverPos(events []evRowOverPos) []overPosRow {
	type key struct {
		asof     string
		fmt, pos int
		pid      int64
		phase    string
	}
	counts := map[key]struct{ balls, bounds, wkts int }{}
	for _, e := range events {
		if !e.BowlerID.Valid || !e.AsOf.Valid || !e.IsLegal {
			continue
		}
		if e.Ball != 1 && e.Ball != 6 {
			continue
		}
		k := key{
			asof:  e.AsOf.Time.Format("2006-01-02"),
			fmt:   e.FormatID,
			pos:   e.Ball,
			pid:   e.BowlerID.Int64,
			phase: e.Phase,
		}
		v := counts[k]
		v.balls++
		if e.RunsBatter == 4 || e.RunsBatter == 6 {
			v.bounds++
		}
		if e.PlayerOutID.Valid {
			v.wkts++
		}
		counts[k] = v
	}
	out := make([]overPosRow, 0, len(counts))
	for k, v := range counts {
		if v.balls == 0 { // should not happen but guard
			continue
		}
		row := overPosRow{
			AsOfDate:   k.asof,
			FormatID:   k.fmt,
			PlayerID:   k.pid,
			Phase:      k.phase,
			Position:   k.pos,
			Balls:      v.balls,
			Boundaries: v.bounds,
			Wickets:    v.wkts,
		}
		row.BoundaryRate = float64(row.Boundaries) / float64(row.Balls)
		row.WicketRate = float64(row.Wickets) / float64(row.Balls)
		out = append(out, row)
	}
	return out
}

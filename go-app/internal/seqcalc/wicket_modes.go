package seqcalc

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/formats"
)

// wicketModesCalc computes wicket mode distributions by bowler and phase.
type wicketModesCalc struct{}

func NewWicketModesCalculator() Calculator { return wicketModesCalc{} }
func (wicketModesCalc) Name() Target       { return TargetWicketModes }

// Compute loads events with wicket_kind and aggregates counts by (as_of, fmt, bowler, phase, mode).
func (wicketModesCalc) Compute(ctx context.Context, params Params, dryRun bool) error {
	if dryRun {
		return nil
	}
	formatIDs := formats.MapFormatIDs(params.FormatCode)
	rows, err := queryEventsWithWicketKind(ctx, formatIDs)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	agg := aggregateWicketModes(rows)
	if len(agg) == 0 {
		return nil
	}
	return db.UpsertWicketModes(ctx, agg)
}

// evRowWK is a local projection that includes wicket_kind.
type evRowWK struct {
	MatchID     int64
	Innings     int
	BallSeq     int
	Phase       string
	IsLegal     bool
	BowlerID    sql.NullInt64
	ExtrasKind  sql.NullString
	RunsTotal   int
	PlayerOutID sql.NullInt64
	WicketKind  sql.NullString
	AsOf        sql.NullTime
	FormatID    int
}

func queryEventsWithWicketKind(ctx context.Context, formatIDs []int) ([]evRowWK, error) {
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
		  be.match_id, be.innings, be.ball_seq, be.phase,
		  be.is_legal,
		  be.bowler_id, be.extras_kind, be.runs_total, be.player_out_id, be.wicket_kind,
		  m.match_date, m.format_id
		FROM ball_event be
		JOIN match m ON m.match_id = be.match_id
		WHERE m.format_id IN (%s)
		ORDER BY be.match_id, be.innings, be.ball_seq
	`, place)
	dr, err := db.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer dr.Close()
	var out []evRowWK
	for dr.Next() {
		var r evRowWK
		if err := dr.Scan(&r.MatchID, &r.Innings, &r.BallSeq, &r.Phase,
			&r.IsLegal,
			&r.BowlerID, &r.ExtrasKind, &r.RunsTotal, &r.PlayerOutID, &r.WicketKind,
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

// key for aggregation
type wkAggKey struct {
	asof     string
	fmt      int
	pid      int64
	ph, mode string
}

func aggregateWicketModes(events []evRowWK) []db.WicketModeRow {
	// First pass: collect total legal balls per (asof, fmt, player, phase)
	totals := map[wkAggKey]int{}
	// Second pass data: wickets per mode for the same key (excluding mode in key for totals)
	modeWkts := map[wkAggKey]int{}
	// Track which phases have at least one wicketed mode so we can decide whether to emit an empty-mode row
	hasAnyWicket := map[wkAggKey]bool{}

	for _, e := range events {
		if !e.BowlerID.Valid {
			continue
		}
		pid := e.BowlerID.Int64
		asof := ""
		if e.AsOf.Valid {
			asof = e.AsOf.Time.Format("2006-01-02")
		}
		baseKey := wkAggKey{asof: asof, fmt: e.FormatID, pid: pid, ph: e.Phase}
		if e.IsLegal {
			totals[baseKey]++
		}
		mode := canonicalMode(e.WicketKind)
		if e.IsLegal && e.PlayerOutID.Valid && mode != "" {
			mk := wkAggKey{asof: asof, fmt: e.FormatID, pid: pid, ph: e.Phase, mode: mode}
			modeWkts[mk]++
			hasAnyWicket[baseKey] = true
		}
	}

	// Emit rows: for keys with any wicket, emit one row per mode with wickets>0; balls = total balls for the base key.
	// For keys with no wickets at all, emit a single row with empty mode and zeros for wickets, balls = total.
	out := make([]db.WicketModeRow, 0, len(modeWkts))
	// Emit per-mode rows
	for mk, wkts := range modeWkts {
		base := wkAggKey{asof: mk.asof, fmt: mk.fmt, pid: mk.pid, ph: mk.ph}
		balls := totals[base]
		var per100 float64
		if balls > 0 {
			per100 = 100.0 * float64(wkts) / float64(balls)
		}
		out = append(out, db.WicketModeRow{
			AsOfDate:      mk.asof,
			FormatID:      mk.fmt,
			Scope:         "overall",
			ScopeID:       nil,
			PlayerID:      mk.pid,
			Phase:         mk.ph,
			Mode:          mk.mode,
			Balls:         balls,
			Wickets:       wkts,
			WicketsPer100: per100,
		})
	}
	// Emit empty-mode rows for bases with totals but no wickets
	for base, balls := range totals {
		if hasAnyWicket[base] {
			continue
		}
		out = append(out, db.WicketModeRow{
			AsOfDate: base.asof,
			FormatID: base.fmt,
			Scope:    "overall",
			ScopeID:  nil,
			PlayerID: base.pid,
			Phase:    base.ph,
			Mode:     "",
			Balls:    balls,
			Wickets:  0,
		})
	}
	return out
}

func canonicalMode(kind sql.NullString) string {
	if !kind.Valid {
		return ""
	}
	s := strings.ToLower(strings.TrimSpace(kind.String))
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, "-", "_")
	s = strings.ReplaceAll(s, "__", "_")
	// map common synonyms
	switch s {
	case "caught_and_bowled":
		return "caught"
	case "runout", "run_out":
		return "run_out"
	}
	return s
}

// guards for imports
var _ = fmt.Sprintf

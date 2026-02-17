package seqcalc

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/formats"
)

// spellsCalc computes bowling spell features split by phase and first-over vs later-overs.
type spellsCalc struct{}

func NewSpellsCalculator() Calculator { return spellsCalc{} }
func (spellsCalc) Name() Target       { return TargetSpells }

// evRowSpell is the local projection for spell aggregation.
type evRowSpell struct {
	MatchID     int64
	Innings     int
	Over        int
	BallSeq     int
	Phase       string
	IsLegal     bool
	BowlerID    sql.NullInt64
	RunsBatter  int
	RunsTotal   int
	ExtrasKind  sql.NullString
	PlayerOutID sql.NullInt64
	AsOf        time.Time
	FormatID    int
}

func (spellsCalc) Compute(ctx context.Context, params Params, dryRun bool) error {
	if dryRun {
		return nil
	}
	formatIDs := formats.MapFormatIDs(params.FormatCode)
	slog.Info("seqcalc.spells.query_start", slog.String("format", params.FormatCode), slog.Any("format_ids", formatIDs))
	ev, err := queryEventsForSpells(ctx, formatIDs)
	if err != nil {
		return err
	}
	if len(ev) == 0 {
		return nil
	}
	rows := aggregateSpells(ev)
	if len(rows) == 0 {
		return nil
	}
	return db.UpsertBowlingSpells(ctx, rows)
}

func queryEventsForSpells(ctx context.Context, formatIDs []int) ([]evRowSpell, error) {
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
		  be.bowler_id, be.runs_batter, be.runs_total, be.extras_kind, be.player_out_id,
    m.match_date, m.format_id
		FROM ball_event be
		JOIN match m ON m.match_id = be.match_id
		WHERE m.format_id IN (%s) AND m.match_date IS NOT NULL
		ORDER BY be.match_id, be.innings, be.ball_seq
	`, place)
	dr, err := db.Pool.Query(ctx, q, args...)
	if err != nil {
		slog.Error("seqcalc.spells.query_failed", slog.Any("format_ids", formatIDs), slog.Any("err", err))
		return nil, err
	}
	defer dr.Close()
	var out []evRowSpell
	for dr.Next() {
		var r evRowSpell
		if err := dr.Scan(&r.MatchID, &r.Innings, &r.Over, &r.BallSeq, &r.Phase,
			&r.IsLegal,
			&r.BowlerID, &r.RunsBatter, &r.RunsTotal, &r.ExtrasKind, &r.PlayerOutID,
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

// spellOverKey identifies an over for a bowler within an innings.
type spellOverKey struct {
	match int64
	inng  int
	bow   int64
	over  int
}

type overAgg struct {
	asof   string
	fmtID  int
	phase  string
	balls  int
	runs   int
	wkts   int
	dots   int
	bounds int
}

// aggregateSpells computes per-phase aggregates splitting first-over vs later-overs across spells.
func aggregateSpells(events []evRowSpell) []db.BowlingSpellRow {
	// Build per-over aggregates first.
	oAgg := map[spellOverKey]*overAgg{}
	for _, e := range events {
		if !e.BowlerID.Valid {
			continue
		}
		k := spellOverKey{match: e.MatchID, inng: e.Innings, bow: e.BowlerID.Int64, over: e.Over}
		o, ok := oAgg[k]
		if !ok {
			o = &overAgg{asof: e.AsOf.Format("2006-01-02"), fmtID: e.FormatID, phase: e.Phase}
			oAgg[k] = o
		}
		// Runs include illegal deliveries; balls count only legal
		o.runs += e.RunsTotal
		if e.IsLegal {
			o.balls++
			if e.RunsTotal == 0 {
				o.dots++
			}
			if e.RunsBatter == 4 || e.RunsBatter == 6 {
				o.bounds++
			}
			if e.PlayerOutID.Valid {
				o.wkts++
			}
		}
	}

	// For each (match, inng, bow), sort overs and detect spells.
	type perBow struct {
		match int64
		inng  int
		bow   int64
	}
	oversByBow := map[perBow][]int{}
	for k := range oAgg {
		pb := perBow{match: k.match, inng: k.inng, bow: k.bow}
		oversByBow[pb] = append(oversByBow[pb], k.over)
	}
	for pb := range oversByBow {
		list := oversByBow[pb]
		sort.Ints(list)
		oversByBow[pb] = list
	}

	// Accumulate into output keyed by (asof, fmt, player, phase)
	type outKey struct {
		asof  string
		fmt   int
		pid   int64
		phase string
	}
	acc := map[outKey]*db.BowlingSpellRow{}

	for pb, list := range oversByBow {
		// Identify spells as consecutive overs
		start := 0
		for i := 0; i < len(list); i++ {
			if i == len(list)-1 || list[i+1] != list[i]+1 {
				// Spell is list[start:i]
				for j := start; j <= i; j++ {
					o := oAgg[spellOverKey{match: pb.match, inng: pb.inng, bow: pb.bow, over: list[j]}]
					key := outKey{asof: o.asof, fmt: o.fmtID, pid: pb.bow, phase: o.phase}
					row := acc[key]
					if row == nil {
						row = &db.BowlingSpellRow{
							AsOfDate: key.asof,
							FormatID: key.fmt,
							PlayerID: key.pid,
							Phase:    key.phase,
							Scope:    "overall",
						}
						acc[key] = row
					}
					// Over-level contribution
					if j == start {
						row.Spells++ // count spells starting in this phase
						row.SpellOvers++
						row.FirstOversBalls += o.balls
						row.FirstOversRuns += o.runs
						row.FirstOversWickets += o.wkts
						row.FirstOversDots += o.dots
						row.FirstOversBoundaries += o.bounds
					} else {
						row.SpellOvers++
						row.LaterOversBalls += o.balls
						row.LaterOversRuns += o.runs
						row.LaterOversWickets += o.wkts
						row.LaterOversDots += o.dots
						row.LaterOversBoundaries += o.bounds
					}
				}
				start = i + 1
			}
		}
	}

	// Finalize economy rates and materialize slice
	out := make([]db.BowlingSpellRow, 0, len(acc))
	for _, r := range acc {
		if r.FirstOversBalls > 0 {
			r.FirstOverEcon = 6.0 * float64(r.FirstOversRuns) / float64(r.FirstOversBalls)
		}
		if r.LaterOversBalls > 0 {
			r.LaterOverEcon = 6.0 * float64(r.LaterOversRuns) / float64(r.LaterOversBalls)
		}
		out = append(out, *r)
	}
	return out
}

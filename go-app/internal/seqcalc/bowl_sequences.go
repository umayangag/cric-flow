package seqcalc

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

// overSummary is a minimal per-over rollup used to form consecutive A→B pairs.
type overSummary struct {
	matchID int64
	innings int
	over    int
	bowler  int64
	phase   string
	date    string
	format  int
	balls   int
	runs    int
	wickets int
	dots    int
}

// NewBowlSequencesCalculator returns the calculator for bowling sequence features.
func NewBowlSequencesCalculator() Calculator { return &bowlSequencesCalc{} }

type bowlSequencesCalc struct{}

func (b *bowlSequencesCalc) Name() Target { return TargetBowlSequences }

// Compute scans ball_event rows and aggregates over-to-over bowler A→B pairs within the same innings.
// For now, scope is 'overall' (scope_id=NULL). Implementation is format-aware via params.FormatCode.
func (b *bowlSequencesCalc) Compute(ctx context.Context, params Params, dryRun bool) error {
	if dryRun {
		return nil
	}
	formatID, err := formatIDFor(params.FormatCode)
	if err != nil {
		return err
	}
	// Query required fields ordered to reconstruct overs and compute basic stats for the next over (B)
	r, err := db.Query(ctx, `
		SELECT be.match_id, be.innings, be.over, be.ball, be.bowler_id, be.phase,
		       be.runs_total, (be.wicket_kind IS NOT NULL) AS is_wicket, (be.runs_total = 0) AS is_dot,
		       md.match_date, md.format_id
		FROM ball_event be
		JOIN match_details md ON md.match_id = be.match_id
		WHERE md.format_id = $1 AND be.is_legal = TRUE AND be.bowler_id IS NOT NULL
		ORDER BY be.match_id, be.innings, be.over, be.ball
	`, formatID)
	if err != nil {
		return err
	}
	defer r.Close()

	type keyOver struct {
		matchID int64
		innings int
		over    int
	}
	type row struct {
		bowlerID int64
		phase    string
		date     string // YYYY-MM-DD
		formatID int
		runs     int
		isWicket bool
		isDot    bool
	}
	overMap := make(map[keyOver][]row)
	var orderKeys []keyOver
	for r.Next() {
		var (
			mid             int64
			inn, over, ball int
			bowlerIDPtr     *int64
			phase           string
			runs            int
			isWicket        bool
			isDot           bool
			matchDate       time.Time
			fmtID           int
		)
		if err := r.Scan(&mid, &inn, &over, &ball, &bowlerIDPtr, &phase, &runs, &isWicket, &isDot, &matchDate, &fmtID); err != nil {
			return err
		}
		if bowlerIDPtr == nil {
			// should not happen due to WHERE, but guard
			continue
		}
		k := keyOver{matchID: mid, innings: inn, over: over}
		if _, ok := overMap[k]; !ok {
			overMap[k] = make([]row, 0, 6)
			orderKeys = append(orderKeys, k)
		}
		overMap[k] = append(overMap[k], row{
			bowlerID: *bowlerIDPtr,
			phase:    phase,
			date:     matchDate.Format("2006-01-02"),
			formatID: fmtID,
			runs:     runs,
			isWicket: isWicket,
			isDot:    isDot,
		})
	}
	// Sort order keys to ensure deterministic processing
	sort.Slice(orderKeys, func(i, j int) bool {
		if orderKeys[i].matchID != orderKeys[j].matchID {
			return orderKeys[i].matchID < orderKeys[j].matchID
		}
		if orderKeys[i].innings != orderKeys[j].innings {
			return orderKeys[i].innings < orderKeys[j].innings
		}
		return orderKeys[i].over < orderKeys[j].over
	})

	// Build an ordered list of over summaries per (match,innings)
	seq := make([]overSummary, 0, len(orderKeys))
	for _, k := range orderKeys {
		rows := overMap[k]
		if len(rows) == 0 {
			continue
		}
		bowler := rows[0].bowlerID
		phase := rows[0].phase
		date := rows[0].date
		fmtID := rows[0].formatID
		balls := len(rows)
		runs := 0
		wkts := 0
		dots := 0
		for _, rr := range rows {
			runs += rr.runs
			if rr.isWicket {
				wkts++
			}
			if rr.isDot {
				dots++
			}
		}
		seq = append(seq, overSummary{
			matchID: k.matchID,
			innings: k.innings,
			over:    k.over,
			bowler:  bowler,
			phase:   phase,
			date:    date,
			format:  fmtID,
			balls:   balls,
			runs:    runs,
			wickets: wkts,
			dots:    dots,
		})
	}

	pairs := pairConsecutiveOvers(seq)
	if len(pairs) == 0 {
		return nil
	}
	rows := make([]db.BowlSequenceRow, 0, len(pairs))
	for _, p := range pairs {
		rows = append(rows, db.BowlSequenceRow{
			AsOfDate:     p.date,
			FormatID:     p.format,
			Scope:        "overall",
			ScopeID:      nil,
			PrevBowlerID: p.prevBowler,
			BowlerID:     p.currBowler,
			Phase:        p.phase,
			OversPairs:   p.oversPairs,
			Balls:        p.balls,
			Runs:         p.runs,
			Wickets:      p.wickets,
			DotBalls:     p.dots,
		})
	}
	return db.UpsertBowlingSequences(ctx, rows)
}

// internal paired aggregate
type pairAgg struct {
	date       string
	format     int
	prevBowler int64
	currBowler int64
	phase      string
	oversPairs int
	balls      int
	runs       int
	wickets    int
	dots       int
}

type overKey struct {
	date   string
	format int
	phase  string
	prev   int64
	curr   int64
}

// pairConsecutiveOvers groups consecutive completed overs within the same match and innings into A→B pairs.
func pairConsecutiveOvers(seq []overSummary) []pairAgg {
	aggs := make(map[overKey]*pairAgg)
	var prev overSummary
	var hasPrev bool

	for _, o := range seq {
		if hasPrev && o.matchID == prev.matchID && o.innings == prev.innings {
			k := overKey{date: o.date, format: o.format, phase: o.phase, prev: prev.bowler, curr: o.bowler}

			agg, ok := aggs[k]
			if !ok {
				agg = &pairAgg{
					date:       k.date,
					format:     k.format,
					prevBowler: k.prev,
					currBowler: k.curr,
					phase:      k.phase,
				}
				aggs[k] = agg
			}

			agg.oversPairs++
			// Note: metrics are attributed to the current over (B) in the A→B pair
			agg.balls += o.balls
			agg.runs += o.runs
			agg.wickets += o.wickets
			agg.dots += o.dots
		}
		prev = o
		hasPrev = true
	}

	out := make([]pairAgg, 0, len(aggs))
	for _, agg := range aggs {
		out = append(out, *agg)
	}
	return out
}

// formatIDFor maps format code to numeric id consistent with seed order (0004 migration notes).
func formatIDFor(code string) (int, error) {
	switch strings.ToUpper(strings.TrimSpace(code)) {
	case "T20":
		return 3, nil
	case "T20I":
		return 4, nil
	case "ODI":
		return 2, nil
	case "TEST":
		return 1, nil
	default:
		return 0, fmt.Errorf("unknown format code: %s", code)
	}
}

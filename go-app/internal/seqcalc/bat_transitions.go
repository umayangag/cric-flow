package seqcalc

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

// batTransitionsCalc implements Calculator for batting transitions (T20-first).
// It reads ball_event joined with match_details to obtain as_of_date (match_date),
// aggregates A->B transitions by phase, and upserts rows idempotently.
//
// Notes:
// - For tests, prefer calling aggregateTransitions with synthetic events.
// - Compute requires a DB connection to be established via db.Connect by the caller.
// - For formats, we treat T20 and T20I the same (format_id in (3,4)).

type batTransitionsCalc struct{}

func NewBatTransitionsCalculator() Calculator { return batTransitionsCalc{} }

func (batTransitionsCalc) Name() Target { return TargetBatTransitions }

// bevent is a minimal projection of ball_event needed for transitions.
type bevent struct {
	MatchID     int64
	Innings     int
	BallSeq     int
	Phase       string
	StrikerID   sql.NullInt64
	RunsBatter  int
	RunsTotal   int
	WicketKind  sql.NullString
	PlayerOutID sql.NullInt64
	AsOf        time.Time // match_date
	FormatID    int
}

func (batTransitionsCalc) Compute(ctx context.Context, params Params, dryRun bool) error {
	if dryRun {
		return nil
	}
	// Map format codes to ids, default to T20/T20I if empty.
	formatIDs := []int{3, 4}
	switch normFormat(params.FormatCode) {
	case "T20":
		formatIDs = []int{3, 4}
	case "ODI":
		formatIDs = []int{2}
	case "TEST":
		formatIDs = []int{1}
	case "":
		// default: T20/T20I
		formatIDs = []int{3, 4}
	}

	if err := runBatTransitionsQueryAndUpsert(ctx, formatIDs); err != nil {
		return err
	}
	return nil
}

func normFormat(code string) string {
	if code == "" {
		return ""
	}
	s := code
	su := ""
	for i := 0; i < len(s); i++ {
		c := s[i]
		if 'a' <= c && c <= 'z' {
			su += string(c - 32)
		} else {
			su += string(c)
		}
	}
	if su == "T20I" {
		return "T20"
	}
	return su
}

func runBatTransitionsQueryAndUpsert(ctx context.Context, formatIDs []int) error {
	if db.Pool == nil {
		return fmt.Errorf("db not connected")
	}
	// Build IN list placeholders for small set
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
		  be.striker_id, be.runs_batter, be.runs_total, be.wicket_kind, be.player_out_id,
		  md.match_date, md.format_id
		FROM ball_event be
		JOIN match_details md ON md.match_id = be.match_id
		WHERE md.format_id IN (%s)
		ORDER BY be.match_id, be.innings, be.ball_seq
	`, place)
	rows, err := db.Pool.Query(ctx, q, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	var evs []bevent
	for rows.Next() {
		var e bevent
		if err := rows.Scan(&e.MatchID, &e.Innings, &e.BallSeq, &e.Phase,
			&e.StrikerID, &e.RunsBatter, &e.RunsTotal, &e.WicketKind, &e.PlayerOutID,
			&e.AsOf, &e.FormatID); err != nil {
			return err
		}
		// if match_date is NULL, skip to avoid violating NOT NULL as_of_date contract
		if e.AsOf.IsZero() {
			continue
		}
		evs = append(evs, e)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	out := aggregateTransitions(evs)
	if len(out) == 0 {
		return nil
	}
	return db.UpsertBattingTransitions(ctx, out)
}

// aggregateTransitions performs in-memory detection of A->B striker transitions and aggregates metrics by phase.
func aggregateTransitions(evs []bevent) []db.BatTransitionRow {
	// group by (match, innings)
	type key struct {
		match int64
		inng  int
	}
	byInng := map[key][]bevent{}
	for _, e := range evs {
		k := key{match: e.MatchID, inng: e.Innings}
		byInng[k] = append(byInng[k], e)
	}
	// output aggregation key: (as_of, format_id, prev, batter, phase)
	type akey struct {
		asOf string
		fmt  int
		prev int64
		bat  int64
		ph   string
	}
	sums := map[akey]*db.BatTransitionRow{}

	for k, seq := range byInng {
		_ = k
		// seq already ordered by ball_seq from query; if used in tests, callers should pre-order
		var prevBatter int64
		var currentStriker int64
		var haveStriker bool
		var curKey *akey
		for i := 0; i < len(seq); i++ {
			e := seq[i]
			if !e.StrikerID.Valid {
				continue
			}
			striker := e.StrikerID.Int64
			if !haveStriker {
				// first legal ball of innings -> initialize striker, no transition yet
				haveStriker = true
				currentStriker = striker
				curKey = nil
				continue
			}
			// If striker changed from previous ball, that means a transition to this striker.
			if striker != currentStriker {
				prevBatter = currentStriker
				currentStriker = striker
				// record/ensure entry for this A->B at this phase/as_of/format; stats should accrue starting on THIS ball
				ak := akey{asOf: seq[i].AsOf.Format("2006-01-02"), fmt: e.FormatID, prev: prevBatter, bat: striker, ph: e.Phase}
				row, ok := sums[ak]
				if !ok {
					row = &db.BatTransitionRow{
						AsOfDate:     ak.asOf,
						FormatID:     ak.fmt,
						Scope:        "overall",
						ScopeID:      0,
						PrevBatterID: ak.prev,
						BatterID:     ak.bat,
						Phase:        ak.ph,
						Balls:        0,
						Runs:         0,
						Dismissals:   0,
						Fours:        0,
						Sixes:        0,
					}
					sums[ak] = row
				}
				curKey = &ak
				// Accrue current delivery to the new striker's transition row
				row.Balls++
				row.Runs += e.RunsBatter
				if e.RunsBatter == 4 {
					row.Fours++
				}
				if e.RunsBatter == 6 {
					row.Sixes++
				}
				if e.PlayerOutID.Valid && e.PlayerOutID.Int64 == currentStriker {
					row.Dismissals++
				}
				continue
			}
			// accrue stats to the current striker's last A->B key if exists
			if curKey != nil {
				row := sums[*curKey]
				row.Balls++
				row.Runs += e.RunsBatter
				// boundaries: infer from runs_batter (simple proxy; dataset lacks 3/5 boundaries distinction)
				if e.RunsBatter == 4 {
					row.Fours++
				}
				if e.RunsBatter == 6 {
					row.Sixes++
				}
				// dismissal: if the player_out is the current striker, count dismissal
				if e.PlayerOutID.Valid && e.PlayerOutID.Int64 == currentStriker {
					row.Dismissals++
				}
			}
		}
	}
	// flatten
	out := make([]db.BatTransitionRow, 0, len(sums))
	for _, v := range sums {
		out = append(out, *v)
	}
	return out
}

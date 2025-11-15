package seqcalc

// Note: this file duplicates the implementation from player_windows.go but is named without
// the OS-specific suffix to ensure it is included on non-Windows builds. The previous filename
// inadvertently matched Go's OS build tag pattern (*_windows.go) and was excluded on darwin/linux.

import (
	"context"
	"database/sql"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

// NewPlayerWindowsCalculator returns the calculator for player rolling window features.
func NewPlayerWindowsCalculator() Calculator { return &playerWindowsCalc{} }

type playerWindowsCalc struct{}

func (p *playerWindowsCalc) Name() Target { return TargetPlayerWindows }

// minimal projection of ball_event joined with match_details for scopes.
type bEvent struct {
	MatchID     int64
	Innings     int
	BallSeq     int
	IsLegal     bool
	Phase       string
	StrikerID   sql.NullInt64
	BowlerID    sql.NullInt64
	RunsBatter  int
	RunsTotal   int
	ExtrasKind  sql.NullString
	WicketKind  sql.NullString
	PlayerOutID sql.NullInt64
	AsOf        time.Time
	FormatID    int
	Opposition  sql.NullInt64
	Venue       sql.NullInt64
	Season      sql.NullInt64
}

// package-level small helper types (Go doesn't allow type declarations within functions)
type pwBatAgg struct{ balls, runs, fours, sixes, dots, dismissals int }

type pwBowlAgg struct{ balls, runs, wickets, dotBalls, boundariesConceded, wideNB int }

type pwBatDeque struct{ items []pwBatAgg }

type pwBowlDeque struct{ items []pwBowlAgg }

type pwBatState struct {
	deques map[int]*pwBatDeque // horizon -> deque
	last   map[int]pwBatAgg    // horizon -> last aggregate in this innings
	phase  map[int]string      // horizon -> last phase at snapshot time
}

type pwBowlState struct {
	deques map[int]*pwBowlDeque
	last   map[int]pwBowlAgg
	phase  map[int]string
}

type pwInningsKey struct {
	match int64
	inn   int
}

func pushBat(d *pwBatDeque, a pwBatAgg, max int) {
	d.items = append(d.items, a)
	for len(d.items) > max {
		d.items = d.items[1:]
	}
}

func batSum(d *pwBatDeque) pwBatAgg {
	var s pwBatAgg
	for _, it := range d.items {
		s.balls += it.balls
		s.runs += it.runs
		s.fours += it.fours
		s.sixes += it.sixes
		s.dots += it.dots
		s.dismissals += it.dismissals
	}
	return s
}

func pushBowl(d *pwBowlDeque, a pwBowlAgg, max int) {
	d.items = append(d.items, a)
	for len(d.items) > max {
		d.items = d.items[1:]
	}
}

func bowlSum(d *pwBowlDeque) pwBowlAgg {
	var s pwBowlAgg
	for _, it := range d.items {
		s.balls += it.balls
		s.runs += it.runs
		s.wickets += it.wickets
		s.dotBalls += it.dotBalls
		s.boundariesConceded += it.boundariesConceded
		s.wideNB += it.wideNB
	}
	return s
}

func (p *playerWindowsCalc) Compute(ctx context.Context, params Params, dryRun bool) error {
	if dryRun {
		return nil
	}
	formatID, err := formatIDFor(params.FormatCode)
	if err != nil {
		return err
	}
	// Fetch required sequence with match context for scopes.
	r, err := db.Query(ctx, `
		SELECT be.match_id, be.innings, be.ball_seq, be.is_legal, be.phase,
		       be.striker_id, be.bowler_id, be.runs_batter, be.runs_total,
		       be.extras_kind, be.wicket_kind, be.player_out_id,
		       md.match_date, md.format_id, md.opposition_id, md.venue_id, md.season_id
		FROM ball_event be
		JOIN match_details md ON md.match_id = be.match_id
		WHERE md.format_id = $1
		ORDER BY be.match_id, be.innings, be.ball_seq
	`, formatID)
	if err != nil {
		return err
	}
	defer r.Close()

	// Horizon specs (confirmed): bat [5,10,15,20,25,30] ; bowl [5,10,15,20,25,30,35]
	batHorizons := []int{5, 10, 15, 20, 25, 30}
	bowlHorizons := []int{5, 10, 15, 20, 25, 30, 35}

	// Track per-innings state
	batByPlayer := make(map[pwInningsKey]map[int64]*pwBatState)
	bowlByPlayer := make(map[pwInningsKey]map[int64]*pwBowlState)
	var currK pwInningsKey
	var haveCurr bool
	var currDate string
	var currFmt int
	var currOpp, currVenue, currSeason *int64

	flushInnings := func() error {
		if !haveCurr {
			return nil
		}
		var rows []db.PlayerWindowRow
		// emit all accumulated latest snapshots for this innings across scopes
		emitScopes := func(scope string, scopeID *int64, role string, pid int64, phase string, h int, b *pwBatAgg, w *pwBowlAgg) {
			row := db.PlayerWindowRow{
				AsOfDate: currDate,
				FormatID: currFmt,
				Scope:    scope,
				ScopeID:  scopeID,
				PlayerID: pid,
				Role:     role,
				Phase:    phase,
				Horizon:  h,
			}
			if role == "bat" && b != nil {
				row.Balls = b.balls
				row.Runs = b.runs
				row.Dots = b.dots
				row.Fours = b.fours
				row.Sixes = b.sixes
				row.Dismissals = b.dismissals
				if b.balls > 0 {
					sr := float64(b.runs) * 100.0 / float64(b.balls)
					br := float64(b.fours+b.sixes) / float64(b.balls)
					dr := float64(b.dots) / float64(b.balls)
					dh := float64(b.dismissals) / float64(b.balls)
					row.SR = &sr
					row.BoundaryRate = &br
					row.DotRate = &dr
					row.DismissalHaz = &dh
				}
			}
			if role == "bowl" && w != nil {
				row.Balls = w.balls
				row.Runs = w.runs
				row.DotBalls = w.dotBalls
				row.Wickets = w.wickets
				row.BoundariesConceded = w.boundariesConceded
				row.WideNoBall = w.wideNB
				if w.balls > 0 {
					econ := float64(w.runs) * 6.0 / float64(w.balls)
					dr := float64(w.dotBalls) / float64(w.balls)
					wr := float64(w.wickets) / float64(w.balls)
					row.Econ = &econ
					row.DotRate = &dr
					row.WicketRate = &wr
				}
			}
			rows = append(rows, row)
		}
		if m := batByPlayer[currK]; m != nil {
			for pid, st := range m {
				for _, h := range batHorizons {
					if agg, ok := st.last[h]; ok {
						ph := st.phase[h]
						emitScopes("overall", nil, "bat", pid, ph, h, &agg, nil)
						if currOpp != nil {
							emitScopes("opposition", currOpp, "bat", pid, ph, h, &agg, nil)
						}
						if currVenue != nil {
							emitScopes("venue", currVenue, "bat", pid, ph, h, &agg, nil)
						}
						if currSeason != nil {
							emitScopes("season", currSeason, "bat", pid, ph, h, &agg, nil)
						}
					}
				}
			}
		}
		if m := bowlByPlayer[currK]; m != nil {
			for pid, st := range m {
				for _, h := range bowlHorizons {
					if agg, ok := st.last[h]; ok {
						ph := st.phase[h]
						emitScopes("overall", nil, "bowl", pid, ph, h, nil, &agg)
						if currOpp != nil {
							emitScopes("opposition", currOpp, "bowl", pid, ph, h, nil, &agg)
						}
						if currVenue != nil {
							emitScopes("venue", currVenue, "bowl", pid, ph, h, nil, &agg)
						}
						if currSeason != nil {
							emitScopes("season", currSeason, "bowl", pid, ph, h, nil, &agg)
						}
					}
				}
			}
		}
		if len(rows) == 0 {
			return nil
		}
		return db.UpsertPlayerWindows(ctx, rows)
	}

	for r.Next() {
		var e bEvent
		if err := r.Scan(&e.MatchID, &e.Innings, &e.BallSeq, &e.IsLegal, &e.Phase,
			&e.StrikerID, &e.BowlerID, &e.RunsBatter, &e.RunsTotal,
			&e.ExtrasKind, &e.WicketKind, &e.PlayerOutID,
			&e.AsOf, &e.FormatID, &e.Opposition, &e.Venue, &e.Season); err != nil {
			return err
		}
		k := pwInningsKey{match: e.MatchID, inn: e.Innings}
		if !haveCurr || k != currK {
			// flush previous innings before switching
			if err := flushInnings(); err != nil {
				return err
			}
			// reset accumulators for new innings context
			currK = k
			haveCurr = true
			currDate = e.AsOf.Format("2006-01-02")
			currFmt = e.FormatID
			if e.Opposition.Valid {
				v := e.Opposition.Int64
				currOpp = &v
			} else {
				currOpp = nil
			}
			if e.Venue.Valid {
				v := e.Venue.Int64
				currVenue = &v
			} else {
				currVenue = nil
			}
			if e.Season.Valid {
				v := e.Season.Int64
				currSeason = &v
			} else {
				currSeason = nil
			}
			if _, ok := batByPlayer[k]; !ok {
				batByPlayer[k] = make(map[int64]*pwBatState)
			}
			if _, ok := bowlByPlayer[k]; !ok {
				bowlByPlayer[k] = make(map[int64]*pwBowlState)
			}
		}
		// Only legal deliveries contribute to window sizes (horizons are in legal balls)
		if e.IsLegal {
			// Batting update
			if e.StrikerID.Valid {
				pid := e.StrikerID.Int64
				st, ok := batByPlayer[k][pid]
				if !ok {
					st = &pwBatState{deques: map[int]*pwBatDeque{}, last: map[int]pwBatAgg{}, phase: map[int]string{}}
					for _, h := range batHorizons {
						st.deques[h] = &pwBatDeque{}
					}
					batByPlayer[k][pid] = st
				}
				// build per-ball contribution
				ba := pwBatAgg{balls: 1, runs: e.RunsBatter}
				if e.RunsBatter == 4 {
					ba.fours = 1
				}
				if e.RunsBatter == 6 {
					ba.sixes = 1
				}
				if e.RunsTotal == 0 {
					ba.dots = 1
				}
				if e.WicketKind.Valid && e.PlayerOutID.Valid && e.PlayerOutID.Int64 == pid {
					ba.dismissals = 1
				}
				for _, h := range batHorizons {
					pushBat(st.deques[h], ba, h)
					s := batSum(st.deques[h])
					st.last[h] = s
					st.phase[h] = e.Phase
				}
			}
			// Bowling update
			if e.BowlerID.Valid {
				pid := e.BowlerID.Int64
				st, ok := bowlByPlayer[k][pid]
				if !ok {
					st = &pwBowlState{deques: map[int]*pwBowlDeque{}, last: map[int]pwBowlAgg{}, phase: map[int]string{}}
					for _, h := range bowlHorizons {
						st.deques[h] = &pwBowlDeque{}
					}
					bowlByPlayer[k][pid] = st
				}
				wa := pwBowlAgg{balls: 1, runs: e.RunsTotal}
				if e.RunsTotal == 0 {
					wa.dotBalls = 1
				}
				if e.RunsBatter == 4 || e.RunsBatter == 6 {
					wa.boundariesConceded = 1
				}
				if e.WicketKind.Valid && e.PlayerOutID.Valid {
					wa.wickets = 1
				}
				// Note: wides/no-balls are illegal and excluded from horizon windows here; wide_nb remains 0.
				for _, h := range bowlHorizons {
					pushBowl(st.deques[h], wa, h)
					s := bowlSum(st.deques[h])
					st.last[h] = s
					st.phase[h] = e.Phase
				}
			}
		}
	}
	if err := flushInnings(); err != nil {
		return err
	}
	return nil
}

package seqcalc

import (
	"context"
	"database/sql"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/formats"
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

// makeBatAgg constructs a batting aggregate contribution for a single legal ball
// for the given striker. Mirrors inline logic to avoid behavior changes.
func makeBatAgg(e bEvent, strikerID int64) pwBatAgg {
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
	if e.WicketKind.Valid && e.PlayerOutID.Valid && e.PlayerOutID.Int64 == strikerID {
		ba.dismissals = 1
	}
	return ba
}

// makeBowlAgg constructs a bowling aggregate contribution for a single legal ball
// for the current bowler. Mirrors inline logic to avoid behavior changes.
func makeBowlAgg(e bEvent) pwBowlAgg {
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
	// Note: wides/no-balls are illegal and excluded from horizon windows here; wideNB remains 0.
	return wa
}

// updateBatState pushes the per-ball batting aggregate into all horizons and
// updates the cached last aggregates and phase.
func updateBatState(st *pwBatState, e bEvent, horizons []int, ballAgg pwBatAgg) {
	for _, h := range horizons {
		pushBat(st.deques[h], ballAgg, h)
		s := batSum(st.deques[h])
		st.last[h] = s
		st.phase[h] = e.Phase
	}
}

// updateBowlState pushes the per-ball bowling aggregate into all horizons and
// updates the cached last aggregates and phase.
func updateBowlState(st *pwBowlState, e bEvent, horizons []int, ballAgg pwBowlAgg) {
	for _, h := range horizons {
		pushBowl(st.deques[h], ballAgg, h)
		s := bowlSum(st.deques[h])
		st.last[h] = s
		st.phase[h] = e.Phase
	}
}

// buildRowsForInnings assembles PlayerWindowRow entries for the current innings
// from the latest aggregates stored in batting and bowling states. It mirrors the
// inline logic previously present in flushInnings to avoid behavior changes.
func buildRowsForInnings(
	asOfDate string,
	formatID int,
	opp *int64,
	venue *int64,
	season *int64,
	batHorizons []int,
	bowlHorizons []int,
	batStates map[int64]*pwBatState,
	bowlStates map[int64]*pwBowlState,
) []db.PlayerWindowRow {
	var rows []db.PlayerWindowRow

	emit := func(scope string, scopeID *int64, role string, pid int64, phase string, h int, b *pwBatAgg, w *pwBowlAgg) {
		row := db.PlayerWindowRow{
			AsOfDate: asOfDate,
			FormatID: formatID,
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

	for pid, st := range batStates {
		for _, h := range batHorizons {
			if agg, ok := st.last[h]; ok {
				ph := st.phase[h]
				emit("overall", nil, "bat", pid, ph, h, &agg, nil)
				if opp != nil {
					emit("opposition", opp, "bat", pid, ph, h, &agg, nil)
				}
				if venue != nil {
					emit("venue", venue, "bat", pid, ph, h, &agg, nil)
				}
				if season != nil {
					emit("season", season, "bat", pid, ph, h, &agg, nil)
				}
			}
		}
	}

	for pid, st := range bowlStates {
		for _, h := range bowlHorizons {
			if agg, ok := st.last[h]; ok {
				ph := st.phase[h]
				emit("overall", nil, "bowl", pid, ph, h, nil, &agg)
				if opp != nil {
					emit("opposition", opp, "bowl", pid, ph, h, nil, &agg)
				}
				if venue != nil {
					emit("venue", venue, "bowl", pid, ph, h, nil, &agg)
				}
				if season != nil {
					emit("season", season, "bowl", pid, ph, h, nil, &agg)
				}
			}
		}
	}

	return rows
}

// upsertPlayerWindowRows writes the assembled rows using the existing repo function.
// Kept as a thin wrapper to aid testability and readability.
func upsertPlayerWindowRows(ctx context.Context, rows []db.PlayerWindowRow) error {
	if len(rows) == 0 {
		return nil
	}
	return db.UpsertPlayerWindows(ctx, rows)
}

func pushBat(d *pwBatDeque, a pwBatAgg, maxWindow int) {
	d.items = append(d.items, a)
	for len(d.items) > maxWindow {
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

func pushBowl(d *pwBowlDeque, a pwBowlAgg, maxWindow int) {
	d.items = append(d.items, a)
	for len(d.items) > maxWindow {
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
	formatID, err := formats.IDForCode(params.FormatCode)
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
		rows := buildRowsForInnings(
			currDate, currFmt, currOpp, currVenue, currSeason,
			batHorizons, bowlHorizons,
			batByPlayer[currK], bowlByPlayer[currK],
		)
		return upsertPlayerWindowRows(ctx, rows)
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
				// build per-ball contribution via helper
				ba := makeBatAgg(e, pid)
				updateBatState(st, e, batHorizons, ba)
			}
			// Bowling update
			if e.BowlerID.Valid {
				pid := e.BowlerID.Int64
				st, ok := bowlByPlayer[k][pid]
				if !ok {
					st = &pwBowlState{
						deques: map[int]*pwBowlDeque{},
						last:   map[int]pwBowlAgg{},
						phase:  map[int]string{},
					}
					for _, h := range bowlHorizons {
						st.deques[h] = &pwBowlDeque{}
					}
					bowlByPlayer[k][pid] = st
				}
				wa := makeBowlAgg(e)
				updateBowlState(st, e, bowlHorizons, wa)
			}
		}
	}
	if err := flushInnings(); err != nil {
		return err
	}
	return nil
}

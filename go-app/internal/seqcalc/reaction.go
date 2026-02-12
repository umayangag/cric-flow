package seqcalc

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/formats"
)

// reactionCalc computes immediate next-ball outcomes conditioned on the prior event
// for both batter and bowler streams. T20-first (format_id in (3,4)).
type reactionCalc struct{}

func NewReactionCalculator() Calculator { return reactionCalc{} }
func (reactionCalc) Name() Target       { return TargetReaction }

// minimal projection of ball_event for reaction/dot logic
type evRow struct {
	MatchID     int64
	Innings     int
	BallSeq     int
	Phase       string
	IsLegal     bool
	StrikerID   sql.NullInt64
	BowlerID    sql.NullInt64
	RunsBatter  int
	RunsTotal   int
	ExtrasKind  sql.NullString
	PlayerOutID sql.NullInt64
	AsOf        time.Time
	FormatID    int
}

func (reactionCalc) Compute(ctx context.Context, params Params, dryRun bool) error {
	if dryRun {
		return nil
	}
	fids := formats.MapFormatIDs(params.FormatCode)
	rows, err := queryEvents(ctx, fids)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	// Aggregate for battter and bowler streams
	bat := aggregateReaction(rows, true)
	bowl := aggregateReaction(rows, false)
	// Upsert
	if err := db.UpsertEventReactions(ctx, append(bat, bowl...)); err != nil {
		return err
	}
	return nil
}

// Removed local helpers in favor of shared formats package.

func queryEvents(ctx context.Context, formatIDs []int) ([]evRow, error) {
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
		  be.striker_id, be.bowler_id, be.runs_batter, be.runs_total, be.extras_kind, be.player_out_id,
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
	var out []evRow
	for dr.Next() {
		var r evRow
		if err := dr.Scan(&r.MatchID, &r.Innings, &r.BallSeq, &r.Phase,
			&r.IsLegal,
			&r.StrikerID, &r.BowlerID, &r.RunsBatter, &r.RunsTotal, &r.ExtrasKind, &r.PlayerOutID,
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

// reaction stream key per player (match, innings, player)
type reactionStreamKey struct {
	match int64
	inng  int
	pid   int64
}

// aggregate key per output row
type reactionAggKey struct {
	asof           string
	fmt            int
	pid            int64
	role, prev, ph string
}

// aggregateReaction computes reaction rows. If forBat is true, use striker streams; else bowler streams.
func aggregateReaction(events []evRow, forBat bool) []db.EventReactionRow {
	// group events per stream
	streams := map[reactionStreamKey][]evRow{}
	for _, e := range events {
		var pid int64
		if forBat {
			if !e.StrikerID.Valid {
				continue
			}
			pid = e.StrikerID.Int64
		} else {
			if !e.BowlerID.Valid {
				continue
			}
			pid = e.BowlerID.Int64
		}
		k := reactionStreamKey{match: e.MatchID, inng: e.Innings, pid: pid}
		streams[k] = append(streams[k], e)
	}
	// aggregate per (as_of, format, player, role, prev_event, phase)
	sums := map[reactionAggKey]*db.EventReactionRow{}
	for _, seq := range streams {
		var prevEvent string
		var havePrev bool
		curPID := streamPID(seq[0], forBat)
		role := roleName(forBat)
		for i := 0; i < len(seq); i++ {
			e := seq[i]
			curPrev := ""
			if havePrev {
				curPrev = prevEvent
			}
			// On each delivery after we have a prev, record next-ball outcome under that prev
			if curPrev != "" {
				ak := reactionAggKey{
					asof: e.AsOf.Format("2006-01-02"),
					fmt:  e.FormatID,
					pid:  curPID,
					role: role,
					prev: curPrev,
					ph:   e.Phase,
				}
				row := ensureEventReaction(sums, ak)
				row.Balls++ // denominator includes illegal deliveries per approval
				row.Runs += e.RunsTotal
				// batting-side counts
				if forBat {
					if e.RunsBatter == 4 || e.RunsBatter == 6 {
						row.Boundaries++
					}
					if e.PlayerOutID.Valid && e.StrikerID.Valid && e.PlayerOutID.Int64 == e.StrikerID.Int64 {
						row.Dismissals++
					}
				} else {
					// bowling-side counts
					if e.RunsTotal == 0 {
						row.DotBalls++
					}
					if e.RunsBatter == 4 || e.RunsBatter == 6 {
						row.BoundariesConceded++
					}
					if e.PlayerOutID.Valid {
						row.Wickets++
					}
				}
			}
			// compute prev_event for next iteration from this delivery
			prevEvent = classifyEvent(e)
			havePrev = prevEvent != ""
		}
	}
	// flatten
	out := make([]db.EventReactionRow, 0, len(sums))
	for _, v := range sums {
		out = append(out, *v)
	}
	return out
}

func streamPID(e evRow, forBat bool) int64 {
	if forBat {
		if e.StrikerID.Valid {
			return e.StrikerID.Int64
		}
	}
	if !forBat {
		if e.BowlerID.Valid {
			return e.BowlerID.Int64
		}
	}
	return 0
}

func roleName(forBat bool) string {
	if forBat {
		return "bat"
	}
	return "bowl"
}

func ensureEventReaction(m map[reactionAggKey]*db.EventReactionRow, ak reactionAggKey) *db.EventReactionRow {
	if r, ok := m[ak]; ok {
		return r
	}
	r := &db.EventReactionRow{
		AsOfDate:  ak.asof,
		FormatID:  ak.fmt,
		Scope:     "overall",
		ScopeID:   0,
		PlayerID:  ak.pid,
		Role:      ak.role,
		PrevEvent: ak.prev,
		Phase:     ak.ph,
	}
	m[ak] = r
	return r
}

// classifyEvent maps a delivery to a prev_event string per approved vocabulary and priority.
func classifyEvent(e evRow) string {
	// priority: wicket > wide/no_ball > boundary number > dot > bye/leg_bye
	if e.PlayerOutID.Valid {
		return "wicket"
	}
	if e.ExtrasKind.Valid {
		k := e.ExtrasKind.String
		if k == "wide" {
			return "wide"
		}
		if k == "no_ball" {
			return "no_ball"
		}
		if k == "bye" {
			return "bye"
		}
		if k == "leg_bye" {
			return "leg_bye"
		}
	}
	switch e.RunsBatter {
	case 4:
		return "4"
	case 6:
		return "6"
	case 3:
		return "3"
	case 2:
		return "2"
	case 1:
		return "1"
	}
	if e.RunsTotal == 0 {
		return "dot"
	}
	return "" // fall back to empty (ignored)
}

package seqcalc

import (
	"context"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/formats"
)

// dotStreaksCalc computes distributions of next-ball outcomes after k consecutive dots.
type dotStreaksCalc struct{}

func NewDotStreaksCalculator() Calculator { return dotStreaksCalc{} }
func (dotStreaksCalc) Name() Target       { return TargetDotStreaks }

func (dotStreaksCalc) Compute(ctx context.Context, params Params, dryRun bool) error {
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
	bat := aggregateDotStreaks(rows, true)
	bowl := aggregateDotStreaks(rows, false)
	return db.UpsertDotStreaks(ctx, append(bat, bowl...))
}

// stream and aggregation keys
type dotStreamKey struct {
	match int64
	inng  int
	pid   int64
}
type dotAggKey struct {
	asof string
	fmt  int
	pid  int64
	role string
	k    int
	ph   string
}

// aggregateDotStreaks groups events by player stream and accumulates next-ball distributions keyed by k-dot streak.
func aggregateDotStreaks(events []evRow, forBat bool) []db.DotStreakRow {
	// group events per stream
	streams := map[dotStreamKey][]evRow{}
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
		k := dotStreamKey{match: e.MatchID, inng: e.Innings, pid: pid}
		streams[k] = append(streams[k], e)
	}
	// output aggregation key
	sums := map[dotAggKey]*db.DotStreakRow{}
	for _, seq := range streams {
		kDots := 0
		role := roleName(forBat)
		pid := streamPID(seq[0], forBat)
		for i := 0; i < len(seq); i++ {
			e := seq[i]
			if kDots < 0 {
				kDots = 0
			}
			if kDots > 6 {
				kDots = 6
			}
			ak := dotAggKey{
				asof: e.AsOf.Format("2006-01-02"),
				fmt:  e.FormatID,
				pid:  pid,
				role: role,
				k:    kDots,
				ph:   e.Phase,
			}
			row := ensureDotStreak(sums, ak)
			row.Balls++ // denominator includes illegal next deliveries per approval
			row.RunsNextTotal += e.RunsTotal
			// categories
			if e.RunsBatter == 4 || e.RunsBatter == 6 {
				row.NextBoundary++
			}
			if e.RunsBatter == 1 {
				row.NextSingle++
			}
			if e.PlayerOutID.Valid {
				row.NextWicket++
			}
			if e.ExtrasKind.Valid &&
				(e.ExtrasKind.String == "wide" || e.ExtrasKind.String == "no_ball" || e.ExtrasKind.String == "bye" || e.ExtrasKind.String == "leg_bye") {
				row.NextExtra++
			}
			if e.RunsTotal == 0 {
				row.NextDot++
			}
			// update kDots using current delivery per legal dot rule
			if e.IsLegal && e.RunsTotal == 0 {
				kDots++
			} else if e.IsLegal {
				kDots = 0
			}
		}
	}
	out := make([]db.DotStreakRow, 0, len(sums))
	for _, v := range sums {
		out = append(out, *v)
	}
	return out
}

func ensureDotStreak(m map[dotAggKey]*db.DotStreakRow, ak dotAggKey) *db.DotStreakRow {
	if r, ok := m[ak]; ok {
		return r
	}
	r := &db.DotStreakRow{
		AsOfDate: ak.asof,
		FormatID: ak.fmt,
		Scope:    "overall",
		ScopeID:  0,
		PlayerID: ak.pid,
		Role:     ak.role,
		K:        ak.k,
		Phase:    ak.ph,
	}
	m[ak] = r
	return r
}

// date helper used in tests

package seqcalc

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/formats"
)

// disciplineCalc computes extras discipline metrics for bowlers by phase.
type disciplineCalc struct{}

func NewDisciplineCalculator() Calculator { return disciplineCalc{} }
func (disciplineCalc) Name() Target       { return TargetDiscipline }

// Compute reads ball_event via queryEvents (T20-first) and aggregates extras discipline per bowler.
func (disciplineCalc) Compute(ctx context.Context, params Params, dryRun bool) error {
	if dryRun {
		return nil
	}
	formatIDs := formats.MapFormatIDs(params.FormatCode)
	events, err := queryEvents(ctx, formatIDs)
	if err != nil {
		return err
	}
	if len(events) == 0 {
		return nil
	}
	rows := aggregateDiscipline(events)
	if len(rows) == 0 {
		return nil
	}
	return db.UpsertExtrasDiscipline(ctx, rows)
}

// disciplineAggKey is the grouping key for extras discipline rows.
// We snapshot by (as_of_date, format_id, bowler_id, phase) with scope overall.
type disciplineAggKey struct {
	asof   string
	format int
	bowler int64
	phase  string
}

type dsum struct {
	ballsBowled int // legal balls only
	runs        int
	wickets     int
	wides       int
	noBalls     int
	byes        int
	legByes     int
	penalty     int
}

// aggregateDiscipline converts event rows into ExtrasDisciplineRow slices (idempotent-friendly counts only).
func aggregateDiscipline(events []evRow) []db.ExtrasDisciplineRow {
	m := map[disciplineAggKey]*dsum{}
	for _, e := range events {
		if !e.BowlerID.Valid {
			continue
		}
		ak := disciplineAggKey{
			asof:   e.AsOf.Format("2006-01-02"),
			format: e.FormatID,
			bowler: e.BowlerID.Int64,
			phase:  e.Phase,
		}
		s := m[ak]
		if s == nil {
			s = &dsum{}
			m[ak] = s
		}
		// Totals
		s.runs += e.RunsTotal
		if e.PlayerOutID.Valid {
			s.wickets++
		}
		// Legal balls contribute to balls_bowled
		if e.IsLegal {
			s.ballsBowled++
		}
		// Extras classification independent of legality
		if e.ExtrasKind.Valid {
			switch e.ExtrasKind.String {
			case "wide":
				s.wides++
			case "no_ball":
				s.noBalls++
			case "bye":
				s.byes++
			case "leg_bye":
				s.legByes++
			default:
				// Penalty or other extras (rare); if RunsTotal>0 and not covered above we can treat as penalty
				// We avoid double-counting: only increment penalty for explicit unknown extras
				if e.ExtrasKind.String == "penalty" {
					s.penalty++
				}
			}
		}
	}
	// Flatten to rows
	out := make([]db.ExtrasDisciplineRow, 0, len(m))
	for k, s := range m {
		overs := 0
		if s.ballsBowled > 0 {
			overs = s.ballsBowled / 6
		}
		extraTotal := s.wides + s.noBalls + s.byes + s.legByes + s.penalty
		var wpo, nbpo, expo float64
		if overs > 0 {
			wpo = float64(s.wides) / float64(overs)
			nbpo = float64(s.noBalls) / float64(overs)
			expo = float64(extraTotal) / float64(overs)
		}
		row := db.ExtrasDisciplineRow{
			AsOfDate:       k.asof,
			FormatID:       k.format,
			Scope:          "overall",
			ScopeID:        nil,
			PlayerID:       k.bowler,
			Phase:          k.phase,
			Overs:          overs,
			BallsBowled:    s.ballsBowled,
			RunsConceded:   s.runs,
			Wickets:        s.wickets,
			Wides:          s.wides,
			NoBalls:        s.noBalls,
			Byes:           s.byes,
			LegByes:        s.legByes,
			PenaltyRuns:    s.penalty,
			ExtrasTotal:    extraTotal,
			WidesPerOver:   wpo,
			NoBallsPerOver: nbpo,
			ExtrasPerOver:  expo,
		}
		out = append(out, row)
	}
	return out
}

// compile-time guards for imports
var (
	_ = sql.NullInt64{}
	_ = fmt.Sprintf
)

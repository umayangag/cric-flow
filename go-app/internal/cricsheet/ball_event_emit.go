package cricsheet

import (
	"context"
	"log/slog"
	"strings"

	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/phase"
)

// playerIDResolver is the slice of match identity that ball events need: every name on a
// delivery belongs to a person the match file's registry can name.
type playerIDResolver interface {
	PlayerID(ctx context.Context, name string) (int64, error)
}

// BuildBallEventRows builds ball_event rows for every innings the match played. It
// numbers innings the way the scorecard aggregates do -- both read Match.PlayedInnings --
// so a delivery's innings number and its match_inning row always describe the same
// innings.
func BuildBallEventRows(
	ctx context.Context,
	identity playerIDResolver,
	m *Match,
	formatID int,
	matchID int64,
) ([]db.BallEventRow, error) {
	var allRows []db.BallEventRow
	for i, inng := range m.PlayedInnings() {
		inningNo := i + 1
		// Pre-compute total legal deliveries in innings for phase clamping
		totalLegal := 0
		for _, over := range inng.Overs {
			for _, d := range over.Deliveries {
				if d.Extras.Wides == 0 && d.Extras.NoBalls == 0 {
					totalLegal++
				}
			}
		}
		if totalLegal == 0 {
			continue
		}
		// Second pass: build rows with ball_seq
		inningRows := make([]db.BallEventRow, 0, totalLegal+8)
		ballSeq := 0
		for _, over := range inng.Overs {
			overNo := over.Over
			ballNo := 0
			for _, d := range over.Deliveries {
				ballNo++
				legal := (d.Extras.Wides == 0 && d.Extras.NoBalls == 0)
				if legal {
					ballSeq++
				}
				// Resolve IDs (best-effort; keep nils on error)
				var strikerID, nonStrikerID, bowlerID *int64
				if s := strings.TrimSpace(d.Batter); s != "" {
					if id, err := identity.PlayerID(ctx, s); err == nil {
						strikerID = &id
					}
				}
				if s := strings.TrimSpace(d.NonStriker); s != "" {
					if id, err := identity.PlayerID(ctx, s); err == nil {
						nonStrikerID = &id
					}
				}
				if s := strings.TrimSpace(d.Bowler); s != "" {
					if id, err := identity.PlayerID(ctx, s); err == nil {
						bowlerID = &id
					} else {
						slog.Error("get/create bowler failed", slog.String("name", s), slog.Any("err", err))
					}
				}
				extrasKind := extrasKindOf(d.Extras)
				// wicket info (first only)
				var wicketKind *string
				var playerOutID *int64
				if d.Wickets != nil && len(*d.Wickets) > 0 {
					wk := strings.TrimSpace((*d.Wickets)[0].Kind)
					if wk != "" {
						wicketKind = &wk
					}
					name := strings.TrimSpace((*d.Wickets)[0].PlayerOut)
					if name != "" {
						if id, err := identity.PlayerID(ctx, name); err == nil {
							playerOutID = &id
						} else {
							slog.Error("get/create player failed", slog.String("name", name), slog.Any("err", err))
						}
					}
				}
				phaseName := phase.PhaseFor(int(formatID), ballSeq, totalLegal)
				inningRows = append(inningRows, db.BallEventRow{
					MatchID:      matchID,
					Innings:      inningNo,
					Over:         overNo,
					Ball:         ballNo,
					BallSeq:      ballSeq,
					IsLegal:      legal,
					Phase:        phaseName,
					StrikerID:    strikerID,
					NonStrikerID: nonStrikerID,
					BowlerID:     bowlerID,
					RunsBatter:     d.Runs.Batter,
					RunsExtras:     d.Runs.Extras,
					RunsTotal:      d.Runs.Total,
					ExtrasWides:    d.Extras.Wides,
					ExtrasNoBalls:  d.Extras.NoBalls,
					ExtrasByes:     d.Extras.Byes,
					ExtrasLegByes:  d.Extras.LegByes,
					ExtrasPenalty:  d.Extras.Penalty,
					ExtrasKind:     extrasKind,
					WicketKind:     wicketKind,
					PlayerOutID:    playerOutID,
				})
			}
		}
		allRows = append(allRows, inningRows...)
	}
	return allRows, nil
}

// extrasKindOf names one kind of extra for ball_event.extras_kind, by precedence: wide,
// then no-ball, then leg-bye, bye and penalty. It is a summary and a lossy one -- a
// no-ball with leg-byes off it reads as 'no_ball' alone -- which is why the row also
// carries every kind in the extras_* columns. Nil when the delivery had no extras.
func extrasKindOf(extras ExtrasBreakdown) *string {
	var kind string
	switch {
	case extras.Wides > 0:
		kind = "wide"
	case extras.NoBalls > 0:
		kind = "no_ball"
	case extras.LegByes > 0:
		kind = "leg_bye"
	case extras.Byes > 0:
		kind = "bye"
	case extras.Penalty > 0:
		kind = "penalty"
	default:
		return nil
	}
	return &kind
}

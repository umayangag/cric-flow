package cricsheet

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/phase"
)

// EmitBallEvents emits ball_event rows
// It computes is_legal, maintains a legal-only ball_seq per innings, and assigns phase via phase.PhaseFor.
func EmitBallEvents(ctx context.Context, m *Match, formatID int, matchID int64) error {
	for i, inng := range m.Innings {
		inningNo := i + 1
		// Pre-compute total legal deliveries in innings for phase clamping
		totalLegal := 0
		for _, over := range inng.Overs {
			for _, d := range over.Deliveries {
				wides := d.Extras["wides"]
				noballs := d.Extras["noballs"]
				if wides == 0 && noballs == 0 {
					totalLegal++
				}
			}
		}
		if totalLegal == 0 {
			continue
		}
		// Second pass: build rows with ball_seq
		rows := make([]db.BallEventRow, 0, totalLegal+8)
		ballSeq := 0
		for _, over := range inng.Overs {
			overNo := over.Over
			ballNo := 0
			for _, d := range over.Deliveries {
				ballNo++
				wides := d.Extras["wides"]
				noballs := d.Extras["noballs"]
				legal := (wides == 0 && noballs == 0)
				if legal {
					ballSeq++
				}
				// Resolve IDs (best-effort; keep nils on error)
				var strikerID, nonStrikerID, bowlerID *int64
				if s := strings.TrimSpace(d.Batter); s != "" {
					if id, err := cricDB.GetOrCreateByName(ctx, s); err == nil {
						strikerID = &id
					}
				}
				if s := strings.TrimSpace(d.NonStriker); s != "" {
					if id, err := cricDB.GetOrCreateByName(ctx, s); err == nil {
						nonStrikerID = &id
					}
				}
				if s := strings.TrimSpace(d.Bowler); s != "" {
					if id, err := cricDB.GetOrCreateByName(ctx, s); err == nil {
						bowlerID = &id
					} else {
						slog.Error("get/create bowler failed", slog.String("name", s), slog.Any("err", err))
					}
				}
				// extras kind (prefer wide/no_ball; else leg_bye/bye/penalty when present)
				var extrasKind *string
				if wides > 0 {
					v := "wide"
					extrasKind = &v
				} else if noballs > 0 {
					v := "no_ball"
					extrasKind = &v
				} else if v, ok := d.Extras["legbyes"]; ok && v > 0 {
					s := "leg_bye"
					extrasKind = &s
				} else if v, ok := d.Extras["byes"]; ok && v > 0 {
					s := "bye"
					extrasKind = &s
				} else if v, ok := d.Extras["penalty"]; ok && v > 0 {
					s := "penalty"
					extrasKind = &s
				}
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
						if id, err := cricDB.GetOrCreateByName(ctx, name); err == nil {
							playerOutID = &id
						} else {
							slog.Error("get/create player failed", slog.String("name", name), slog.Any("err", err))
						}
					}
				}
				phaseName := phase.PhaseFor(int(formatID), ballSeq, totalLegal)
				rows = append(rows, db.BallEventRow{
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
					RunsBatter:   d.Runs.Batter,
					RunsExtras:   d.Runs.Extras,
					RunsTotal:    d.Runs.Total,
					ExtrasKind:   extrasKind,
					WicketKind:   wicketKind,
					PlayerOutID:  playerOutID,
				})
			}
		}
		if err := insertBallEventsFn(ctx, rows); err != nil {
			slog.Error("failed to insert ball_event rows", slog.Any("err", err))
			return fmt.Errorf("failed to insert ball events for inning %d: %w", inningNo, err)
		}
	}
	return nil
}

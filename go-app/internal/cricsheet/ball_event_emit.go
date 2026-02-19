package cricsheet

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/phase"
)

// BuildBallEventRows builds ball_event rows for all innings. Used by both EmitBallEvents and transactional import.
func BuildBallEventRows(ctx context.Context, m *Match, formatID int, matchID int64) ([]db.BallEventRow, error) {
	cache := db.GetGlobalCache()
	var allRows []db.BallEventRow
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
		inningRows := make([]db.BallEventRow, 0, totalLegal+8)
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
				// Resolve IDs (best-effort; use cache; keep nils on error)
				var strikerID, nonStrikerID, bowlerID *int64
				if s := strings.TrimSpace(d.Batter); s != "" {
					if id, err := cache.GetPlayerID(ctx, s); err == nil {
						strikerID = &id
					}
				}
				if s := strings.TrimSpace(d.NonStriker); s != "" {
					if id, err := cache.GetPlayerID(ctx, s); err == nil {
						nonStrikerID = &id
					}
				}
				if s := strings.TrimSpace(d.Bowler); s != "" {
					if id, err := cache.GetPlayerID(ctx, s); err == nil {
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
						if id, err := cache.GetPlayerID(ctx, name); err == nil {
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
					RunsBatter:   d.Runs.Batter,
					RunsExtras:   d.Runs.Extras,
					RunsTotal:    d.Runs.Total,
					ExtrasKind:   extrasKind,
					WicketKind:   wicketKind,
					PlayerOutID:  playerOutID,
				})
			}
		}
		allRows = append(allRows, inningRows...)
	}
	return allRows, nil
}

// EmitBallEvents emits ball_event rows using the default inserter.
func EmitBallEvents(ctx context.Context, m *Match, formatID int, matchID int64) error {
	rows, err := BuildBallEventRows(ctx, m, formatID, matchID)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	if err := insertBallEventsFn(ctx, rows); err != nil {
		slog.Error("failed to insert ball_event rows", slog.Any("err", err))
		return fmt.Errorf("failed to insert ball events: %w", err)
	}
	return nil
}

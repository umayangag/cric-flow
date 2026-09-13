package cricsheet

import (
	"context"
	"log/slog"
	"strings"

	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/phase"
	"github.com/umayangag/cric-flow/go-app/internal/wicketkinds"
)

// playerIDResolver is the slice of match identity that ball events need: every name on a
// delivery belongs to a person the match file's registry can name.
type playerIDResolver interface {
	PlayerID(ctx context.Context, name string) (int64, error)
}

// BallEvents is what a match's deliveries become: one ball_event row per delivery and
// one ball_event_wicket row per wicket on it.
type BallEvents struct {
	Deliveries []db.BallEventRow
	Wickets    []db.BallEventWicketRow
}

// BuildBallEventRows builds ball_event rows for every innings the match played. It
// numbers innings the way the scorecard aggregates do -- both read Match.PlayedInnings --
// so a delivery's innings number and its match_inning row always describe the same
// innings. Every wicket on a delivery becomes a row of its own, in the order the file
// lists them; until IMPORT-06 was fixed only the first was kept.
func BuildBallEventRows(
	ctx context.Context,
	identity playerIDResolver,
	vocabulary wicketkinds.Vocabulary,
	m *Match,
	formatID int,
	matchID int64,
) (BallEvents, error) {
	var events BallEvents
	for i, inng := range m.PlayedInnings() {
		inningNo := i + 1
		// Pre-compute total legal deliveries in innings for phase clamping
		totalLegal := 0
		for _, over := range inng.Overs {
			for _, d := range over.Deliveries {
				if d.IsLegal() {
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
				// is_legal is the bowler's count (Delivery.IsLegal): a no-ball is faced by the
				// batter but is not one of the over's balls. The row carries no faced flag; a
				// ball faced is extras_wides == 0, which every reader derives by one rule.
				legal := d.IsLegal()
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
				wicketRows, err := buildWicketRows(ctx, identity, vocabulary, d, matchID, inningNo, overNo, ballNo)
				if err != nil {
					return BallEvents{}, err
				}
				events.Wickets = append(events.Wickets, wicketRows...)
				phaseName := phase.PhaseFor(int(formatID), ballSeq, totalLegal)
				inningRows = append(inningRows, db.BallEventRow{
					MatchID:       matchID,
					Innings:       inningNo,
					Over:          overNo,
					Ball:          ballNo,
					BallSeq:       ballSeq,
					IsLegal:       legal,
					Phase:         phaseName,
					StrikerID:     strikerID,
					NonStrikerID:  nonStrikerID,
					BowlerID:      bowlerID,
					RunsBatter:    d.Runs.Batter,
					RunsExtras:    d.Runs.Extras,
					RunsTotal:     d.Runs.Total,
					ExtrasWides:   d.Extras.Wides,
					ExtrasNoBalls: d.Extras.NoBalls,
					ExtrasByes:    d.Extras.Byes,
					ExtrasLegByes: d.Extras.LegByes,
					ExtrasPenalty: d.Extras.Penalty,
					ExtrasKind:    extrasKind,
				})
			}
		}
		events.Deliveries = append(events.Deliveries, inningRows...)
	}
	return events, nil
}

// buildWicketRows is one delivery's wickets as ball_event_wicket rows. The kind is
// stored as the vocabulary spells it, and a kind the vocabulary does not know fails the
// file: the row is what the rating pass reads, and a kind it cannot classify would be a
// wicket it cannot count.
func buildWicketRows(
	ctx context.Context,
	identity playerIDResolver,
	vocabulary wicketkinds.Vocabulary,
	d Delivery,
	matchID int64,
	inningNo, overNo, ballNo int,
) ([]db.BallEventWicketRow, error) {
	if d.Wickets == nil {
		return nil, nil
	}
	rows := make([]db.BallEventWicketRow, 0, len(*d.Wickets))
	for i, w := range *d.Wickets {
		kind, err := vocabulary.Kind(w.Kind)
		if err != nil {
			slog.Error("wicket kind not in the vocabulary",
				slog.Int64("match_id", matchID),
				slog.Int("innings", inningNo),
				slog.Int("over", overNo),
				slog.Int("ball", ballNo),
				slog.String("kind", w.Kind),
				slog.Any("err", err))
			return nil, err
		}
		var playerOutID *int64
		if name := strings.TrimSpace(w.PlayerOut); name != "" {
			if id, err := identity.PlayerID(ctx, name); err == nil {
				playerOutID = &id
			} else {
				slog.Error("get/create player failed", slog.String("name", name), slog.Any("err", err))
			}
		}
		rows = append(rows, db.BallEventWicketRow{
			MatchID:      matchID,
			Innings:      inningNo,
			Over:         overNo,
			Ball:         ballNo,
			WicketNumber: i + 1,
			Kind:         kind.Name,
			PlayerOutID:  playerOutID,
		})
	}
	return rows, nil
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

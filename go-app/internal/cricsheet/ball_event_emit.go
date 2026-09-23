package cricsheet

import (
	"context"
	"fmt"
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
//
// Every delivery the file lists becomes a row, legal or not. An innings can be all
// extras -- the archive holds one, a two-run chase won off a single wide (match 514034) --
// and skipping it for having no legal ball dropped a delivery that was bowled and left
// that innings number missing from ball_event while its match_inning row kept it
// (IMPORT-12). is_legal already says which deliveries the bowler's count includes, so
// nothing downstream needs the innings to be absent to know it faced no legal ball.
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
				where := deliveryAt{matchID: matchID, innings: inningNo, over: overNo, ball: ballNo}
				strikerID, err := deliveryPlayerID(ctx, identity, d.Batter, "striker", where)
				if err != nil {
					return BallEvents{}, err
				}
				nonStrikerID, err := deliveryPlayerID(ctx, identity, d.NonStriker, "non_striker", where)
				if err != nil {
					return BallEvents{}, err
				}
				bowlerID, err := deliveryPlayerID(ctx, identity, d.Bowler, "bowler", where)
				if err != nil {
					return BallEvents{}, err
				}
				extrasKind := extrasKindOf(d.Extras)
				wicketRows, err := buildWicketRows(ctx, identity, vocabulary, d, where)
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
	where deliveryAt,
) ([]db.BallEventWicketRow, error) {
	if d.Wickets == nil {
		return nil, nil
	}
	rows := make([]db.BallEventWicketRow, 0, len(*d.Wickets))
	for i, w := range *d.Wickets {
		kind, err := vocabulary.Kind(w.Kind)
		if err != nil {
			slog.Error("wicket kind not in the vocabulary",
				append(where.logAttrs(), slog.String("kind", w.Kind), slog.Any("err", err))...)
			return nil, err
		}
		playerOutID, err := deliveryPlayerID(ctx, identity, w.PlayerOut, "player_out", where)
		if err != nil {
			return nil, err
		}
		rows = append(rows, db.BallEventWicketRow{
			MatchID:      where.matchID,
			Innings:      where.innings,
			Over:         where.over,
			Ball:         where.ball,
			WicketNumber: i + 1,
			Kind:         kind.Name,
			PlayerOutID:  playerOutID,
		})
	}
	return rows, nil
}

// deliveryAt is where in the match a delivery sits, so that anything said about it names
// the ball and not just the file.
type deliveryAt struct {
	matchID int64
	innings int
	over    int
	ball    int
}

func (d deliveryAt) logAttrs() []any {
	return []any{
		slog.Int64("match_id", d.matchID),
		slog.Int("innings", d.innings),
		slog.Int("over", d.over),
		slog.Int("ball", d.ball),
	}
}

// deliveryPlayerID resolves one name on a delivery to a player id.
//
// A blank name is nobody and reads as NULL -- Cricsheet leaves the non-striker off a
// delivery it does not know one for. A name the resolver cannot key is a failure, and it
// fails the file: every other resolution site in the importer already does, and a
// ball_event whose striker is NULL is a delivery nobody faced. It used to be swallowed
// without even a log line, so the row went in wrong and nothing said so (IMPORT-13).
func deliveryPlayerID(
	ctx context.Context,
	identity playerIDResolver,
	name, role string,
	where deliveryAt,
) (*int64, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return nil, nil
	}
	id, err := identity.PlayerID(ctx, trimmed)
	if err != nil {
		slog.Error("cricsheet: could not resolve a name on a delivery",
			append(where.logAttrs(), slog.String("role", role), slog.String("name", trimmed), slog.Any("err", err))...)
		return nil, fmt.Errorf("resolve %s %q on match %d innings %d over %d ball %d: %w",
			role, trimmed, where.matchID, where.innings, where.over, where.ball, err)
	}
	return &id, nil
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

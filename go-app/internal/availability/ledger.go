package availability

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// Flag is one user's claim that a player has retired, and what became of it.
//
// A flag with no PromotedAt hides the player from that user's default pools and changes
// nothing for anyone else. A promoted flag also raised `player.is_retired`, and carries
// the criterion that allowed it to.
type Flag struct {
	PlayerID   int64
	Actor      string
	FlaggedAt  time.Time
	PromotedAt time.Time
	Criterion  string
	Detail     string
}

// Promoted reports whether this claim was corroborated into the stored fact.
func (f Flag) Promoted() bool { return !f.PromotedAt.IsZero() }

// Reason names why a pool leaves this player out: the stored fact where the claim was
// corroborated, the bare claim where it was not.
func (f Flag) Reason() string {
	if f.Promoted() {
		return ReasonRetired
	}
	return ReasonUserFlagged
}

// Store is the ledger's persistence. Two writes and two reads, and each write is one
// state change in full -- the claim, the stored fact it does or does not raise, and the
// events that explain both -- because a promotion recorded without its reason is the
// thing this whole design exists to prevent.
type Store interface {
	// ListFlags returns the actor's current claims, keyed by player id.
	ListFlags(ctx context.Context, actor string) (map[int64]Flag, error)

	// RetirementEvidence reads what the criteria are allowed to see about a player.
	RetirementEvidence(ctx context.Context, playerID int64) (Evidence, error)

	// SaveFlag records a claim, raises `player.is_retired` when the flag carries a
	// corroborating criterion, and appends the events describing what happened.
	SaveFlag(ctx context.Context, flag Flag) error

	// DeleteFlag removes a claim. Where that claim had raised the stored fact, the fact
	// is lowered again and the demotion is recorded. It reports whether a claim was
	// there to remove.
	DeleteFlag(ctx context.Context, actor string, playerID int64) (Flag, bool, error)
}

// Ledger applies the retirement rules over a Store. It holds no state of its own: the
// criteria are injected so a test can pin the promotion and demotion rules without a
// database, and the production list comes from Criteria(config.Load()).
type Ledger struct {
	store    Store
	criteria []Criterion
}

// NewLedger returns a ledger backed by store, corroborating with the given criteria.
func NewLedger(store Store, criteria []Criterion) *Ledger {
	return &Ledger{store: store, criteria: criteria}
}

// FlagResult is what a flag did: the claim as stored, and -- when nothing corroborated
// it -- what each criterion said, so the surface can tell a user why the exclusion is
// theirs alone rather than leaving them to guess.
type FlagResult struct {
	Flag Flag
	// Unchecked names the criteria whose evidence does not exist yet (X-1a).
	Unchecked []string
	// Notes is one line per criterion that answered, in the order they were tried.
	Notes []string
}

// Flags returns the actor's current claims, keyed by player id.
func (l *Ledger) Flags(ctx context.Context, actor string) (map[int64]Flag, error) {
	flags, err := l.store.ListFlags(ctx, normalizeActor(actor))
	if err != nil {
		slog.Error("availability.Ledger.Flags failed", slog.String("actor", actor), slog.Any("err", err))
		return nil, fmt.Errorf("list retirement flags: %w", err)
	}
	return flags, nil
}

// Flag records a user's claim that a player has retired, and promotes it to the stored
// fact where a criterion corroborates it.
//
// Promotion is evaluated here, once, at flag time -- not lazily on read. A pool query
// that re-derived retirement on every request would answer differently as the calendar
// moved without anything having been decided, and there would be no moment to record.
func (l *Ledger) Flag(
	ctx context.Context,
	actor string,
	playerID int64,
	formatCode string,
	at time.Time,
) (FlagResult, error) {
	actor = normalizeActor(actor)
	evidence, err := l.store.RetirementEvidence(ctx, playerID)
	if err != nil {
		slog.Error("availability.Ledger.Flag evidence failed",
			slog.Int64("player_id", playerID), slog.Any("err", err))
		return FlagResult{}, fmt.Errorf("retirement evidence for player %d: %w", playerID, err)
	}

	result := FlagResult{Flag: Flag{PlayerID: playerID, Actor: actor, FlaggedAt: at}}
	subject := Subject{Evidence: evidence, FormatCode: formatCode, At: at}
	for _, criterion := range l.criteria {
		verdict := criterion.Corroborates(subject)
		result.Notes = append(result.Notes, criterion.Name()+": "+verdict.Detail)
		if verdict.Unavailable {
			result.Unchecked = append(result.Unchecked, criterion.Name())
			continue
		}
		if verdict.Corroborated {
			result.Flag.PromotedAt = at
			result.Flag.Criterion = criterion.Name()
			result.Flag.Detail = verdict.Detail
			break
		}
	}

	if err := l.store.SaveFlag(ctx, result.Flag); err != nil {
		slog.Error("availability.Ledger.Flag save failed",
			slog.Int64("player_id", playerID), slog.String("actor", actor), slog.Any("err", err))
		return FlagResult{}, fmt.Errorf("save retirement flag for player %d: %w", playerID, err)
	}
	slog.Info("availability.Ledger.Flag recorded",
		slog.Int64("player_id", playerID),
		slog.String("actor", actor),
		slog.Bool("promoted", result.Flag.Promoted()),
		slog.String("criterion", result.Flag.Criterion))
	return result, nil
}

// Unflag removes a user's claim, demoting the stored fact where that claim had raised
// it. It reports whether there was a claim to remove, so a surface can tell "undone"
// from "there was nothing there".
func (l *Ledger) Unflag(ctx context.Context, actor string, playerID int64) (Flag, bool, error) {
	actor = normalizeActor(actor)
	removed, existed, err := l.store.DeleteFlag(ctx, actor, playerID)
	if err != nil {
		slog.Error("availability.Ledger.Unflag failed",
			slog.Int64("player_id", playerID), slog.String("actor", actor), slog.Any("err", err))
		return Flag{}, false, fmt.Errorf("remove retirement flag for player %d: %w", playerID, err)
	}
	slog.Info("availability.Ledger.Unflag done",
		slog.Int64("player_id", playerID),
		slog.String("actor", actor),
		slog.Bool("existed", existed),
		slog.Bool("demoted", existed && removed.Promoted()))
	return removed, existed, nil
}

// normalizeActor turns a missing or blank actor into DefaultActor, so one deployment's
// single user always addresses the same ledger.
func normalizeActor(actor string) string {
	trimmed := strings.TrimSpace(actor)
	if trimmed == "" {
		return DefaultActor
	}
	return trimmed
}

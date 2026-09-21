package predictteam

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/availability"
	"github.com/umayangag/cric-flow/go-app/internal/config"
	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// PoolRequest is what a caller asked for about one side's candidates (D-12).
//
// The zero value is the default: the per-format recency window, no manual picking. That
// is deliberate — the all-time pool is now a widening a user asks for, never what a
// caller gets by saying nothing.
type PoolRequest struct {
	// WindowMonths overrides the per-format default window. Zero uses the default.
	WindowMonths int
	// AllTime widens the pool back to everyone who has ever appeared for the club in
	// the format. It is what the pool used to be, unconditionally, and it is what made
	// the Upcoming-match tab offer players who retired a decade ago.
	AllTime bool
	// Manual is the subset a user ticked out of the candidate list. When it is set it
	// *is* the pool: the user has looked at the candidates and chosen, which is better
	// evidence about availability than any window. Extra ids are still added to it.
	Manual []int64
}

// ExcludedCandidate is a candidate the ledger kept out of the pool, on the wire.
//
// It is in the response for §8.7's reason: a filter that removes a player has to say so
// where the answer is read, not only in a log line. The surface strikes him through with
// the reason beside him, and a user can undo it.
type ExcludedCandidate struct {
	PlayerID   int64  `json:"player_id"`
	PlayerName string `json:"player_name"`
	// LastPlayed is YYYY-MM-DD, or absent where he has no appearance for this club in
	// this format before the cutoff.
	LastPlayed string `json:"last_played,omitempty"`
	// Reason is availability.ReasonRetired or availability.ReasonUserFlagged.
	Reason string `json:"reason"`
	// Detail is the criterion's evidence, where a criterion corroborated the flag.
	Detail string `json:"detail,omitempty"`
}

// PoolSummary says which candidates a side's XI was chosen out of, and who was left out.
//
// Before D-12 the pool was all-time and the response said nothing about it, so a user
// reading an XI could not tell whether a name he did not recognise was a discovery or a
// player who retired in 2015. Every number here is on the wire so the surface can say
// "played for <team> in the last <N> months (<M> players, <K> excluded as retired)".
type PoolSummary struct {
	// Source is availability.SourceRecencyWindow, SourceAllTime or SourceManual.
	Source string `json:"source"`
	// WindowMonths is the window that was applied; absent on an all-time or manual pool.
	WindowMonths int `json:"window_months,omitempty"`
	// Since is the first match date the window accepted, YYYY-MM-DD; absent likewise.
	Since string `json:"since,omitempty"`
	// Size is how many players were offered to the optimiser.
	Size int `json:"size"`
	// RetiredExcluded is how many candidates the ledger removed.
	RetiredExcluded int `json:"retired_excluded"`
	// Excluded names them, so the exclusion is visible and reversible.
	Excluded []ExcludedCandidate `json:"excluded,omitempty"`
}

// InsufficientPoolError reports a pool too small to field an XI.
//
// It names the window, because after D-12 that is the likely cause and the fix is a
// choice the user can make: widen to all-time, or pick candidates by hand.
type InsufficientPoolError struct {
	Team    string
	Size    int
	Need    int
	Summary PoolSummary
}

func (e *InsufficientPoolError) Error() string {
	if e.Summary.Source == availability.SourceRecencyWindow {
		return fmt.Sprintf(
			"%s has only %d players who appeared in the last %d months, need at least %d; "+
				"widen the pool to all-time or choose the candidates by hand",
			e.Team, e.Size, e.Summary.WindowMonths, e.Need)
	}
	return fmt.Sprintf("%s has only %d players, need at least %d", e.Team, e.Size, e.Need)
}

// loadPool builds one side's candidate pool and the summary that explains it.
//
// applyLedger is off for backtests. A retirement flag set today is not evidence about a
// 2019 pool, and neither is the fact it was promoted to, so a backtest at a 2019 cutoff
// still sees the players of 2019 (H-19). The window is not skipped for a backtest: it is
// relative to the as-of date, so it describes 2019's recent players and not today's.
func loadPool(
	ctx context.Context,
	format string,
	team db.TeamSide,
	cutoff time.Time,
	request PoolRequest,
	extra []int64,
	flags map[int64]availability.Flag,
	applyLedger bool,
	teamSize int,
) ([]db.PlayerPoolRow, PoolSummary, error) {
	label := team.Label()
	if len(request.Manual) > 0 {
		return loadManualPool(ctx, format, team, cutoff, request.Manual, extra, teamSize)
	}

	query, summary := poolQueryFor(format, team.ClubID, cutoff, request, flags, applyLedger)
	query.ExtraPlayerIDs = extra

	pool, err := db.ListPlayerPoolByOpposition(ctx, query)
	if err != nil {
		slog.Error("predictteam.PredictTeams pool failed", slog.String("team", label), slog.Any("err", err))
		return nil, PoolSummary{}, fmt.Errorf("%s pool: %w", label, err)
	}
	summary.Size = len(pool.Players)
	summary.RetiredExcluded = len(pool.Excluded)
	summary.Excluded = newExcludedCandidates(pool.Excluded)
	if err := checkPoolSize(label, len(pool.Players), teamSize, summary); err != nil {
		return nil, PoolSummary{}, err
	}
	return pool.Players, summary, nil
}

// poolQueryFor turns a caller's pool request into the query that answers it, and the
// summary that will explain the answer.
//
// It is shared by the prediction path and the candidate list so the list a user ticks
// from is provably the pool a prediction would have used: two copies of this arithmetic
// would drift, and the whole point of showing the window is that it is the real one.
func poolQueryFor(
	format string,
	clubID int64,
	cutoff time.Time,
	request PoolRequest,
	flags map[int64]availability.Flag,
	applyLedger bool,
) (db.PoolQuery, PoolSummary) {
	// The cutoff is the fixture's own calendar day, whatever zone the caller wrote it in:
	// the window is [start, cutoff) over a `date` column, so an instant carrying a UTC
	// offset has to become a day before any of this arithmetic runs (GO-09).
	query := db.PoolQuery{
		FormatCode:   format,
		OppositionID: clubID,
		Cutoff:       availability.CalendarDay(cutoff),
		ApplyLedger:  applyLedger,
	}
	if applyLedger {
		query.Flags = flags
	}
	summary := PoolSummary{Source: availability.SourceAllTime}
	if request.AllTime {
		return query, summary
	}
	months := request.WindowMonths
	if months <= 0 {
		months = availability.WindowMonths(config.Load(), format)
	}
	query.Since = availability.WindowStart(query.Cutoff, months)
	summary.Source = availability.SourceRecencyWindow
	summary.WindowMonths = months
	summary.Since = query.Since.Format(time.DateOnly)
	return query, summary
}

// loadManualPool resolves the ids a user ticked, plus the extra ids, as the pool.
//
// Neither the window nor the ledger is applied: both exist to guess at availability, and
// the user has just told us. The summary still reports the size, so the response says
// what was scored.
func loadManualPool(
	ctx context.Context,
	format string,
	team db.TeamSide,
	cutoff time.Time,
	manual, extra []int64,
	teamSize int,
) ([]db.PlayerPoolRow, PoolSummary, error) {
	label := team.Label()
	rows, err := db.ListPlayerRowsByID(ctx, format, team.ClubID, cutoff, append(append([]int64{}, manual...), extra...))
	if err != nil {
		slog.Error("predictteam.PredictTeams manual pool failed",
			slog.String("team", label), slog.Any("err", err))
		return nil, PoolSummary{}, fmt.Errorf("%s pool: %w", label, err)
	}
	summary := PoolSummary{Source: availability.SourceManual, Size: len(rows)}
	if err := checkPoolSize(label, len(rows), teamSize, summary); err != nil {
		return nil, PoolSummary{}, err
	}
	return rows, summary, nil
}

// checkPoolSize refuses a pool too small to field an XI, naming the window that produced
// it so the refusal points at the choice that would fix it.
func checkPoolSize(label string, size, teamSize int, summary PoolSummary) error {
	if size >= teamSize {
		return nil
	}
	err := &InsufficientPoolError{Team: label, Size: size, Need: teamSize, Summary: summary}
	slog.Error("predictteam.PredictTeams pool size",
		slog.String("team", label),
		slog.String("pool_source", summary.Source),
		slog.Int("window_months", summary.WindowMonths),
		slog.Any("err", err))
	return err
}

// newExcludedCandidates turns the repository's exclusions into the wire shape.
func newExcludedCandidates(excluded []db.ExcludedPlayer) []ExcludedCandidate {
	if len(excluded) == 0 {
		return nil
	}
	out := make([]ExcludedCandidate, 0, len(excluded))
	for _, player := range excluded {
		out = append(out, ExcludedCandidate{
			PlayerID:   player.PlayerID,
			PlayerName: player.PlayerName,
			LastPlayed: formatDate(player.LastPlayed),
			Reason:     player.Reason,
			Detail:     player.Detail,
		})
	}
	return out
}

// formatDate renders a date for the wire, or "" where there is none.
func formatDate(at time.Time) string {
	if at.IsZero() {
		return ""
	}
	return at.Format(time.DateOnly)
}

// poolFlags reads the actor's retirement claims once per request, so the two pool
// queries do not each go and fetch them.
//
// A backtest passes applyLedger false and never gets here: its pool is the pool of its
// as-of date, and a claim made later is not evidence about it.
func poolFlags(ctx context.Context, actor string) (map[int64]availability.Flag, error) {
	ledger := availability.NewLedger(db.NewPlayerStatusStore(), availability.Criteria(config.Load()))
	return ledger.Flags(ctx, actor)
}

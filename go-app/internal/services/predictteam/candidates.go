package predictteam

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// CandidatesInput asks for the list a user picks a pool out of (D-12, manual picking).
type CandidatesInput struct {
	Format string
	Team   db.TeamRef
	// MatchDate is the fixture the pool is for; the window ends here.
	MatchDate time.Time
	// Request is the scope: the recency window by default, all-time when asked. A manual
	// pick means nothing here — this list is what a manual pick is made from.
	Request PoolRequest
	// Actor is whose retirement claims are shown. Empty is the default user.
	Actor string
}

// Candidate is one player on the list, with what a person needs in order to decide:
// when he last played for this club in this format, and whether the ledger is currently
// keeping him out.
//
// Age belongs here too and is not here yet: no date of birth exists in this schema until
// X-1a lands, and inventing one from an appearance history would be a guess presented as
// a fact.
type Candidate struct {
	PlayerID       int64  `json:"player_id"`
	PlayerName     string `json:"player_name"`
	IsWicketKeeper bool   `json:"is_wicket_keeper"`
	// LastPlayed is YYYY-MM-DD, or absent where he never played for this club in this
	// format before the cutoff.
	LastPlayed string `json:"last_played,omitempty"`
	// Excluded is true where the ledger is keeping him out of the default pool. He is on
	// the list all the same: an exclusion a user cannot see is one he cannot undo.
	Excluded bool `json:"excluded"`
	// Reason is availability.ReasonRetired or availability.ReasonUserFlagged; empty
	// where he is not excluded.
	Reason string `json:"reason,omitempty"`
	// Detail is the criterion's evidence behind a promoted flag.
	Detail string `json:"detail,omitempty"`
}

// CandidatesResult is the list and the scope it was drawn from.
type CandidatesResult struct {
	Side ResolvedSide `json:"side"`
	Pool PoolSummary  `json:"pool"`
	// Candidates holds the offered players and the excluded ones together, sorted by
	// name, because they are one list to a person looking at a team sheet.
	Candidates []Candidate `json:"candidates"`
}

// Candidates lists the players a side's pool would be drawn from, excluded ones
// included and marked.
//
// It is the same query the prediction path runs, through the same builder, so what a
// user ticks from is what a prediction would otherwise have used. Unlike the prediction
// path it does not refuse a pool too small to field an XI: a short list is what the user
// opened this to see.
func Candidates(ctx context.Context, input CandidatesInput) (*CandidatesResult, error) {
	format := normalizeFormat(input.Format)
	if format == "" || input.Team.IsEmpty() {
		err := fmt.Errorf("format and a side are required")
		slog.Error("predictteam.Candidates validation failed", slog.Any("err", err))
		return nil, err
	}
	side, err := resolveSide(ctx, input.Team, format, "team")
	if err != nil {
		return nil, err
	}
	cutoff := input.MatchDate.Truncate(24 * time.Hour)

	flags, err := poolFlags(ctx, input.Actor)
	if err != nil {
		return nil, err
	}
	query, summary := poolQueryFor(format, side.ClubID, cutoff, input.Request, flags, true)
	pool, err := db.ListPlayerPoolByOpposition(ctx, query)
	if err != nil {
		slog.Error("predictteam.Candidates pool failed",
			slog.String("team", side.Label()), slog.Any("err", err))
		return nil, fmt.Errorf("%s candidates: %w", side.Label(), err)
	}

	summary.Size = len(pool.Players)
	summary.RetiredExcluded = len(pool.Excluded)
	return &CandidatesResult{
		Side:       newResolvedSide(side),
		Pool:       summary,
		Candidates: newCandidates(pool),
	}, nil
}

// newCandidates merges the offered and the excluded players into one name-ordered list.
func newCandidates(pool db.PlayerPool) []Candidate {
	candidates := make([]Candidate, 0, len(pool.Players)+len(pool.Excluded))
	for _, player := range pool.Players {
		candidates = append(candidates, Candidate{
			PlayerID:       player.PlayerID,
			PlayerName:     player.PlayerName,
			IsWicketKeeper: player.IsWicketKeeper != 0,
			LastPlayed:     formatDate(player.LastPlayed),
		})
	}
	for _, player := range pool.Excluded {
		candidates = append(candidates, Candidate{
			PlayerID:   player.PlayerID,
			PlayerName: player.PlayerName,
			LastPlayed: formatDate(player.LastPlayed),
			Excluded:   true,
			Reason:     player.Reason,
			Detail:     player.Detail,
		})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].PlayerName == candidates[j].PlayerName {
			return candidates[i].PlayerID < candidates[j].PlayerID
		}
		return candidates[i].PlayerName < candidates[j].PlayerName
	})
	return candidates
}

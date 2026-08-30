package cricsheet

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// squadIDResolver is the slice of the entity cache that squad rows need. Narrow so
// the row builder can be tested without a database.
type squadIDResolver interface {
	GetPlayerID(ctx context.Context, name string) (int64, error)
	GetOppositionID(ctx context.Context, name string) (int64, error)
}

// SquadMember is one entry of info.players: a player and the side that picked them.
type SquadMember struct {
	Team   string
	Player string
}

// ErrNoSquad reports that a match file carries no info.players.
//
// It is a named condition rather than an empty result because the two are not the
// same thing to a caller. A match with no recorded squad has to be excluded from the
// win export; a match whose squad happens to be empty would be a side of nobody, and
// the export treating those alike is how the leak this table exists to remove got in.
// Every one of the 22,734 files in the current dataset has the key, so in practice
// this fires on a truncated or hand-edited file.
var ErrNoSquad = fmt.Errorf("match file carries no info.players")

// SquadFromInfo flattens info.players into a validated, deterministically ordered list.
//
// Order follows info.teams so two imports of the same file produce the same rows in
// the same order; Go map iteration would not. Teams that appear only in info.players
// are appended in name order rather than dropped, so a file whose two lists disagree
// is still recorded in full and the mismatch is caught below rather than half-applied.
func SquadFromInfo(info Info) ([]SquadMember, error) {
	if len(info.Players) == 0 {
		return nil, ErrNoSquad
	}

	teamOrder := make([]string, 0, len(info.Players))
	seenTeam := map[string]bool{}
	for _, t := range info.Teams {
		team := strings.TrimSpace(t)
		if team == "" || seenTeam[team] {
			continue
		}
		if _, ok := info.Players[team]; ok {
			teamOrder = append(teamOrder, team)
			seenTeam[team] = true
		}
	}
	extra := make([]string, 0)
	for team := range info.Players {
		if name := strings.TrimSpace(team); name != "" && !seenTeam[name] {
			extra = append(extra, name)
		}
	}
	sort.Strings(extra)
	teamOrder = append(teamOrder, extra...)

	members := make([]SquadMember, 0, len(info.Players)*11)
	// A player belongs to exactly one side, which is what match_player's primary key
	// asserts. Catching the violation here names the player and both teams; letting
	// the insert catch it would surface as a constraint error naming neither.
	pickedBy := map[string]string{}
	for _, team := range teamOrder {
		for _, raw := range info.Players[team] {
			player := strings.TrimSpace(raw)
			if player == "" {
				return nil, fmt.Errorf("team %q lists an empty player name", team)
			}
			if previous, ok := pickedBy[player]; ok {
				if previous == team {
					return nil, fmt.Errorf("player %q is listed twice for team %q", player, team)
				}
				return nil, fmt.Errorf("player %q is listed for both %q and %q", player, previous, team)
			}
			pickedBy[player] = team
			members = append(members, SquadMember{Team: team, Player: player})
		}
	}
	if len(members) == 0 {
		return nil, ErrNoSquad
	}
	return members, nil
}

// buildMatchPlayerRows resolves a match's squads into match_player rows.
//
// A file with no info.players yields no rows and a warning rather than an error: it is
// a gap in the source data, not a corrupt file, and failing the import would lose the
// ball-by-ball record over it. The win export excludes matches with no recorded squad,
// so the gap stays visible where it matters instead of becoming a side of nobody.
func buildMatchPlayerRows(
	ctx context.Context,
	resolver squadIDResolver,
	info Info,
	matchID int64,
	path string,
	dateISO string,
) ([]db.MatchPlayer, error) {
	members, err := SquadFromInfo(info)
	if err != nil {
		if errors.Is(err, ErrNoSquad) {
			slog.Warn("cricsheet: match has no info.players, squad not recorded",
				slog.String("file", path),
				slog.Int64("match_id", matchID),
				slog.String("match_date", dateISO),
				slog.String("teams", strings.Join(info.Teams, " vs ")))
			return nil, nil
		}
		slog.Error("cricsheet: info.players is unusable",
			slog.String("file", path),
			slog.Int64("match_id", matchID),
			slog.String("match_date", dateISO),
			slog.Any("err", err))
		return nil, fmt.Errorf("read info.players: %w", err)
	}

	oppositionIDs := map[string]int64{}
	rows := make([]db.MatchPlayer, 0, len(members))
	for _, member := range members {
		oppositionID, ok := oppositionIDs[member.Team]
		if !ok {
			oppositionID, err = resolver.GetOppositionID(ctx, member.Team)
			if err != nil {
				slog.Error("get/create opposition for squad failed",
					slog.String("file", path),
					slog.Int64("match_id", matchID),
					slog.String("match_date", dateISO),
					slog.String("team", member.Team),
					slog.Any("err", err))
				return nil, fmt.Errorf("get/create opposition for squad team %q: %w", member.Team, err)
			}
			oppositionIDs[member.Team] = oppositionID
		}
		playerID, err := resolver.GetPlayerID(ctx, member.Player)
		if err != nil {
			slog.Error("get/create player for squad failed",
				slog.String("file", path),
				slog.Int64("match_id", matchID),
				slog.String("match_date", dateISO),
				slog.String("team", member.Team),
				slog.String("player", member.Player),
				slog.Any("err", err))
			return nil, fmt.Errorf("get/create player %q for squad: %w", member.Player, err)
		}
		rows = append(rows, db.MatchPlayer{
			MatchID:      matchID,
			PlayerID:     playerID,
			OppositionID: oppositionID,
		})
	}
	return rows, nil
}

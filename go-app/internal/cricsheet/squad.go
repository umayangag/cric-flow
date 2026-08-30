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

// SquadFromInfo flattens info.players into a validated, deterministically ordered list,
// along with the names it could not attribute to one side.
//
// Order follows info.teams so two imports of the same file produce the same rows in
// the same order; Go map iteration would not. Teams that appear only in info.players
// are appended in name order rather than dropped, so a file whose two lists disagree
// is still recorded in full and the mismatch is caught below rather than half-applied.
//
// A name appearing under both teams is two people who share a scorecard name, not a
// corrupt file: Cricsheet's own registry is keyed by name, so it collapses them into
// one identifier and the source cannot say which side each delivery belongs to. Since
// this repository identifies players by name too, they are already one player_id, and
// match_player's primary key cannot hold that id twice for one match. Guessing a side
// would invent data, so the name is dropped from *both* squads and returned for the
// caller to log. Two files in the current dataset are affected, each losing one player
// from an eleven.
//
// The same name twice within one team is a different case and is deduplicated rather
// than dropped: which side they played for is not in doubt.
func SquadFromInfo(info Info) (members []SquadMember, ambiguous []string, err error) {
	if len(info.Players) == 0 {
		return nil, nil, ErrNoSquad
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

	// Which side claimed each name, and the names more than one side claimed. Both are
	// needed before any member is emitted: a name is only known to be ambiguous once
	// the second team has been read, by which point the first team's entry is already
	// built, so the drop has to happen in a second pass.
	pickedBy := map[string]string{}
	contested := map[string]bool{}
	for _, team := range teamOrder {
		for _, raw := range info.Players[team] {
			player := strings.TrimSpace(raw)
			if player == "" {
				return nil, nil, fmt.Errorf("team %q lists an empty player name", team)
			}
			if previous, ok := pickedBy[player]; ok && previous != team {
				contested[player] = true
			}
			pickedBy[player] = team
		}
	}

	members = make([]SquadMember, 0, len(pickedBy))
	emitted := map[string]bool{}
	for _, team := range teamOrder {
		for _, raw := range info.Players[team] {
			player := strings.TrimSpace(raw)
			// Deduplicate within a team: the same name twice is one player_id either
			// way, and unlike the contested case there is no doubt about the side.
			if contested[player] || emitted[player] {
				continue
			}
			emitted[player] = true
			members = append(members, SquadMember{Team: team, Player: player})
		}
	}

	ambiguous = make([]string, 0, len(contested))
	for player := range contested {
		ambiguous = append(ambiguous, player)
	}
	sort.Strings(ambiguous)

	if len(members) == 0 {
		return nil, ambiguous, ErrNoSquad
	}
	return members, ambiguous, nil
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
	members, ambiguous, err := SquadFromInfo(info)
	if len(ambiguous) > 0 {
		// Not an error: the source cannot say which side these played for, so they are
		// left out of both squads rather than guessed at. Logged because a squad of ten
		// is a fact about the export's inputs that should be traceable to its cause.
		slog.Warn("cricsheet: player named on both teams, omitted from both squads",
			slog.String("file", path),
			slog.Int64("match_id", matchID),
			slog.String("match_date", dateISO),
			slog.String("players", strings.Join(ambiguous, ", ")))
	}
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

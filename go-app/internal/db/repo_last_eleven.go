package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/umayangag/cric-flow/go-app/internal/auction"
)

// The last eleven a side actually fielded (P3-2).
//
// A projection needs an opposing eleven and a league has no single one, so the operator
// names it. This read is the starting point they edit, and it is deliberately a *fact*:
// the eleven this database records a side fielding in its most recent match in the format.
// Offering a plausible-looking side assembled by rating would be inventing an opposition
// and calling it evidence; offering the last real one is a fact with a date on it, which
// is why the date comes back with the names.

// LastFieldedEleven is one side's most recent recorded eleven, with the match it came
// from, so an operator seeding an assumption can see how old the fact is.
type LastFieldedEleven struct {
	OppositionID   int64
	OppositionName string
	MatchDate      time.Time
	EventName      string
	VenueName      string
	Players        []auction.NamedPlayer
}

// ErrNoFieldedEleven reports that this database holds no eleven for the side in the
// format. Refused rather than answered with a short list: an operator seeding an
// opposition from a seven-man record would be seeding an eleven that never played.
var ErrNoFieldedEleven = errors.New("no eleven recorded for that side in that format")

// ReadLastFieldedEleven returns the eleven a side last fielded in a format.
//
// "Last" is by match date and then by match id, so a side with two matches on one day
// resolves the same way on every read rather than by whatever order the planner chose.
// Only a match whose recorded side holds a full eleven is offered — the importer records
// what the source listed, and a match whose sheet was partial would seed an assumption
// with a hole in it that nothing downstream would name.
func ReadLastFieldedEleven(
	ctx context.Context,
	formatCode string,
	oppositionID int64,
) (*LastFieldedEleven, error) {
	if Pool == nil {
		return nil, errors.New("db pool not initialized")
	}
	var found LastFieldedEleven
	var matchID int64
	err := Pool.QueryRow(ctx, `
		SELECT m.match_id, m.match_date, COALESCE(m.event_name, ''), COALESCE(v.venue_name, ''),
		       o.id, o.opposition_name
		FROM match m
		JOIN match_format mf ON mf.id = m.format_id
		JOIN match_player mp ON mp.match_id = m.match_id AND mp.opposition_id = $2
		JOIN opposition o ON o.id = mp.opposition_id
		LEFT JOIN venue v ON v.id = m.venue_id
		WHERE mf.code = $1
		GROUP BY m.match_id, m.match_date, m.event_name, v.venue_name, o.id, o.opposition_name
		HAVING COUNT(*) = $3
		ORDER BY m.match_date DESC, m.match_id DESC
		LIMIT 1
	`, formatCode, oppositionID, auction.TeamSize).
		Scan(&matchID, &found.MatchDate, &found.EventName, &found.VenueName,
			&found.OppositionID, &found.OppositionName)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNoFieldedEleven
	}
	if err != nil {
		return nil, fmt.Errorf("read last fielded eleven for %d in %s: %w", oppositionID, formatCode, err)
	}
	if found.Players, err = readMatchSidePlayers(ctx, matchID, oppositionID); err != nil {
		return nil, err
	}
	return &found, nil
}

// readMatchSidePlayers reads one side's players from one match, name-ordered.
func readMatchSidePlayers(ctx context.Context, matchID, oppositionID int64) ([]auction.NamedPlayer, error) {
	rows, err := Pool.Query(ctx, `
		SELECT mp.player_id, COALESCE(p.external_id, ''), p.player_name
		FROM match_player mp
		JOIN player p ON p.id = mp.player_id
		WHERE mp.match_id = $1 AND mp.opposition_id = $2
		ORDER BY p.player_name, mp.player_id
	`, matchID, oppositionID)
	if err != nil {
		return nil, fmt.Errorf("read fielded eleven of match %d: %w", matchID, err)
	}
	defer rows.Close()

	players := make([]auction.NamedPlayer, 0, auction.TeamSize)
	for rows.Next() {
		var player auction.NamedPlayer
		if err := rows.Scan(&player.PlayerID, &player.ExternalID, &player.PlayerName); err != nil {
			return nil, fmt.Errorf("read fielded eleven of match %d: %w", matchID, err)
		}
		players = append(players, player)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read fielded eleven of match %d: %w", matchID, err)
	}
	return players, nil
}

// VenueName is a ground's name for the surface, by the id the record and the model both
// know it by. Empty where the database holds no such venue — the caller names the ground
// by its id rather than showing a blank where a name should be.
func VenueName(ctx context.Context, venueID int64) (string, error) {
	if Pool == nil {
		return "", errors.New("db pool not initialized")
	}
	var name string
	err := Pool.QueryRow(ctx, `SELECT venue_name FROM venue WHERE id = $1`, venueID).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read venue %d: %w", venueID, err)
	}
	return name, nil
}

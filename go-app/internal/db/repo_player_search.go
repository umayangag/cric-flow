package db

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/umayangag/cric-flow/go-app/internal/availability"
)

// PlayerSearchQuery asks for players by name across every club (P3-1).
//
// `GET /api/options/candidates` is per club, because a prediction is about one side. An
// auction list is not about one side: it spans every franchise in the room, and a player
// is looked up by the name the auctioneer just called. So this is a separate read rather
// than a widened pool query — it applies no recency window, because a window is a claim
// about who would be picked and an auction list is a claim about who is in the room.
type PlayerSearchQuery struct {
	// Prefix matches the start of the player's name or of any word in it, so "kohli"
	// finds "V Kohli" the way an auctioneer would say it.
	Prefix string
	// FormatCode narrows the clubs and the last-played date to one format. Empty reports
	// every format he has played.
	FormatCode string
	Limit      int
	// Flags are the requesting user's retirement claims, so the search can mark a player
	// the ledger is keeping out of pools. Marked, never removed: an auction list is what
	// the room is selling and the ledger's opinion about it belongs beside a name, not
	// instead of one (§8.7).
	Flags map[int64]availability.Flag
}

// PlayerSearchRow is one player a cross-club search found.
type PlayerSearchRow struct {
	PlayerID int64
	// ExternalID is the Cricsheet registry id ml-service knows him by; empty where this
	// database has none.
	ExternalID string
	PlayerName string
	// IsWicketKeeper is `player.is_wicket_keeper`, the *database's* flag, set from the
	// name sets an import read. It is not the model's keeper role — that is read off the
	// served rating vectors — and every surface that shows it says which of the two it is.
	IsWicketKeeper bool
	// LastPlayed is his most recent appearance, in the searched format where one was
	// named. Zero where this database holds no appearance for him at all.
	LastPlayed time.Time
	// Clubs are the sides he has appeared for, most recently first.
	Clubs []string
	// Formats are the format codes he has appeared in.
	Formats []string
	// Excluded and Reason are the retirement ledger's, where it holds a claim about him.
	Excluded bool
	Reason   string
	Detail   string
}

// playerSearchLimitDefault and playerSearchLimitMax bound one search. A name prefix at an
// auction is typed to find one player; the cap is there so a one-letter prefix cannot ask
// for the whole registry.
const (
	playerSearchLimitDefault = 25
	playerSearchLimitMax     = 100
)

// SearchPlayers finds players by name prefix across every club, with the clubs and
// formats they have played and the retirement ledger's flag where it holds one.
//
// The appearance half of the query is the candidate list's own: the same two unions over
// `batting_data` and `bowling_data` through `match_inning`, and the same
// `COALESCE(canonical_id, id)` club grouping, so a renamed franchise is one club here
// exactly as it is there. What it does not reuse is the window and the pool's exclusion:
// this list is not a pool.
func SearchPlayers(ctx context.Context, query PlayerSearchQuery) ([]PlayerSearchRow, error) {
	if Pool == nil {
		return nil, errors.New("db pool not initialized")
	}
	prefix := strings.TrimSpace(query.Prefix)
	if len(prefix) < 2 {
		return nil, nil
	}
	limit := query.Limit
	if limit <= 0 {
		limit = playerSearchLimitDefault
	}
	limit = min(limit, playerSearchLimitMax)

	var formatCode any
	if code := strings.TrimSpace(query.FormatCode); code != "" {
		formatCode = code
	}
	rows, err := Pool.Query(ctx, playerSearchSQL, prefix+"%", "% "+prefix+"%", limit, formatCode)
	if err != nil {
		return nil, fmt.Errorf("search players %q: %w", prefix, err)
	}
	defer rows.Close()

	found := make([]PlayerSearchRow, 0, limit)
	for rows.Next() {
		row, err := scanPlayerSearchRow(rows)
		if err != nil {
			return nil, fmt.Errorf("search players %q: %w", prefix, err)
		}
		found = append(found, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("search players %q: %w", prefix, err)
	}
	return applyFlags(found, query.Flags), nil
}

// playerSearchSQL is the search itself. It is a constant so the reader meets the whole
// statement at once rather than in fragments of a Go string.
const playerSearchSQL = `
	WITH matched AS (
	  SELECT p.id, COALESCE(p.external_id, '') AS external_id, p.player_name,
	         p.is_wicket_keeper, p.is_retired
	  FROM player p
	  WHERE p.player_name ILIKE $1 OR p.player_name ILIKE $2
	  ORDER BY p.player_name, p.id
	  LIMIT $3
	), appearances AS (
	  SELECT bd.player_id AS id, m.match_date, m.format_id,
	         mi.batting_team_opposition_id AS opposition_id
	  FROM batting_data bd
	  JOIN match_inning mi ON mi.match_id = bd.match_id AND mi.inning_number = bd.inning_number
	  JOIN match m ON m.match_id = bd.match_id
	  WHERE bd.player_id IN (SELECT id FROM matched)
	  UNION ALL
	  SELECT bw.player_id, m.match_date, m.format_id,
	         mi.bowling_team_opposition_id
	  FROM bowling_data bw
	  JOIN match_inning mi ON mi.match_id = bw.match_id AND mi.inning_number = bw.inning_number
	  JOIN match m ON m.match_id = bw.match_id
	  WHERE bw.player_id IN (SELECT id FROM matched)
	), in_format AS (
	  SELECT a.* FROM appearances a
	  JOIN match_format mf ON mf.id = a.format_id
	  WHERE $4::text IS NULL OR mf.code = $4
	), per_club AS (
	  SELECT f.id, COALESCE(o.canonical_id, o.id) AS club_id, MAX(f.match_date) AS last_for_club
	  FROM in_format f
	  JOIN opposition o ON o.id = f.opposition_id
	  GROUP BY 1, 2
	), clubs AS (
	  SELECT pc.id, array_agg(c.opposition_name::text ORDER BY pc.last_for_club DESC) AS club_names
	  FROM per_club pc
	  JOIN opposition c ON c.id = pc.club_id
	  GROUP BY pc.id
	), played AS (
	  SELECT f.id, MAX(f.match_date) AS last_played FROM in_format f GROUP BY f.id
	), played_formats AS (
	  SELECT a.id, array_agg(DISTINCT mf.code::text) AS format_codes
	  FROM appearances a
	  JOIN match_format mf ON mf.id = a.format_id
	  GROUP BY a.id
	)
	SELECT m.id, m.external_id, m.player_name, m.is_wicket_keeper, m.is_retired,
	       pl.last_played,
	       COALESCE(c.club_names, ARRAY[]::text[]),
	       COALESCE(pf.format_codes, ARRAY[]::text[])
	FROM matched m
	LEFT JOIN clubs c ON c.id = m.id
	LEFT JOIN played pl ON pl.id = m.id
	LEFT JOIN played_formats pf ON pf.id = m.id
	ORDER BY m.player_name, m.id
`

// scanPlayerSearchRow reads one row and applies the ledger's verdict to it.
func scanPlayerSearchRow(rows pgx.Rows) (PlayerSearchRow, error) {
	var row PlayerSearchRow
	var isKeeper, isRetired int16
	var lastPlayed *time.Time
	if err := rows.Scan(&row.PlayerID, &row.ExternalID, &row.PlayerName, &isKeeper, &isRetired,
		&lastPlayed, &row.Clubs, &row.Formats); err != nil {
		return PlayerSearchRow{}, err
	}
	row.IsWicketKeeper = isKeeper != 0
	if lastPlayed != nil {
		row.LastPlayed = *lastPlayed
	}
	if isRetired != 0 {
		row.Excluded = true
		row.Reason = availability.ReasonRetired
	}
	return row, nil
}

// applyFlags marks the players this user has claimed have retired.
//
// The stored fact outranks a claim, exactly as the candidate list reads it: a promoted
// flag is `is_retired = 1` for everybody and is reported as the fact, and an
// uncorroborated claim is reported as this user's own.
func applyFlags(found []PlayerSearchRow, flags map[int64]availability.Flag) []PlayerSearchRow {
	for i := range found {
		flag, claimed := flags[found[i].PlayerID]
		if !claimed {
			continue
		}
		found[i].Detail = flag.Detail
		if found[i].Excluded {
			continue
		}
		found[i].Excluded = true
		found[i].Reason = flag.Reason()
	}
	return found
}

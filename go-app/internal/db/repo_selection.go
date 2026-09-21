package db

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/availability"
)

// PlayerPoolRow represents a candidate player.
type PlayerPoolRow struct {
	PlayerID int64
	// ExternalID is the Cricsheet registry id, and it is what the ML service knows a player
	// by: the rating state has been keyed on it since P-1, on both sources. The database id
	// is this repo's own and means nothing to ml-service.
	ExternalID     string
	PlayerName     string
	IsWicketKeeper int16
	// LastPlayed is the player's most recent appearance for this club in this format
	// before the cutoff. It is what makes the recency window checkable by a person: the
	// candidate list shows it beside every name. Zero for a row that entered by id
	// (must-include or a manual pick) without an appearance for the club in the window.
	LastPlayed time.Time
}

// ExcludedPlayer is a candidate the pool left out, and why.
//
// Exclusions are returned rather than filtered away because §8.7 applies to a filter as
// much as to a substitution: a player the ledger removes must be visible with his reason
// so a user can see the exclusion and undo it. The old `is_retired = 0` predicate hid
// nobody only because nothing ever wrote the column; had anything written it, the pool
// would have shrunk silently.
type ExcludedPlayer struct {
	PlayerID   int64
	PlayerName string
	LastPlayed time.Time
	// Reason is availability.ReasonRetired (the stored fact) or
	// availability.ReasonUserFlagged (this actor's own claim, uncorroborated).
	Reason string
	// Detail is the criterion's evidence for a promoted flag; empty otherwise.
	Detail string
}

// PlayerPool is a side's candidates: who may be picked, and who was left out.
type PlayerPool struct {
	Players  []PlayerPoolRow
	Excluded []ExcludedPlayer
}

// PoolQuery asks for a club's candidate players in one format, as of a cutoff.
//
// It takes an opposition id rather than a team name because a name is no longer one
// team: 130 of the 394 names in the dataset belong to both a men's and a women's side.
// Resolving the name is the caller's job (FindOppositionIDForFormat), which also stops
// this read path from creating a team row as a side effect of a prediction request.
//
// The id is a *club*: the pool spans every name the club has played under.
type PoolQuery struct {
	FormatCode   string
	OppositionID int64
	// Cutoff is the match being predicted. The pool is built from matches strictly
	// before it, so a prediction cannot read its own result.
	Cutoff time.Time
	// Since bounds the pool by recency: only players who appeared for the club in this
	// format on or after this date are candidates. Zero widens the pool to all-time,
	// which is a request a user makes deliberately and never a default (D-12).
	Since time.Time
	// ExtraPlayerIDs are added whatever any filter says — a new signing with no history
	// for the club, a player the caller insists on. They bypass the window and the
	// ledger both, because the caller naming a player by id is better evidence about
	// availability than anything this repository holds.
	ExtraPlayerIDs []int64
	// ApplyLedger turns the retirement ledger on. It is on for upcoming-match requests
	// and off for backtests: a flag set in 2026 is not evidence about a 2019 pool, and
	// neither is the fact it was promoted to, so a backtest at a 2019 cutoff still sees
	// the players of 2019 (H-19).
	ApplyLedger bool
	// Flags are the requesting user's claims, keyed by player id, read once by the
	// caller so this query stays a single round trip. Nil is the same as no claims.
	Flags map[int64]availability.Flag
}

// ListPlayerPoolByOpposition returns the club's candidates in the format: players with a
// batting or bowling appearance for the club before the cutoff and, when the query is
// recency-bounded, no earlier than Since.
//
// The pool is who *may* play, and nothing more: it used to carry each player's windowed
// consistency from `feature_raw_stats_snapshots`, for the score weights P-5 deleted. P-6
// dropped that table, and the rating state ml-service holds is where a player's form
// lives now -- go-app sends ids and lets the model read them.
func ListPlayerPoolByOpposition(ctx context.Context, q PoolQuery) (PlayerPool, error) {
	if Pool == nil {
		return PlayerPool{}, errors.New("db pool not initialized")
	}
	formatID, err := GetOrCreateMatchFormat(ctx, q.FormatCode)
	if err != nil {
		return PlayerPool{}, err
	}
	// Both bounds are the caller's calendar day, not the instant truncated: pgx encodes a
	// `date` parameter from the value's own year/month/day, so a cutoff carrying a UTC
	// offset would otherwise reach Postgres as the day before (GO-09).
	cutoffDate := availability.CalendarDay(q.Cutoff)

	// Players who have batted or bowled for this team (opposition) in matches before cutoff.
	// batting_team_opposition_id = team that batted (batters in batting_data play for that team);
	// bowling_team_opposition_id = team that bowled (bowlers in bowling_data play for that team).
	// A CTE gets the player_ids via JOINs, avoiding EXISTS per row.
	//
	// `since` is passed as a nullable date so one statement serves both the bounded and
	// the all-time pool: NULL is all-time. `p.is_retired` is selected rather than
	// filtered on, so the caller can report the exclusion instead of performing it
	// silently (§8.7) -- and it is now a column something writes (the retirement ledger,
	// migration 0009), which it was not when the predicate `is_retired = 0` was added.
	var since any
	if !q.Since.IsZero() {
		since = availability.CalendarDay(q.Since)
	}
	rows, err := Pool.Query(ctx, `
		WITH club AS (
		  -- Every opposition row belonging to the same club, so a rename does not halve
		  -- the pool: Royal Challengers Bengaluru's players include the ones who only ever
		  -- appear under Bangalore.
		  SELECT id FROM opposition WHERE COALESCE(canonical_id, id) = $3
		), eligible AS (
		  SELECT bd.player_id AS id, m.match_date
		  FROM batting_data bd
		  JOIN match_inning mi ON mi.match_id = bd.match_id AND mi.inning_number = bd.inning_number
		  JOIN match m ON m.match_id = bd.match_id
		  WHERE m.format_id = $1 AND m.match_date < $2
		    AND ($4::date IS NULL OR m.match_date >= $4)
		    AND mi.batting_team_opposition_id IN (SELECT id FROM club)
		  UNION ALL
		  SELECT bw.player_id, m.match_date
		  FROM bowling_data bw
		  JOIN match_inning mi ON mi.match_id = bw.match_id AND mi.inning_number = bw.inning_number
		  JOIN match m ON m.match_id = bw.match_id
		  WHERE m.format_id = $1 AND m.match_date < $2
		    AND ($4::date IS NULL OR m.match_date >= $4)
		    AND mi.bowling_team_opposition_id IN (SELECT id FROM club)
		), last_played AS (
		  SELECT id, MAX(match_date) AS played_on FROM eligible GROUP BY id
		)
		SELECT p.id, COALESCE(p.external_id, ''), p.player_name, p.is_wicket_keeper,
		       lp.played_on, p.is_retired
		FROM player p
		JOIN last_played lp ON lp.id = p.id
		ORDER BY p.player_name, p.id
	`, formatID, cutoffDate, q.OppositionID, since)
	if err != nil {
		return PlayerPool{}, err
	}
	defer rows.Close()

	pool := PlayerPool{}
	seen := make(map[int64]bool)
	for rows.Next() {
		var row PlayerPoolRow
		var isRetired int16
		if err := rows.Scan(
			&row.PlayerID, &row.ExternalID, &row.PlayerName, &row.IsWicketKeeper,
			&row.LastPlayed, &isRetired,
		); err != nil {
			return PlayerPool{}, err
		}
		if seen[row.PlayerID] {
			continue
		}
		seen[row.PlayerID] = true
		if excluded, ok := q.excludedBy(row, isRetired); ok {
			pool.Excluded = append(pool.Excluded, excluded)
			continue
		}
		pool.Players = append(pool.Players, row)
	}
	if err := rows.Err(); err != nil {
		return PlayerPool{}, err
	}

	extras, err := listPlayerRowsByFormatID(ctx, formatID, q.OppositionID, cutoffDate, q.ExtraPlayerIDs)
	if err != nil {
		return PlayerPool{}, err
	}
	for _, extra := range extras {
		if seen[extra.PlayerID] {
			continue
		}
		seen[extra.PlayerID] = true
		pool.Players = append(pool.Players, extra)
	}

	return pool, nil
}

// excludedBy reports whether the ledger keeps this candidate out of the pool, and why.
//
// The stored fact outranks a claim: a promoted flag is `is_retired = 1` for everybody, so
// it is reported as the fact rather than as the flagging user's opinion. A query with the
// ledger off excludes nobody, which is what a backtest asks for.
func (q PoolQuery) excludedBy(row PlayerPoolRow, isRetired int16) (ExcludedPlayer, bool) {
	if !q.ApplyLedger {
		return ExcludedPlayer{}, false
	}
	excluded := ExcludedPlayer{
		PlayerID:   row.PlayerID,
		PlayerName: row.PlayerName,
		LastPlayed: row.LastPlayed,
	}
	if isRetired != 0 {
		excluded.Reason = availability.ReasonRetired
		excluded.Detail = q.Flags[row.PlayerID].Detail
		return excluded, true
	}
	if flag, ok := q.Flags[row.PlayerID]; ok {
		excluded.Reason = flag.Reason()
		excluded.Detail = flag.Detail
		return excluded, true
	}
	return ExcludedPlayer{}, false
}

// ListPlayerRowsByID resolves players by id, with their last appearance for the club in
// the format before the cutoff where they have one.
//
// It serves the two paths that name players directly rather than by history: the extra
// ids a caller insists on, and a manual pool a user ticked out of the candidate list.
// Both bypass the window and the ledger by design, so this function applies neither.
func ListPlayerRowsByID(
	ctx context.Context,
	formatCode string,
	oppID int64,
	cutoff time.Time,
	playerIDs []int64,
) ([]PlayerPoolRow, error) {
	if len(playerIDs) == 0 {
		return nil, nil
	}
	formatID, err := GetOrCreateMatchFormat(ctx, formatCode)
	if err != nil {
		return nil, err
	}
	return listPlayerRowsByFormatID(ctx, formatID, oppID, availability.CalendarDay(cutoff), playerIDs)
}

// listPlayerRowsByFormatID is ListPlayerRowsByID with the format already resolved, so
// the pool query does not look it up twice.
func listPlayerRowsByFormatID(
	ctx context.Context,
	formatID int64,
	oppID int64,
	cutoff time.Time,
	playerIDs []int64,
) ([]PlayerPoolRow, error) {
	if len(playerIDs) == 0 {
		return nil, nil
	}
	if Pool == nil {
		return nil, errors.New("db pool not initialized")
	}
	out := make([]PlayerPoolRow, 0, len(playerIDs))
	seen := make(map[int64]bool, len(playerIDs))
	for _, playerID := range playerIDs {
		if seen[playerID] {
			continue
		}
		seen[playerID] = true
		row := PlayerPoolRow{PlayerID: playerID}
		var lastPlayed *time.Time
		err := Pool.QueryRow(ctx, `
			WITH club AS (
			  SELECT id FROM opposition WHERE COALESCE(canonical_id, id) = $3
			), played AS (
			  SELECT MAX(m.match_date) AS played_on
			  FROM batting_data bd
			  JOIN match_inning mi ON mi.match_id = bd.match_id AND mi.inning_number = bd.inning_number
			  JOIN match m ON m.match_id = bd.match_id
			  WHERE bd.player_id = $1 AND m.format_id = $2 AND m.match_date < $4
			    AND mi.batting_team_opposition_id IN (SELECT id FROM club)
			  UNION ALL
			  SELECT MAX(m.match_date)
			  FROM bowling_data bw
			  JOIN match_inning mi ON mi.match_id = bw.match_id AND mi.inning_number = bw.inning_number
			  JOIN match m ON m.match_id = bw.match_id
			  WHERE bw.player_id = $1 AND m.format_id = $2 AND m.match_date < $4
			    AND mi.bowling_team_opposition_id IN (SELECT id FROM club)
			)
			SELECT COALESCE(p.external_id, ''), p.player_name, p.is_wicket_keeper,
			       (SELECT MAX(played_on) FROM played)
			FROM player p WHERE p.id = $1
		`, playerID, formatID, oppID, cutoff).
			Scan(&row.ExternalID, &row.PlayerName, &row.IsWicketKeeper, &lastPlayed)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return nil, err
		}
		if lastPlayed != nil {
			row.LastPlayed = *lastPlayed
		}
		out = append(out, row)
	}
	return out, nil
}

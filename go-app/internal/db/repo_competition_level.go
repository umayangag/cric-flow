package db

import (
	"context"
	"errors"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/availability"
)

// A club's level is the level of the matches it has played *before the fixture's day*,
// folded onto its canonical id so a club that renamed reads as one club (I-4). The date
// bound is what makes the read as-of (H-21): a backtest at a 2022 fixture may not learn
// from a side's 2023 matches what level it plays at, the way the pool window and the home
// flag (FEAT-05) read nothing past the cutoff. Only a club whose every recorded match was
// at one level is returned: a club at both levels has no single level to read, and a club
// with none recorded -- a database migrated but not re-imported since 0022, or a side that
// has not played yet -- has none at all. Both are absent rather than guessed.
const competitionLevelsByClubSQL = `
		SELECT club_id, MIN(competition_level)
		FROM (
			SELECT DISTINCT COALESCE(o.canonical_id, o.id) AS club_id, m.competition_level
			FROM match_inning mi
			JOIN match m ON m.match_id = mi.match_id
			JOIN opposition o ON o.id IN (mi.batting_team_opposition_id, mi.bowling_team_opposition_id)
			WHERE COALESCE(o.canonical_id, o.id) = ANY($1)
			  AND m.match_date < $2
			  AND m.competition_level IS NOT NULL
		) levels
		GROUP BY club_id
		HAVING COUNT(*) = 1`

// CompetitionLevelsByClub returns, per club id, the one competition level every match the
// club played before the calendar day of `before` was at. A club that has played at
// both levels, or at none, is absent from the map. Read-only: the prediction path never
// writes.
func CompetitionLevelsByClub(ctx context.Context, clubIDs []int64, before time.Time) (map[int64]string, error) {
	if Pool == nil {
		return nil, errors.New("db pool not initialized")
	}
	rows, err := Pool.Query(ctx, competitionLevelsByClubSQL, clubIDs, availability.CalendarDay(before))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	levels := make(map[int64]string, len(clubIDs))
	for rows.Next() {
		var clubID int64
		var level string
		if err := rows.Scan(&clubID, &level); err != nil {
			return nil, err
		}
		levels[clubID] = level
	}
	return levels, rows.Err()
}

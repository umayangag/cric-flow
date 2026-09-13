package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/umayangag/cric-flow/go-app/internal/trackrecord"
)

// MatchLookup resolves a predicted fixture to the match the import brought (P2-4). It
// implements trackrecord.MatchLookup; there are no rules here, only the SQL.
type MatchLookup struct{}

// NewMatchLookup returns the lookup backed by the process's connection pool.
func NewMatchLookup() *MatchLookup { return &MatchLookup{} }

// Every opposition id this file reads is folded to its club -- COALESCE(canonical_id, id)
// -- before it leaves, because that is the space the record it feeds is written in.
//
// A stored prediction holds `result.Team{1,2}Side.ClubID`, which ResolveTeamSide already
// folds (repo_team_side.go), while the match tables hold whatever the team was called on
// the day: Cricsheet splits a renamed club into two opposition rows and an import writes
// the row belonging to that match. Matching the two spaces against each other is how a
// fixture between a club that has since renamed never resolves, and how a match that does
// resolve on one side scores the other side's half wrong -- team1_won inverted, no
// team1_total, team1_batted_first reversed, an eleven overlap of zero (GO-02). Folding
// here, at the one place the rows are read, keeps trackrecord free of the lineage rule:
// PlayedMatch arrives in club ids and every comparison in build.go is like against like.
const (
	// clubsOnMatch is every club either match table says was on the correlated match `m`.
	// The match table names no teams of its own -- the sides are the batting and bowling
	// teams of its innings and the sides its players fielded for -- so a side is "on the
	// match" when either table says so. A match abandoned without a ball has innings rows
	// with no runs, or none at all, and its players still, which is why both are consulted.
	// The joins are inner because both columns are NOT NULL and foreign keys to
	// opposition: folding widens what resolves and drops nothing.
	clubsOnMatch = `SELECT COALESCE(o.canonical_id, o.id)
			     FROM match_inning i JOIN opposition o ON o.id = i.batting_team_opposition_id
			     WHERE i.match_id = m.match_id
			   UNION
			   SELECT COALESCE(o.canonical_id, o.id)
			     FROM match_player p JOIN opposition o ON o.id = p.opposition_id
			     WHERE p.match_id = m.match_id`

	// findMatchesSQL keys a match on the exact date, the format, the gender and both
	// sides in club-id space. The winner is joined LEFT: it is null for a no-result, a
	// tie or a draw, and folding must leave that null a null.
	findMatchesSQL = `
		SELECT m.match_id, COALESCE(winner.canonical_id, winner.id),
		       m.outcome_by_runs, m.outcome_by_wickets
		FROM match m
		JOIN match_format f ON f.id = m.format_id
		LEFT JOIN opposition winner ON winner.id = m.outcome_winner_opposition_id
		WHERE m.match_date = $1
		  AND f.code = $2
		  AND m.gender = $3
		  AND $4::bigint <> $5::bigint
		  AND $4::bigint IN (` + clubsOnMatch + `)
		  AND $5::bigint IN (` + clubsOnMatch + `)
		ORDER BY m.match_id`
)

// FindMatches returns every match that fits the fixture exactly: the same match_date,
// both of the fixture's clubs on the match in either order, the same format code and
// gender. The fixture's ids are club ids, and so is everything the answer carries.
//
// The date is exact on purpose. A series plays the same two sides days apart, and the
// prediction was about one of those days; a match on a neighbouring date is a different
// match, and scoring a forecast against it would be scoring a claim nobody made. The
// caller reads an empty answer as "not imported yet", and more than one as a
// double-header it cannot tell apart.
func (l *MatchLookup) FindMatches(ctx context.Context, fixture trackrecord.Fixture) ([]trackrecord.PlayedMatch, error) {
	if Pool == nil {
		return nil, errors.New("db pool not initialized")
	}
	rows, err := Pool.Query(ctx, findMatchesSQL,
		fixture.MatchDate, fixture.Format, fixture.Gender, fixture.Team1, fixture.Team2)
	if err != nil {
		return nil, fmt.Errorf("find matches for the fixture: %w", err)
	}
	defer rows.Close()

	matches := make([]trackrecord.PlayedMatch, 0, 1)
	for rows.Next() {
		var match trackrecord.PlayedMatch
		if err := rows.Scan(&match.MatchID, &match.WinnerOppositionID,
			&match.OutcomeByRuns, &match.OutcomeByWickets); err != nil {
			return nil, fmt.Errorf("find matches for the fixture: %w", err)
		}
		matches = append(matches, match)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("find matches for the fixture: %w", err)
	}
	for i := range matches {
		if err := l.fill(ctx, &matches[i]); err != nil {
			return nil, err
		}
	}
	return matches, nil
}

// fill reads the innings and the fielded players of one match, both sided in club ids so
// the caller can compare them against what the prediction stored.
func (l *MatchLookup) fill(ctx context.Context, match *trackrecord.PlayedMatch) error {
	innings, err := Pool.Query(ctx, `
		SELECT i.inning_number, COALESCE(o.canonical_id, o.id), i.runs_scored
		FROM match_inning i JOIN opposition o ON o.id = i.batting_team_opposition_id
		WHERE i.match_id = $1 ORDER BY i.inning_number`, match.MatchID)
	if err != nil {
		return fmt.Errorf("read innings of match %d: %w", match.MatchID, err)
	}
	defer innings.Close()
	for innings.Next() {
		var played trackrecord.PlayedInnings
		if err := innings.Scan(&played.Number, &played.BattingOppositionID, &played.Runs); err != nil {
			return fmt.Errorf("read innings of match %d: %w", match.MatchID, err)
		}
		match.Innings = append(match.Innings, played)
	}
	if err := innings.Err(); err != nil {
		return fmt.Errorf("read innings of match %d: %w", match.MatchID, err)
	}

	fielded, err := Pool.Query(ctx, `
		SELECT p.player_id, COALESCE(o.canonical_id, o.id)
		FROM match_player p JOIN opposition o ON o.id = p.opposition_id
		WHERE p.match_id = $1`, match.MatchID)
	if err != nil {
		return fmt.Errorf("read fielded players of match %d: %w", match.MatchID, err)
	}
	defer fielded.Close()
	match.FieldedPlayers = map[int64]int64{}
	for fielded.Next() {
		var playerID, oppositionID int64
		if err := fielded.Scan(&playerID, &oppositionID); err != nil {
			return fmt.Errorf("read fielded players of match %d: %w", match.MatchID, err)
		}
		match.FieldedPlayers[playerID] = oppositionID
	}
	if err := fielded.Err(); err != nil {
		return fmt.Errorf("read fielded players of match %d: %w", match.MatchID, err)
	}
	return nil
}

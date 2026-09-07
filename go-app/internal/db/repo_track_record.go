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

// FindMatches returns every match that fits the fixture exactly: the same match_date,
// both opposition ids on the match in either order, the same format code and gender.
//
// The date is exact on purpose. A series plays the same two sides days apart, and the
// prediction was about one of those days; a match on a neighbouring date is a different
// match, and scoring a forecast against it would be scoring a claim nobody made. The
// caller reads an empty answer as "not imported yet", and more than one as a
// double-header it cannot tell apart.
//
// The match table names no teams of its own -- the sides are the batting and bowling
// teams of its innings and the sides its players fielded for -- so a side is "on the
// match" when either table says so. A match abandoned without a ball has innings rows
// with no runs, or none at all, and its players still, which is why both are consulted.
func (l *MatchLookup) FindMatches(ctx context.Context, fixture trackrecord.Fixture) ([]trackrecord.PlayedMatch, error) {
	if Pool == nil {
		return nil, errors.New("db pool not initialized")
	}
	rows, err := Pool.Query(ctx, `
		SELECT m.match_id, m.outcome_winner_opposition_id, m.outcome_by_runs, m.outcome_by_wickets
		FROM match m
		JOIN match_format f ON f.id = m.format_id
		WHERE m.match_date = $1
		  AND f.code = $2
		  AND m.gender = $3
		  AND $4 <> $5
		  AND $4 IN (SELECT batting_team_opposition_id FROM match_inning WHERE match_id = m.match_id
		             UNION SELECT opposition_id FROM match_player WHERE match_id = m.match_id)
		  AND $5 IN (SELECT batting_team_opposition_id FROM match_inning WHERE match_id = m.match_id
		             UNION SELECT opposition_id FROM match_player WHERE match_id = m.match_id)
		ORDER BY m.match_id`,
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

// fill reads the innings and the fielded players of one match.
func (l *MatchLookup) fill(ctx context.Context, match *trackrecord.PlayedMatch) error {
	innings, err := Pool.Query(ctx, `
		SELECT inning_number, batting_team_opposition_id, runs_scored
		FROM match_inning WHERE match_id = $1 ORDER BY inning_number`, match.MatchID)
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

	fielded, err := Pool.Query(ctx,
		`SELECT player_id, opposition_id FROM match_player WHERE match_id = $1`, match.MatchID)
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

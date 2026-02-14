package db

import (
	"context"
	"errors"
)

// MatchInsert carries match-level fields for the match table.
type MatchInsert struct {
	MatchID                   int64
	FormatID                  int64
	MatchDate                 string // YYYY-MM-DD
	OriginalMatchType         string
	VenueID                   *int64
	SeasonID                  *int64
	TossWinnerOppositionID    *int64
	TossDecision              *string
	OutcomeWinnerOppositionID *int64
	OutcomeByRuns             *int
	OutcomeByWickets          *int
	EventName                 *string
	MatchNumber               *int
	Gender                    *string
	BallsPerOver              int
	ScheduledOversPerInnings  *int
}

// MatchInningInsert carries inning-level fields for the match_inning table.
type MatchInningInsert struct {
	MatchID                 int64
	InningNumber            int
	BattingTeamOppositionID int64
	BowlingTeamOppositionID int64
	RunsScored              int
	WicketsLost             int
	OversBowled             float32
	BallsBowled             int
	RunRate                 *float32
	TargetRuns              *int
	Extras                  int
	WinnerOppositionID      *int64
}

// UpsertMatch inserts or updates the match table. Call once per match.
func UpsertMatch(ctx context.Context, m *MatchInsert) error {
	if Pool == nil {
		return errors.New("db pool not initialized")
	}
	_, err := Pool.Exec(ctx, `
		INSERT INTO match (
			match_id, format_id, match_date, original_match_type, venue_id, season_id,
			toss_winner_opposition_id, toss_decision, outcome_winner_opposition_id,
			outcome_by_runs, outcome_by_wickets, event_name, match_number, gender,
			balls_per_over, scheduled_overs_per_innings
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		ON CONFLICT (match_id) DO UPDATE SET
			format_id = EXCLUDED.format_id,
			match_date = EXCLUDED.match_date,
			original_match_type = EXCLUDED.original_match_type,
			venue_id = EXCLUDED.venue_id,
			season_id = EXCLUDED.season_id,
			toss_winner_opposition_id = EXCLUDED.toss_winner_opposition_id,
			toss_decision = EXCLUDED.toss_decision,
			outcome_winner_opposition_id = EXCLUDED.outcome_winner_opposition_id,
			outcome_by_runs = EXCLUDED.outcome_by_runs,
			outcome_by_wickets = EXCLUDED.outcome_by_wickets,
			event_name = EXCLUDED.event_name,
			match_number = EXCLUDED.match_number,
			gender = EXCLUDED.gender,
			balls_per_over = EXCLUDED.balls_per_over,
			scheduled_overs_per_innings = EXCLUDED.scheduled_overs_per_innings
	`,
		m.MatchID, m.FormatID, m.MatchDate, m.OriginalMatchType, m.VenueID, m.SeasonID,
		m.TossWinnerOppositionID, m.TossDecision, m.OutcomeWinnerOppositionID,
		m.OutcomeByRuns, m.OutcomeByWickets, m.EventName, m.MatchNumber, m.Gender,
		m.BallsPerOver, m.ScheduledOversPerInnings,
	)
	return err
}

// UpsertMatchInning inserts or updates a match_inning row. Call once per inning.
func UpsertMatchInning(ctx context.Context, mi *MatchInningInsert) error {
	if Pool == nil {
		return errors.New("db pool not initialized")
	}
	_, err := Pool.Exec(ctx, `
		INSERT INTO match_inning (
			match_id, inning_number, batting_team_opposition_id, bowling_team_opposition_id,
			runs_scored, wickets_lost, overs_bowled, balls_bowled, run_rate,
			target_runs, extras, winner_opposition_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (match_id, inning_number) DO UPDATE SET
			batting_team_opposition_id = EXCLUDED.batting_team_opposition_id,
			bowling_team_opposition_id = EXCLUDED.bowling_team_opposition_id,
			runs_scored = EXCLUDED.runs_scored,
			wickets_lost = EXCLUDED.wickets_lost,
			overs_bowled = EXCLUDED.overs_bowled,
			balls_bowled = EXCLUDED.balls_bowled,
			run_rate = EXCLUDED.run_rate,
			target_runs = EXCLUDED.target_runs,
			extras = EXCLUDED.extras,
			winner_opposition_id = EXCLUDED.winner_opposition_id
	`,
		mi.MatchID, mi.InningNumber, mi.BattingTeamOppositionID, mi.BowlingTeamOppositionID,
		mi.RunsScored, mi.WicketsLost, mi.OversBowled, mi.BallsBowled, mi.RunRate,
		mi.TargetRuns, mi.Extras, mi.WinnerOppositionID,
	)
	return err
}

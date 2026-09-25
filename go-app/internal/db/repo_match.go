package db

import (
	"context"
	"errors"
)

// MatchInsert carries match-level fields for the match table.
type MatchInsert struct {
	MatchID   int64
	FormatID  int64
	MatchDate string // YYYY-MM-DD
	// MatchEndDate is the match's last day (YYYY-MM-DD; migration 0024, FEAT-09): the day
	// after which the rating pass folds it into its state, so a Test's later days are not
	// in the state of a fixture played while it was on. MatchDate is its first day.
	MatchEndDate      string
	OriginalMatchType string
	// CompetitionLevel is Cricsheet's info.team_type verbatim -- "international" or
	// "club" -- and MatchTypeNumber the ICC's number for an official international, nil
	// elsewhere (migration 0022). Together with OriginalMatchType they keep a Test apart
	// from a Sheffield Shield round under the one format code both are rated in.
	CompetitionLevel          string
	MatchTypeNumber           *int
	VenueID                   *int64
	SeasonID                  *int64
	TossWinnerOppositionID    *int64
	TossDecision              *string
	OutcomeWinnerOppositionID *int64
	OutcomeByRuns             *int
	OutcomeByWickets          *int
	// Result and ResultMethod are Cricsheet's own words for how the match was decided
	// ("tie", "draw", "no result"; "D/L", "Awarded", ...), nil where the archive says
	// nothing. A winner beside Result "tie" is a tie-breaker win (migration 0017).
	Result                   *string
	ResultMethod             *string
	EventName                *string
	EventStage               *string
	EventGroup               *string
	MatchNumber              *int
	Gender                   *string
	BallsPerOver             int
	ScheduledOversPerInnings *int
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
	TargetOvers             *float32
	Extras                  int
	WinnerOppositionID      *int64
}

// upsertMatchSQL is the one statement both entry points below run. It was written out
// twice, which is how a column added to one of them would reach only half the callers.
const upsertMatchSQL = `
		INSERT INTO match (
			match_id, format_id, match_date, original_match_type, competition_level, match_type_number,
			venue_id, season_id,
			toss_winner_opposition_id, toss_decision, outcome_winner_opposition_id,
			outcome_by_runs, outcome_by_wickets, result, result_method,
			event_name, event_stage, event_group,
			match_number, gender, balls_per_over, scheduled_overs_per_innings, match_end_date
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23)
		ON CONFLICT (match_id) DO UPDATE SET
			format_id = EXCLUDED.format_id,
			match_date = EXCLUDED.match_date,
			original_match_type = EXCLUDED.original_match_type,
			competition_level = EXCLUDED.competition_level,
			match_type_number = EXCLUDED.match_type_number,
			venue_id = EXCLUDED.venue_id,
			season_id = EXCLUDED.season_id,
			toss_winner_opposition_id = EXCLUDED.toss_winner_opposition_id,
			toss_decision = EXCLUDED.toss_decision,
			outcome_winner_opposition_id = EXCLUDED.outcome_winner_opposition_id,
			outcome_by_runs = EXCLUDED.outcome_by_runs,
			outcome_by_wickets = EXCLUDED.outcome_by_wickets,
			result = EXCLUDED.result,
			result_method = EXCLUDED.result_method,
			event_name = EXCLUDED.event_name,
			event_stage = EXCLUDED.event_stage,
			event_group = EXCLUDED.event_group,
			match_number = EXCLUDED.match_number,
			gender = EXCLUDED.gender,
			balls_per_over = EXCLUDED.balls_per_over,
			scheduled_overs_per_innings = EXCLUDED.scheduled_overs_per_innings,
			match_end_date = EXCLUDED.match_end_date
	`

// upsertMatchArgs is the argument list for upsertMatchSQL, in the statement's order.
func upsertMatchArgs(m *MatchInsert) []any {
	return []any{
		m.MatchID, m.FormatID, m.MatchDate, m.OriginalMatchType, m.CompetitionLevel, m.MatchTypeNumber,
		m.VenueID, m.SeasonID,
		m.TossWinnerOppositionID, m.TossDecision, m.OutcomeWinnerOppositionID,
		m.OutcomeByRuns, m.OutcomeByWickets, m.Result, m.ResultMethod,
		m.EventName, m.EventStage, m.EventGroup,
		m.MatchNumber, m.Gender, m.BallsPerOver, m.ScheduledOversPerInnings, m.MatchEndDate,
	}
}

// UpsertMatch inserts or updates the match table. Call once per match.
func UpsertMatch(ctx context.Context, m *MatchInsert) error {
	if Pool == nil {
		return errors.New("db pool not initialized")
	}
	_, err := Pool.Exec(ctx, upsertMatchSQL, upsertMatchArgs(m)...)
	return err
}

// UpsertMatchTx inserts or updates the match table using the given transaction.
func UpsertMatchTx(ctx context.Context, tx CopyFromTx, m *MatchInsert) error {
	return tx.Exec(ctx, upsertMatchSQL, upsertMatchArgs(m)...)
}

// upsertMatchInningSQL is the one statement both entry points below run. Like
// upsertMatchSQL above it used to be written out twice, and target_overs would have been
// the column that reached only half the callers.
const upsertMatchInningSQL = `
		INSERT INTO match_inning (
			match_id, inning_number, batting_team_opposition_id, bowling_team_opposition_id,
			runs_scored, wickets_lost, overs_bowled, balls_bowled, run_rate,
			target_runs, target_overs, extras, winner_opposition_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (match_id, inning_number) DO UPDATE SET
			batting_team_opposition_id = EXCLUDED.batting_team_opposition_id,
			bowling_team_opposition_id = EXCLUDED.bowling_team_opposition_id,
			runs_scored = EXCLUDED.runs_scored,
			wickets_lost = EXCLUDED.wickets_lost,
			overs_bowled = EXCLUDED.overs_bowled,
			balls_bowled = EXCLUDED.balls_bowled,
			run_rate = EXCLUDED.run_rate,
			target_runs = EXCLUDED.target_runs,
			target_overs = EXCLUDED.target_overs,
			extras = EXCLUDED.extras,
			winner_opposition_id = EXCLUDED.winner_opposition_id
	`

// matchInningArgs are upsertMatchInningSQL's bind values, in the statement's order.
func matchInningArgs(mi *MatchInningInsert) []any {
	return []any{
		mi.MatchID, mi.InningNumber, mi.BattingTeamOppositionID, mi.BowlingTeamOppositionID,
		mi.RunsScored, mi.WicketsLost, mi.OversBowled, mi.BallsBowled, mi.RunRate,
		mi.TargetRuns, mi.TargetOvers, mi.Extras, mi.WinnerOppositionID,
	}
}

// UpsertMatchInning inserts or updates a match_inning row. Call once per inning.
func UpsertMatchInning(ctx context.Context, mi *MatchInningInsert) error {
	if Pool == nil {
		return errors.New("db pool not initialized")
	}
	_, err := Pool.Exec(ctx, upsertMatchInningSQL, matchInningArgs(mi)...)
	return err
}

// UpsertMatchInningTx inserts or updates a match_inning row using the given transaction.
func UpsertMatchInningTx(ctx context.Context, tx CopyFromTx, mi *MatchInningInsert) error {
	return tx.Exec(ctx, upsertMatchInningSQL, matchInningArgs(mi)...)
}

package db

import (
	"context"
	"errors"
)

// MatchInfoUpdate carries optional fields to update in match_details.
// Use nil to skip updating a field (keep existing value).
type MatchInfoUpdate struct {
	Score          *int
	Wickets        *int
	Overs          *float32
	Balls          *int
	RPO            *float32
	Target         *int
	Inning         *int
	Result         *int64
	OppositionID   *int64
	MatchDate      *string // YYYY-MM-DD
	BattingSession *string
	BowlingSession *string
	VenueID        *int64
	Extras         *int
	Toss           *string
	SeasonID       *int64
	MatchNumber    *int
}

// UpdateMatchDetails updates match_details for a given match_id using COALESCE logic.
func UpdateMatchDetails(ctx context.Context, matchID int64, u *MatchInfoUpdate) error {
	if Pool == nil {
		return errors.New("db pool not initialized")
	}
	_, err := Pool.Exec(
		ctx,
		`UPDATE match_details SET
		score = COALESCE($2, score),
		wickets = COALESCE($3, wickets),
		overs = COALESCE($4, overs),
		balls = COALESCE($5, balls),
		rpo = COALESCE($6, rpo),
		target = COALESCE($7, target),
		inning = COALESCE($8, inning),
		result = COALESCE($9, result),
		opposition_id = COALESCE($10, opposition_id),
		match_date = COALESCE($11, match_date),
		batting_session = COALESCE($12, batting_session),
		bowling_session = COALESCE($13, bowling_session),
		venue_id = COALESCE($14, venue_id),
		extras = COALESCE($15, extras),
		toss = COALESCE($16, toss),
		season_id = COALESCE($17, season_id),
		match_number = COALESCE($18, match_number)
		WHERE match_id = $1`,
		matchID,
		u.Score,
		u.Wickets,
		u.Overs,
		u.Balls,
		u.RPO,
		u.Target,
		u.Inning,
		u.Result,
		u.OppositionID,
		u.MatchDate,
		u.BattingSession,
		u.BowlingSession,
		u.VenueID,
		u.Extras,
		u.Toss,
		u.SeasonID,
		u.MatchNumber,
	)
	return err
}

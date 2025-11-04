package db

import (
	"context"
	"database/sql"
	"errors"
)

// MatchContext carries the minimal fields needed for feature building.
type MatchContext struct {
	Inning       sql.NullInt64
	Session      sql.NullInt64
	Toss         sql.NullInt64 // already encoded numeric in our exporters; default 0 if NULL
	VenueID      sql.NullInt64
	OppositionID sql.NullInt64
	SeasonID     sql.NullInt64
	MatchNumber  sql.NullInt64
}

// GetMatchContext fetches match context fields from match_details.
func GetMatchContext(ctx context.Context, matchID int64) (*MatchContext, error) {
	if Pool == nil {
		return nil, errors.New("db pool not initialized")
	}
	mc := &MatchContext{}
	err := Pool.QueryRow(ctx, `
		SELECT batting_inning, session, toss, venue_id, opposition_id, season_id, match_number
		FROM match_details WHERE match_id = $1
	`, matchID).Scan(&mc.Inning, &mc.Session, &mc.Toss, &mc.VenueID, &mc.OppositionID, &mc.SeasonID, &mc.MatchNumber)
	if err != nil {
		return nil, err
	}
	return mc, nil
}

// PlayerPoolRow represents a candidate player with consistency and flags.
type PlayerPoolRow struct {
	PlayerID           int64
	PlayerName         string
	IsWicketKeeper     int16
	BattingConsistency sql.NullFloat64
	BowlingConsistency sql.NullFloat64
}

// ListPlayerPoolConsistency returns non-retired players with any non-zero consistency for the given season/format.
func ListPlayerPoolConsistency(ctx context.Context, seasonName, formatCode string) ([]PlayerPoolRow, error) {
	if Pool == nil {
		return nil, errors.New("db pool not initialized")
	}
	sid, err := GetOrCreateSeason(ctx, seasonName)
	if err != nil {
		return nil, err
	}
	fid, err := GetOrCreateMatchFormat(ctx, formatCode)
	if err != nil {
		return nil, err
	}
	rows, err := Pool.Query(ctx, `
		SELECT p.id, p.player_name, p.is_wicket_keeper,
		       COALESCE(pc.batting_consistency,0)::real AS batting_consistency,
		       COALESCE(pc.bowling_consistency,0)::real AS bowling_consistency
		FROM player p
		LEFT JOIN player_consistency_data_fmt pc
		  ON pc.player_id = p.id AND pc.season_id = $1 AND pc.format_id = $2
		WHERE p.is_retired = 0 AND (COALESCE(pc.batting_consistency,0) != 0 OR COALESCE(pc.bowling_consistency,0) != 0)
	`, sid, fid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PlayerPoolRow
	for rows.Next() {
		var r PlayerPoolRow
		if err := rows.Scan(&r.PlayerID, &r.PlayerName, &r.IsWicketKeeper, &r.BattingConsistency, &r.BowlingConsistency); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetPlayerFormFmt returns batting and bowling form for player in given season/format.
func GetPlayerFormFmt(ctx context.Context, playerID, seasonID, formatID int64) (bat float64, bowl float64, err error) {
	if Pool == nil {
		return 0, 0, errors.New("db pool not initialized")
	}
	var b, w sql.NullFloat64
	err = Pool.QueryRow(ctx, `
		SELECT batting_form, bowling_form FROM player_form_data_fmt
		WHERE player_id=$1 AND season_id=$2 AND format_id=$3
	`, playerID, seasonID, formatID).Scan(&b, &w)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, 0, nil
		}
		return 0, 0, err
	}
	if b.Valid {
		bat = b.Float64
	}
	if w.Valid {
		bowl = w.Float64
	}
	return bat, bowl, nil
}

// GetPlayerVenueEffectFmt returns batting_venue and bowling_venue for player at venue/format.
func GetPlayerVenueEffectFmt(
	ctx context.Context,
	playerID, venueID, formatID int64,
) (bat float64, bowl float64, err error) {
	if Pool == nil {
		return 0, 0, errors.New("db pool not initialized")
	}
	var b, w sql.NullFloat64
	err = Pool.QueryRow(ctx, `
		SELECT batting_venue, bowling_venue FROM player_venue_data_fmt
		WHERE player_id=$1 AND venue_id=$2 AND format_id=$3
	`, playerID, venueID, formatID).Scan(&b, &w)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, 0, nil
		}
		return 0, 0, err
	}
	if b.Valid {
		bat = b.Float64
	}
	if w.Valid {
		bowl = w.Float64
	}
	return bat, bowl, nil
}

// GetPlayerOppositionEffectFmt returns batting_opposition and bowling_opposition for player vs opposition/format.
func GetPlayerOppositionEffectFmt(
	ctx context.Context,
	playerID, oppositionID, formatID int64,
) (bat float64, bowl float64, err error) {
	if Pool == nil {
		return 0, 0, errors.New("db pool not initialized")
	}
	var b, w sql.NullFloat64
	err = Pool.QueryRow(ctx, `
		SELECT batting_opposition, bowling_opposition FROM player_opposition_data_fmt
		WHERE player_id=$1 AND opposition_id=$2 AND format_id=$3
	`, playerID, oppositionID, formatID).Scan(&b, &w)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, 0, nil
		}
		return 0, 0, err
	}
	if b.Valid {
		bat = b.Float64
	}
	if w.Valid {
		bowl = w.Float64
	}
	return bat, bowl, nil
}

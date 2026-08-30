package db

import (
	"context"
	"errors"
)

// MatchFeatureContext holds match-level context used to compute features at cutoff (format, venue, season, per-player oppositions).
type MatchFeatureContext struct {
	FormatID   int64
	VenueID    *int64
	SeasonID   *int64
	PlayerOpps []PlayerOpposition
}

// PlayerOpposition holds batting and bowling opposition IDs for a player in a match (from the inning they batted/bowled).
type PlayerOpposition struct {
	PlayerID            int64
	BattingOppositionID *int64 // opposition when batting (bowling_team_opposition_id of that inning)
	BowlingOppositionID *int64 // opposition when bowling (batting_team_opposition_id of that inning)
}

// GetMatchFeatureContext returns format_id, venue_id, season_id and per-player batting/bowling opposition IDs for the match.
// Used by exportqueries.ComputeFeaturesAtCutoffForMatch to compute EWM/consistency/venue/opposition with correct context.
func GetMatchFeatureContext(ctx context.Context, matchID int64) (*MatchFeatureContext, error) {
	if defaultDB == nil {
		return nil, errors.New("db pool not initialized")
	}
	var m MatchFeatureContext
	var venueID, seasonID *int64
	err := QueryRow(ctx, `
		SELECT format_id, venue_id, season_id FROM match WHERE match_id = $1
	`, matchID).Scan(&m.FormatID, &venueID, &seasonID)
	if err != nil {
		return nil, err
	}
	m.VenueID = venueID
	m.SeasonID = seasonID

	// Batting: player_id -> opposition they faced (bowling_team_opposition_id of that inning)
	batOpp := make(map[int64]*int64)
	rows, err := Query(ctx, `
		SELECT bd.player_id, mi.bowling_team_opposition_id
		FROM batting_data bd
		JOIN match_inning mi ON mi.match_id = bd.match_id AND mi.inning_number = bd.inning_number
		WHERE bd.match_id = $1
	`, matchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var pid int64
		var oppID *int64
		if err := rows.Scan(&pid, &oppID); err != nil {
			return nil, err
		}
		batOpp[pid] = oppID
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Bowling: player_id -> opposition they faced (batting_team_opposition_id of that inning)
	bowlOpp := make(map[int64]*int64)
	rows2, err := Query(ctx, `
		SELECT bd.player_id, mi.batting_team_opposition_id
		FROM bowling_data bd
		JOIN match_inning mi ON mi.match_id = bd.match_id AND mi.inning_number = bd.inning_number
		WHERE bd.match_id = $1
	`, matchID)
	if err != nil {
		return nil, err
	}
	defer rows2.Close()
	for rows2.Next() {
		var pid int64
		var oppID *int64
		if err := rows2.Scan(&pid, &oppID); err != nil {
			return nil, err
		}
		bowlOpp[pid] = oppID
	}
	if err := rows2.Err(); err != nil {
		return nil, err
	}

	// Union of all player IDs that batted or bowled
	seen := make(map[int64]struct{})
	for pid := range batOpp {
		seen[pid] = struct{}{}
	}
	for pid := range bowlOpp {
		seen[pid] = struct{}{}
	}
	m.PlayerOpps = make([]PlayerOpposition, 0, len(seen))
	for pid := range seen {
		m.PlayerOpps = append(m.PlayerOpps, PlayerOpposition{
			PlayerID:            pid,
			BattingOppositionID: batOpp[pid],
			BowlingOppositionID: bowlOpp[pid],
		})
	}
	return &m, nil
}

// MatchWinContext holds match-level IDs needed to call the win model (team1 = batting first, team2 = bowling first).
type MatchWinContext struct {
	FormatID          int64
	VenueID           int64
	Team1OppositionID int64
	Team2OppositionID int64
}

// GetMatchWinContext returns format_id, venue_id, team1_opposition_id, team2_opposition_id
// for the match (team1 = batting in inning 1). Used to build win model features for backtest.
func GetMatchWinContext(ctx context.Context, matchID int64) (*MatchWinContext, error) {
	if defaultDB == nil {
		return nil, errors.New("db pool not initialized")
	}
	var out MatchWinContext
	err := QueryRow(ctx, `
		SELECT m.format_id,
			COALESCE(m.venue_id, 0),
			COALESCE(mi.batting_team_opposition_id, 0),
			COALESCE(mi.bowling_team_opposition_id, 0)
		FROM match m
		LEFT JOIN match_inning mi ON mi.match_id = m.match_id AND mi.inning_number = 1
		WHERE m.match_id = $1
	`, matchID).Scan(&out.FormatID, &out.VenueID, &out.Team1OppositionID, &out.Team2OppositionID)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// GetMatchPlayerTeams returns a map of player_id -> team (opposition_name) for the match,
// so predicted runs can be summed by team to derive winner. Includes all squad players:
// batting_team for batters, bowling_team for bowlers, and field-only players via fielding_data
// (using fielding_event to infer inning, or first inning's bowling team as fallback).
// This matches the full squad from GetMatchFullSquadPlayerIDs so buildPredictedScorecard
// has correct team mappings for DNB entries and predicted run totals.
func GetMatchPlayerTeams(ctx context.Context, matchID int64) (map[int64]string, error) {
	if defaultDB == nil {
		return nil, errors.New("db pool not initialized")
	}
	out := make(map[int64]string)
	rows, err := Query(ctx, `
		SELECT bd.player_id, COALESCE(o.opposition_name, '')
		FROM batting_data bd
		JOIN match_inning mi ON mi.match_id = bd.match_id AND mi.inning_number = bd.inning_number
		JOIN opposition o ON o.id = mi.batting_team_opposition_id
		WHERE bd.match_id = $1
		UNION
		SELECT bd.player_id, COALESCE(o.opposition_name, '')
		FROM bowling_data bd
		JOIN match_inning mi ON mi.match_id = bd.match_id AND mi.inning_number = bd.inning_number
		JOIN opposition o ON o.id = mi.bowling_team_opposition_id
		WHERE bd.match_id = $2
	`, matchID, matchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var pid int64
		var team string
		if err := rows.Scan(&pid, &team); err != nil {
			return nil, err
		}
		// First occurrence wins (batting team); if player only bowled they get bowling team
		if _, ok := out[pid]; !ok {
			out[pid] = team
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Include field-only players (in fielding_data but not batting/bowling).
	// Use fielding_event to get inning → bowling_team; fallback to first inning's bowling team.
	rows2, err := Query(ctx, `
		SELECT fd.player_id, COALESCE(
			(SELECT o2.opposition_name FROM fielding_event fe
			 JOIN match_inning mi2 ON mi2.match_id = fe.match_id AND mi2.inning_number = fe.innings
			 JOIN opposition o2 ON o2.id = mi2.bowling_team_opposition_id
			 WHERE fe.match_id = fd.match_id AND fe.fielder_id = fd.player_id
			 LIMIT 1),
			(SELECT o2.opposition_name FROM match_inning mi2
			 JOIN opposition o2 ON o2.id = mi2.bowling_team_opposition_id
			 WHERE mi2.match_id = fd.match_id
			 ORDER BY mi2.inning_number LIMIT 1),
			''
		)
		FROM fielding_data fd
		WHERE fd.match_id = $1
		  AND NOT EXISTS (SELECT 1 FROM batting_data bd WHERE bd.match_id = fd.match_id AND bd.player_id = fd.player_id)
		  AND NOT EXISTS (SELECT 1 FROM bowling_data bd WHERE bd.match_id = fd.match_id AND bd.player_id = fd.player_id)
	`, matchID)
	if err != nil {
		return nil, err
	}
	defer rows2.Close()
	for rows2.Next() {
		var pid int64
		var team string
		if err := rows2.Scan(&pid, &team); err != nil {
			return nil, err
		}
		if _, ok := out[pid]; !ok && team != "" {
			out[pid] = team
		}
	}
	return out, rows2.Err()
}

// GetMatchFullSquadPlayerIDs returns player IDs for all 11 players per team (when available from the match).
// Unions batting_data, bowling_data, and fielding_data so we include every participating player, including
// those who only fielded. This balances predicted runs and win prediction across teams.
// For 2-innings matches this returns 22 players (11 per team). For single-innings, returns what we have.
func GetMatchFullSquadPlayerIDs(ctx context.Context, matchID int64) ([]int64, error) {
	if defaultDB == nil {
		return nil, errors.New("db pool not initialized")
	}
	rows, err := Query(ctx, `
		SELECT DISTINCT player_id FROM (
			SELECT player_id FROM batting_data WHERE match_id = $1
			UNION
			SELECT player_id FROM bowling_data WHERE match_id = $1
			UNION
			SELECT player_id FROM fielding_data WHERE match_id = $1
		) AS players ORDER BY player_id
	`, matchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]int64, 0, 22)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

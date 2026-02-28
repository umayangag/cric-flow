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

// GetMatchPlayerTeams returns a map of player_id -> team (opposition_name) for the match,
// so predicted runs can be summed by team to derive winner. Uses batting_team for batters and bowling_team for bowlers-only.
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
	return out, rows.Err()
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

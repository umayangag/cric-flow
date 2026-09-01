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

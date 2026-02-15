package db

import (
	"context"
	"database/sql"
	"errors"
)

// ScorecardInning holds summary and card rows for one innings.
type ScorecardInning struct {
	InningNumber    int                 `json:"inning_number"`
	BattingTeamName string              `json:"batting_team_name"`
	BowlingTeamName string              `json:"bowling_team_name"`
	RunsScored      int                 `json:"runs_scored"`
	WicketsLost     int                 `json:"wickets_lost"`
	Extras          int                 `json:"extras"`
	TargetRuns      *int                `json:"target_runs,omitempty"`
	Batting         []ScorecardBatting  `json:"batting"`
	Bowling         []ScorecardBowling  `json:"bowling"`
}

// ScorecardBatting is one batting line (player, runs, balls, how out, etc.).
// PlayerID is set for backend use when building predicted scorecards.
type ScorecardBatting struct {
	PlayerID   int64    `json:"player_id,omitempty"`
	PlayerName string   `json:"player_name"`
	Runs       *int     `json:"runs"`
	Balls      *int     `json:"balls"`
	Fours      *int     `json:"fours"`
	Sixes      *int     `json:"sixes"`
	StrikeRate *float32 `json:"strike_rate"`
	HowOut     *string  `json:"how_out"` // description from batting_data
}

// ScorecardBowling is one bowling line (bowler, overs, runs, wickets, etc.).
// PlayerID is set for backend use when building predicted scorecards.
type ScorecardBowling struct {
	PlayerID   int64    `json:"player_id,omitempty"`
	PlayerName string   `json:"player_name"`
	Overs      *float32 `json:"overs"`
	Maidens    *int     `json:"maidens"`
	Runs       *int     `json:"runs"`
	Wickets    *int     `json:"wickets"`
	Economy    *float32 `json:"economy"`
	Wides      *int     `json:"wides"`
	NoBalls    *int     `json:"no_balls"`
	Balls      *int     `json:"balls"` // total balls bowled for strike rate
}

// MatchScorecard holds match-level info and per-innings scorecards.
type MatchScorecard struct {
	MatchID   int64              `json:"match_id"`
	MatchDate string             `json:"match_date"`
	Venue     string             `json:"venue"`
	Innings   []ScorecardInning  `json:"innings"`
}

// GetMatchScorecard returns full scorecard (innings, batting, bowling) for a match.
func GetMatchScorecard(ctx context.Context, matchID int64) (*MatchScorecard, error) {
	if Pool == nil {
		return nil, errors.New("db pool not initialized")
	}

	out := &MatchScorecard{MatchID: matchID}

	// Match date and venue; return NotFound if match does not exist
	if err := Pool.QueryRow(ctx, `
		SELECT COALESCE(m.match_date::text, ''), COALESCE(v.display_name, v.venue_name, '')
		FROM match m
		LEFT JOIN venue v ON v.id = m.venue_id
		WHERE m.match_id = $1
	`, matchID).Scan(&out.MatchDate, &out.Venue); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}

	// Innings summary: team names, runs, wickets, extras, target
	rows, err := Pool.Query(ctx, `
		SELECT
			mi.inning_number,
			COALESCE(o_bat.opposition_name, ''),
			COALESCE(o_bowl.opposition_name, ''),
			mi.runs_scored,
			mi.wickets_lost,
			mi.extras,
			mi.target_runs
		FROM match_inning mi
		LEFT JOIN opposition o_bat ON o_bat.id = mi.batting_team_opposition_id
		LEFT JOIN opposition o_bowl ON o_bowl.id = mi.bowling_team_opposition_id
		WHERE mi.match_id = $1
		ORDER BY mi.inning_number
	`, matchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var innings []ScorecardInning
	for rows.Next() {
		var in ScorecardInning
		var target *int
		if err := rows.Scan(&in.InningNumber, &in.BattingTeamName, &in.BowlingTeamName,
			&in.RunsScored, &in.WicketsLost, &in.Extras, &target); err != nil {
			return nil, err
		}
		in.TargetRuns = target
		innings = append(innings, in)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Batting lines per inning
	for i := range innings {
		batRows, err := Pool.Query(ctx, `
			SELECT bd.player_id, COALESCE(p.player_name, ''), bd.description, bd.runs, bd.balls, bd.fours, bd.sixes, bd.strike_rate
			FROM batting_data bd
			JOIN player p ON p.id = bd.player_id
			WHERE bd.match_id = $1 AND bd.inning_number = $2
			ORDER BY COALESCE(bd.batting_position, 99), bd.player_id
		`, matchID, innings[i].InningNumber)
		if err != nil {
			return nil, err
		}
		for batRows.Next() {
			var b ScorecardBatting
			if err := batRows.Scan(&b.PlayerID, &b.PlayerName, &b.HowOut, &b.Runs, &b.Balls, &b.Fours, &b.Sixes, &b.StrikeRate); err != nil {
				batRows.Close()
				return nil, err
			}
			innings[i].Batting = append(innings[i].Batting, b)
		}
		batRows.Close()
		if err := batRows.Err(); err != nil {
			return nil, err
		}
	}

	// Bowling lines per inning
	for i := range innings {
		bowlRows, err := Pool.Query(ctx, `
			SELECT bw.player_id, COALESCE(p.player_name, ''), bw.overs, bw.maidens, bw.runs, bw.wickets, bw.econ, bw.wides, bw.no_balls, bw.balls
			FROM bowling_data bw
			JOIN player p ON p.id = bw.player_id
			WHERE bw.match_id = $1 AND bw.inning_number = $2
			ORDER BY bw.player_id
		`, matchID, innings[i].InningNumber)
		if err != nil {
			return nil, err
		}
		for bowlRows.Next() {
			var w ScorecardBowling
			if err := bowlRows.Scan(&w.PlayerID, &w.PlayerName, &w.Overs, &w.Maidens, &w.Runs, &w.Wickets, &w.Economy, &w.Wides, &w.NoBalls, &w.Balls); err != nil {
				bowlRows.Close()
				return nil, err
			}
			innings[i].Bowling = append(innings[i].Bowling, w)
		}
		bowlRows.Close()
		if err := bowlRows.Err(); err != nil {
			return nil, err
		}
	}

	out.Innings = innings
	return out, nil
}

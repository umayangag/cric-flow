package backtest

import "github.com/umayangag/cric-flow/go-app/internal/db"

// Candidate represents a played match eligible for backtesting.
type Candidate struct {
	MatchID        int64  `json:"match_id"`
	StableID       string `json:"stable_id"`
	MatchDate      string `json:"match_date"`
	Venue          string `json:"venue"`
	Season         string `json:"season"`
	Format         string `json:"format"`
	Team1          string `json:"team1"`
	Team2          string `json:"team2"`
	WinnerTeamCode string `json:"winner_team_code"`
}

// SelectResponse is the API response for backtest select mode.
type SelectResponse struct {
	Filters    map[string]any `json:"filters"`
	Candidates []Candidate    `json:"candidates"`
}

// PlayerPredictions holds ML-predicted stats for a single player.
type PlayerPredictions struct {
	Runs    float64
	Balls   float64
	Fours   float64
	Sixes   float64
	Wickets float64
	Economy float64
	Catches float64
	RunOuts float64
}

// PlayerActuals holds actual match stats for a single player.
type PlayerActuals struct {
	Runs    float64
	Wickets float64
	Economy float64
	Catches float64
	RunOuts float64
}

// MatchAggregates holds match-level totals.
type MatchAggregates struct {
	Runs           float64
	Wickets        float64
	Extras         float64
	WinnerTeamCode string
}

// PlayerResult is a player-level backtest result with predicted, actual, and error values.
type PlayerResult struct {
	PlayerID  int64              `json:"player_id"`
	Predicted map[string]float64 `json:"predicted"`
	Actual    map[string]float64 `json:"actual"`
	Errors    map[string]float64 `json:"errors"`
}

// EvaluateResponse is the API response for backtest evaluate mode.
type EvaluateResponse struct {
	Filters map[string]any `json:"filters"`
	Match   struct {
		MatchID   int64  `json:"match_id"`
		MatchDate string `json:"match_date"`
	} `json:"match"`
	MatchAggregates struct {
		Predicted map[string]any     `json:"predicted"`
		Actual    map[string]any     `json:"actual"`
		Errors    map[string]float64 `json:"errors"`
	} `json:"match_aggregates,omitempty"`
	Players            []PlayerResult     `json:"players"`
	Metrics            map[string]float64 `json:"metrics"`
	PredictedScorecard *db.MatchScorecard `json:"predicted_scorecard,omitempty"`
}

// AccuracyTrendItemDTO is one match's metrics for the accuracy-trend API response.
type AccuracyTrendItemDTO struct {
	MatchID   int64              `json:"match_id"`
	MatchDate string             `json:"match_date"`
	Format    string             `json:"format"`
	Team1     string             `json:"team1"`
	Team2     string             `json:"team2"`
	Metrics   map[string]float64 `json:"metrics"`
}

// AccuracyTrendResponse is the API response for the accuracy-trend endpoint.
type AccuracyTrendResponse struct {
	Filters     map[string]any         `json:"filters"`
	Count       int                    `json:"count"`
	Results     []AccuracyTrendItemDTO `json:"results"`
	Summary     map[string]float64     `json:"summary"`
	Progressive []map[string]float64   `json:"progressive"`
}

// ContributionRow holds one player-match contribution for CSV export.
type ContributionRow struct {
	BatScore   float64
	BowlScore  float64
	FieldScore float64
	IsKeeper   int
	Format     string
	Target     float64
}

// ExportContributionsRequest is the request body for export-contributions.
type ExportContributionsRequest struct {
	Format   string  `json:"format"`
	Team1    string  `json:"team1"`
	Team2    string  `json:"team2"`
	MatchIDs []int64 `json:"match_ids"`
}

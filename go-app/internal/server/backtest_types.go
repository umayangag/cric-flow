package server

// Backtest selection response
type backtestSelectResponse struct {
	Filters    map[string]any      `json:"filters"`
	Candidates []backtestCandidate `json:"candidates"`
}

// Candidate match for backtesting
type backtestCandidate struct {
	MatchID        int64  `json:"match_id"`
	StableID       string `json:"stable_id"`
	Date           string `json:"date"`
	Venue          string `json:"venue"`
	Season         string `json:"season"`
	Format         string `json:"format"`
	Team1          string `json:"team1"`
	Team2          string `json:"team2"`
	WinnerTeamCode string `json:"winner_team_code"`
}

// Evaluate-mode DTOs
type playerPredictions struct {
	Runs    float64
	Wickets float64
	Economy float64
	Catches float64
	RunOuts float64
}

type playerActuals struct {
	Runs    float64
	Wickets float64
	Economy float64
	Catches float64
	RunOuts float64
}

// Internal struct for match-level aggregates
type matchAggregates struct {
	Runs           float64
	Wickets        float64
	Extras         float64
	WinnerTeamCode string
}

// Backtest evaluate API response
type backtestEvaluateResponse struct {
	Filters map[string]any `json:"filters"`
	Match   struct {
		MatchID int64  `json:"match_id"`
		Date    string `json:"date"`
	} `json:"match"`
	MatchAggregates struct {
		Predicted map[string]any     `json:"predicted"`
		Actual    map[string]any     `json:"actual"`
		Errors    map[string]float64 `json:"errors"`
	} `json:"match_aggregates,omitempty"`
	Players []struct {
		PlayerID  int64              `json:"player_id"`
		Predicted map[string]float64 `json:"predicted"`
		Actual    map[string]float64 `json:"actual"`
		Errors    map[string]float64 `json:"errors"`
	} `json:"players"`
	Metrics map[string]float64 `json:"metrics"`
}

// Accuracy trend DTOs
type accuracyTrendItem struct {
	MatchID int64              `json:"match_id"`
	Date    string             `json:"date"`
	Format  string             `json:"format"`
	Team1   string             `json:"team1"`
	Team2   string             `json:"team2"`
	Metrics map[string]float64 `json:"metrics"`
}

type accuracyTrendResponse struct {
	Filters     map[string]any       `json:"filters"`
	Count       int                  `json:"count"`
	Results     []accuracyTrendItem  `json:"results"`
	Summary     map[string]float64   `json:"summary"`
	Progressive []map[string]float64 `json:"progressive"`
}

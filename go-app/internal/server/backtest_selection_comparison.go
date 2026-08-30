package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/umayangag/cric-flow/go-app/internal/config"
	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/services/predictteam"
	sb "github.com/umayangag/cric-flow/go-app/internal/services/selectionbacktest"
)

// selectionComparisonRequest asks for a greedy-versus-win-probability comparison over
// matches that have already been played.
type selectionComparisonRequest struct {
	Format string `json:"format"`
	Team1  string `json:"team1"`
	Team2  string `json:"team2"`
	// Limit bounds how many matches are selected. Each match is selected once per arm,
	// and a win-probability selection is a search, so this is the difference between a
	// request and an outage.
	Limit         int  `json:"limit,omitempty"`
	MinBowlers    int  `json:"min_bowlers,omitempty"`
	RequireKeeper bool `json:"require_keeper,omitempty"`
}

// defaultSelectionComparisonLimit bounds an unspecified request. Deliberately small:
// each match costs two selections and one of them searches.
const defaultSelectionComparisonLimit = 10

// maxSelectionComparisonLimit is the ceiling a caller cannot raise. The endpoint is
// synchronous, so the real constraint is the server's write timeout.
const maxSelectionComparisonLimit = 50

// backtestSelectionComparisonHandler handles POST /api/backtest/selection-comparison.
//
// It answers "does maximising win probability pick different teams than greedy scoring,
// and does it call more results right?" — see the selectionbacktest package for what
// those numbers can and cannot show.
func (a *App) backtestSelectionComparisonHandler(w http.ResponseWriter, r *http.Request) {
	var body selectionComparisonRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, apiError{Code: "INVALID_JSON", Message: err.Error()})
		return
	}
	format := strings.TrimSpace(strings.ToUpper(body.Format))
	team1 := strings.TrimSpace(body.Team1)
	team2 := strings.TrimSpace(body.Team2)
	if format == "" || team1 == "" || team2 == "" {
		writeJSON(w, http.StatusBadRequest,
			apiError{Code: "INVALID_PARAM", Message: "format, team1, team2 are required"})
		return
	}
	limit := body.Limit
	if limit <= 0 {
		limit = defaultSelectionComparisonLimit
	}
	if limit > maxSelectionComparisonLimit {
		writeJSON(w, http.StatusBadRequest, apiError{
			Code:    "INVALID_PARAM",
			Message: "limit exceeds the maximum of 50; run it in batches",
		})
		return
	}

	matches, err := selectionComparisonMatches(r.Context(), format, team1, team2, limit)
	if err != nil {
		respondErr(w, err)
		return
	}
	if len(matches) == 0 {
		writeJSON(w, http.StatusNotFound, apiError{
			Code:    "NO_MATCHES",
			Message: "no played matches for that format and pair",
			Hint:    "check the team names against /api/backtest/match select mode",
		})
		return
	}

	arms := []sb.Arm{
		{Name: "greedy", Mode: predictteam.SelectionModeGreedy},
		{Name: "winprob", Mode: predictteam.SelectionModeWinProbability},
	}
	selector := newSelectionComparisonSelector(body.MinBowlers, body.RequireKeeper)
	report := sb.Summarize(sb.Run(r.Context(), matches, arms, selector))
	writeJSON(w, http.StatusOK, report)
}

// selectionComparisonMatches lists played matches for the pair, newest first, bounded.
func selectionComparisonMatches(
	ctx context.Context,
	format, team1, team2 string,
	limit int,
) ([]sb.Match, error) {
	candidates, err := listPlayedByFmtTeams(ctx, format, team1, team2)
	if err != nil {
		return nil, err
	}
	if len(candidates) > limit {
		// The most recent matches are the ones whose features are best populated.
		candidates = candidates[len(candidates)-limit:]
	}

	out := make([]sb.Match, 0, len(candidates))
	for i := range candidates {
		candidate := candidates[i]
		out = append(out, sb.Match{
			MatchID:          candidate.MatchID,
			Format:           format,
			Team1:            candidate.Team1,
			Team2:            candidate.Team2,
			MatchDate:        candidate.MatchDate,
			ActualWinner:     candidate.WinnerTeam.String,
			FieldedPlayerIDs: fieldedPlayerIDs(ctx, candidate.MatchID),
		})
	}
	return out, nil
}

// fieldedPlayerIDs returns who actually played, or nil when that is not recorded.
//
// A missing squad is not a failure: the overlap metric is context, and the comparison
// that matters — winner accuracy and divergence between arms — does not need it.
func fieldedPlayerIDs(ctx context.Context, matchID int64) []int64 {
	ids, err := db.GetMatchFullSquadPlayerIDs(ctx, matchID)
	if err != nil {
		return nil
	}
	return ids
}

// newSelectionComparisonSelector runs one arm for one match through the real prediction
// path, so the comparison measures what the API would actually have picked.
func newSelectionComparisonSelector(minBowlers int, requireKeeper bool) sb.Selector {
	if minBowlers <= 0 {
		minBowlers = config.DefaultMinBowlers
	}
	return func(
		ctx context.Context,
		match sb.Match,
		mode predictteam.SelectionMode,
	) (sb.ArmSelection, error) {
		result, err := predictteam.PredictTeams(ctx, predictteam.Input{
			Format:        match.Format,
			Team1:         match.Team1,
			Team2:         match.Team2,
			MatchDate:     match.MatchDate,
			MinBowlers:    minBowlers,
			RequireKeeper: requireKeeper,
			SelectionMode: mode,
		}, mlPredictorAdapter{}, nil)
		if err != nil {
			return sb.ArmSelection{}, err
		}
		return armSelectionFromResult(result), nil
	}
}

// armSelectionFromResult reduces a prediction to the parts the comparison scores.
func armSelectionFromResult(result *predictteam.Result) sb.ArmSelection {
	out := sb.ArmSelection{}
	if result == nil {
		return out
	}
	for _, p := range result.Team1 {
		out.SelectedPlayerIDs = append(out.SelectedPlayerIDs, p.PlayerID)
	}
	for _, p := range result.Team2 {
		out.SelectedPlayerIDs = append(out.SelectedPlayerIDs, p.PlayerID)
	}
	if result.ScorecardSummary != nil {
		out.Team1WinProbability = result.ScorecardSummary.Team1WinProbability
		out.PredictedWinner = result.ScorecardSummary.PredictedWinner
	}
	return out
}

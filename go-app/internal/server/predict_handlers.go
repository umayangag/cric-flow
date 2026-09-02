package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/services/predictteam"
)

// predictTeamRequest holds the parsed request body for team-selection prediction.
type predictTeamRequest struct {
	Format    string `json:"format"`
	Team1     string `json:"team1"`
	Team2     string `json:"team2"`
	Venue     string `json:"venue"`
	MatchDate string `json:"match_date"`
	// Weather is decoded only to refuse it. It is retired (consumer plan W0-3), and an
	// unknown field is silently dropped by encoding/json — so a caller still sending a
	// forecast would get a prediction computed without it and no indication why.
	Weather json.RawMessage `json:"weather,omitempty"`
	// Retired in P-5, decoded for the same reason: the Normal Monte Carlo over top-k XIs
	// and the reconciled scorecard are both gone, replaced by /simulate.
	Simulate               json.RawMessage `json:"simulate,omitempty"`
	UseReconciledScorecard json.RawMessage `json:"use_reconciled_scorecard,omitempty"`
	IncludeBothScorecards  json.RawMessage `json:"include_both_scorecards,omitempty"`
	ExtraTeam1             []int64         `json:"extra_team1"`
	ExtraTeam2             []int64         `json:"extra_team2"`
	MinBowlers             int             `json:"min_bowlers"`
	RequireKeeper          *bool           `json:"require_keeper"`
}

// retiredPredictBodyFields are request fields whose behaviour P-5 deleted. Refusing them
// beats ignoring them: a caller still asking for a reconciled scorecard would otherwise get
// a simulated one and no indication that it asked for something else.
var retiredPredictBodyFields = []struct {
	name  string
	value func(predictTeamRequest) json.RawMessage
}{
	{"weather", func(b predictTeamRequest) json.RawMessage { return b.Weather }},
	{"simulate", func(b predictTeamRequest) json.RawMessage { return b.Simulate }},
	{"use_reconciled_scorecard", func(b predictTeamRequest) json.RawMessage {
		return b.UseReconciledScorecard
	}},
	{"include_both_scorecards", func(b predictTeamRequest) json.RawMessage {
		return b.IncludeBothScorecards
	}},
}

// parsePredictTeamRequest decodes the request body from JSON or query params.
func parsePredictTeamRequest(r *http.Request) (predictTeamRequest, error) {
	var body predictTeamRequest
	if r.Method == http.MethodPost && r.Header.Get("Content-Type") == "application/json" {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return body, err
		}
		for _, field := range retiredPredictBodyFields {
			if err := rejectRetiredBodyField(len(field.value(body)) > 0, field.name); err != nil {
				return body, err
			}
		}
		return body, nil
	}
	q := r.URL.Query()
	body.Format = strings.TrimSpace(q.Get("format"))
	body.Team1 = strings.TrimSpace(q.Get("team1"))
	body.Team2 = strings.TrimSpace(q.Get("team2"))
	body.Venue = strings.TrimSpace(q.Get("venue"))
	body.MatchDate = strings.TrimSpace(q.Get("match_date"))
	if s := q.Get("min_bowlers"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			body.MinBowlers = n
		}
	}
	if s := q.Get("require_keeper"); s != "" {
		v := strings.EqualFold(s, "true") || s == "1"
		body.RequireKeeper = &v
	}
	return body, nil
}

// buildPredictInput converts a parsed request into a predictteam.Input.
func buildPredictInput(body predictTeamRequest, matchDate time.Time) predictteam.Input {
	input := predictteam.Input{
		Format:        body.Format,
		Team1:         body.Team1,
		Team2:         body.Team2,
		Venue:         body.Venue,
		MatchDate:     matchDate,
		ExtraTeam1:    body.ExtraTeam1,
		ExtraTeam2:    body.ExtraTeam2,
		MinBowlers:    body.MinBowlers,
		RequireKeeper: true,
	}
	if body.RequireKeeper != nil {
		input.RequireKeeper = *body.RequireKeeper
	}
	return input
}

// predictTeamSelectionHandler handles POST /api/predict/team-selection.
func (a *App) predictTeamSelectionHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, apiError{Code: "METHOD_NOT_ALLOWED", Message: "POST or GET required"})
		return
	}
	body, err := parsePredictTeamRequest(r)
	if err != nil {
		var retired retiredBodyFieldError
		if errors.As(err, &retired) {
			writeJSON(w, http.StatusBadRequest, apiError{
				Code:    retired.param.Code,
				Message: retired.param.Message,
				Hint:    retired.param.Hint,
			})
			return
		}
		writeJSON(w, http.StatusBadRequest, apiError{Code: "INVALID_JSON", Message: err.Error()})
		return
	}
	if body.Format == "" || body.Team1 == "" || body.Team2 == "" {
		writeJSON(
			w,
			http.StatusBadRequest,
			apiError{Code: "INVALID_PARAM", Message: "format, team1, team2 are required"},
		)
		return
	}
	if body.MatchDate == "" {
		writeJSON(
			w,
			http.StatusBadRequest,
			apiError{Code: "INVALID_PARAM", Message: "match_date is required (RFC3339 or YYYY-MM-DD)"},
		)
		return
	}
	matchDate, err := parseMatchDate(body.MatchDate)
	if err != nil {
		writeJSON(
			w,
			http.StatusBadRequest,
			apiError{Code: "INVALID_PARAM", Message: "match_date must be RFC3339 or YYYY-MM-DD: " + err.Error()},
		)
		return
	}
	result, err := predictteam.PredictTeams(r.Context(), buildPredictInput(body, matchDate), a.mlClient)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func parseMatchDate(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}
	return time.Time{}, errors.New("invalid date format")
}

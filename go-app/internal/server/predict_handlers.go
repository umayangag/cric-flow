package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/services/predictteam"
	"github.com/umayangag/cric-flow/go-app/internal/teams"
)

// predictTeamRequest holds the parsed request body for team-selection prediction.
//
// A side is named by `team1_id` -- the `club_id` the options endpoint returned -- or by
// `team1` plus `team1_gender`. A bare `team1` is honoured only where the format holds one
// side of that name; it used to be honoured always, and the resolver picked the more
// recently active side with nothing but a server log to say so (D-10).
type predictTeamRequest struct {
	Format      string `json:"format"`
	Team1ID     int64  `json:"team1_id"`
	Team2ID     int64  `json:"team2_id"`
	Team1       string `json:"team1"`
	Team2       string `json:"team2"`
	Team1Gender string `json:"team1_gender"`
	Team2Gender string `json:"team2_gender"`
	Venue       string `json:"venue"`
	MatchDate   string `json:"match_date"`
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
	body.Team1ID = parseClubID(q.Get("team1_id"))
	body.Team2ID = parseClubID(q.Get("team2_id"))
	body.Team1Gender = strings.TrimSpace(q.Get("team1_gender"))
	body.Team2Gender = strings.TrimSpace(q.Get("team2_gender"))
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

// parseClubID reads a club id from a query parameter. A value that is not a positive
// integer is no reference at all, and is left zero so the name path -- or the "name a side"
// refusal -- handles it.
func parseClubID(raw string) int64 {
	id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || id <= 0 {
		return 0
	}
	return id
}

// buildPredictInput converts a parsed request into a predictteam.Input.
func buildPredictInput(body predictTeamRequest, matchDate time.Time) predictteam.Input {
	input := predictteam.Input{
		Format:        body.Format,
		Team1:         db.TeamRef{ClubID: body.Team1ID, Name: body.Team1, Gender: body.Team1Gender},
		Team2:         db.TeamRef{ClubID: body.Team2ID, Name: body.Team2, Gender: body.Team2Gender},
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
	if invalid := validateSideReferences(body); invalid != nil {
		writeJSON(w, http.StatusBadRequest, *invalid)
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
		respondPredictErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// validateSideReferences checks that the request names two sides at all, and that any gender
// it carries is one this system stores. It does not resolve them -- that needs the database
// -- it only refuses a request that names no side however it is read.
func validateSideReferences(body predictTeamRequest) *apiError {
	if body.Format == "" || (body.Team1ID == 0 && body.Team1 == "") || (body.Team2ID == 0 && body.Team2 == "") {
		return &apiError{
			Code:    "INVALID_PARAM",
			Message: "format is required, and each side must be named by an id or a name",
			Hint: "send team1_id/team2_id (the club_id from /api/options/teams-by-format), " +
				"or team1/team2 with team1_gender/team2_gender",
		}
	}
	genders := []struct{ field, value string }{
		{"team1_gender", body.Team1Gender},
		{"team2_gender", body.Team2Gender},
	}
	for _, side := range genders {
		if side.value != "" && !teams.IsKnownGender(side.value) {
			return &apiError{
				Code:      "INVALID_PARAM",
				Message:   side.field + " is not a gender this system stores",
				Available: teams.Genders(),
			}
		}
	}
	return nil
}

// respondPredictErr turns the two refusals D-10 introduced into 400s that say what to do
// next, and leaves everything else to respondErr.
//
// Both are the caller's request being unanswerable rather than this service failing: a name
// that means two sides has no single answer, and a fixture whose sides are different genders
// is not a match anyone plays. Answering either with a prediction is the defect.
func respondPredictErr(w http.ResponseWriter, err error) {
	var ambiguous *db.AmbiguousTeamNameError
	if errors.As(err, &ambiguous) {
		writeJSON(w, http.StatusBadRequest, apiError{
			Code:      "TEAM_AMBIGUOUS",
			Message:   ambiguous.Error(),
			Hint:      "send the club_id from /api/options/teams-by-format, or add team1_gender/team2_gender",
			Available: ambiguous.CandidateLabels(),
		})
		return
	}
	var crossGender *predictteam.CrossGenderFixtureError
	if errors.As(err, &crossGender) {
		writeJSON(w, http.StatusBadRequest, apiError{
			Code:    "FIXTURE_CROSS_GENDER",
			Message: crossGender.Error(),
			Hint:    "both sides of a fixture are the same gender; pick two sides from one list",
		})
		return
	}
	if errors.Is(err, db.ErrOppositionNotFound) {
		writeJSON(w, http.StatusBadRequest, apiError{
			Code:    "TEAM_NOT_FOUND",
			Message: err.Error(),
			Hint:    "the side must have played this format; pick one from /api/options/teams-by-format",
		})
		return
	}
	respondErr(w, err)
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

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
	// Team1BatsFirst is the toss (P1-1): true where team1 bats first, false where team2
	// does, absent where it is unknown. Absent is the default and is today's behaviour —
	// the simulator draws half the matches each way and reports `toss_marginalised`.
	Team1BatsFirst *bool `json:"team1_bats_first"`
	// The candidate pool each side is chosen from (D-12). Omitted, each side gets the
	// per-format recency window, which is the default and the fix: the pool used to be
	// all-time and offered players who retired a decade ago.
	Team1Pool poolRequestBody `json:"team1_pool"`
	Team2Pool poolRequestBody `json:"team2_pool"`
}

// poolRequestBody is one side's pool scope as a caller spells it.
//
// `players` is the manual pick: the subset a user ticked out of
// /api/options/candidates. When it is present it is the pool, and neither the window nor
// the ledger applies to it — the user has looked at the candidates and chosen, which is
// better evidence about availability than anything this service holds.
type poolRequestBody struct {
	WindowMonths int     `json:"window_months,omitempty"`
	AllTime      bool    `json:"all_time,omitempty"`
	Players      []int64 `json:"players,omitempty"`
}

// poolRequest converts the body's pool scope into the service's.
func (b poolRequestBody) poolRequest() predictteam.PoolRequest {
	return predictteam.PoolRequest{
		WindowMonths: b.WindowMonths,
		AllTime:      b.AllTime,
		Manual:       b.Players,
	}
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
	batsFirst, apiErr := parseTossParam(q.Get("team1_bats_first"))
	if apiErr != nil {
		return body, *apiErr
	}
	body.Team1BatsFirst = batsFirst
	// The pool scope is readable off the query string too, so a `curl` of the GET form
	// can widen a pool. Manual picking is not: a list of ticked ids belongs in a body.
	pool, apiErr := parsePoolRequest(q.Get("window_months"), q.Get("all_time"), nil)
	if apiErr != nil {
		return body, *apiErr
	}
	body.Team1Pool = poolRequestBody{WindowMonths: pool.WindowMonths, AllTime: pool.AllTime}
	body.Team2Pool = body.Team1Pool
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

// parseTossParam reads the toss off the query string, where the three states are "true",
// "false" and absent.
//
// An unreadable value is refused rather than read as unknown: "bat first" and "we do not
// know" are different questions, and answering the second when the first was asked is the
// silent substitution §8.7 forbids.
func parseTossParam(raw string) (*bool, *apiError) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if strings.EqualFold(raw, "true") || raw == "1" {
		batsFirst := true
		return &batsFirst, nil
	}
	if strings.EqualFold(raw, "false") || raw == "0" {
		batsFirst := false
		return &batsFirst, nil
	}
	return nil, &apiError{
		Code:    "INVALID_PARAM",
		Message: "team1_bats_first must be true or false",
		Hint:    "leave it out where the toss is unknown; the simulator then draws both batting orders",
	}
}

// buildPredictInput converts a parsed request into a predictteam.Input.
func buildPredictInput(body predictTeamRequest, matchDate time.Time, actor string) predictteam.Input {
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
		Team1Pool:     body.Team1Pool.poolRequest(),
		Team2Pool:     body.Team2Pool.poolRequest(),
		// Nil is unknown, which is the marginalised default: the field is nullable all the
		// way down so that "unknown" is a state and not a value standing in for one.
		Team1BatsFirst: body.Team1BatsFirst,
		Actor:          actor,
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
		var invalid apiError
		if errors.As(err, &invalid) {
			writeJSON(w, http.StatusBadRequest, invalid)
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
	result, err := predictteam.PredictTeams(
		r.Context(), buildPredictInput(body, matchDate, actorFrom(r)), a.mlClient)
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
	// A pool too small to field an XI is the caller's scope being too narrow, not a
	// failure here, and after D-12 the recency window is the likely cause. The refusal
	// names the window and the two ways out, because both are the user's to choose.
	var insufficient *predictteam.InsufficientPoolError
	if errors.As(err, &insufficient) {
		writeJSON(w, http.StatusBadRequest, apiError{
			Code:    "POOL_TOO_SMALL",
			Message: insufficient.Error(),
			Hint: "send all_time=true to widen the pool, window_months to change it, " +
				"or team1_pool.players / team2_pool.players to choose the candidates by hand",
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
	// A prediction assembled across a reload has no single date to carry, so it is not
	// carried at all: the state changed under the request, and running it again is the
	// whole remedy (P1-5).
	var runChanged *predictteam.ServedRunChangedError
	if errors.As(err, &runChanged) {
		writeJSON(w, http.StatusConflict, apiError{
			Code:    "SERVED_RUN_CHANGED",
			Message: runChanged.Error(),
			Hint:    "a reload landed while this prediction was being assembled; run it again",
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

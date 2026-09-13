package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/availability"
	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/services/predictteam"
	"github.com/umayangag/cric-flow/go-app/internal/teams"
)

// The two sides "India" names in T20I, with the club ids and the last-played order the
// live database holds. The men's side played most recently, which is the side the old
// resolver silently returned for every request naming "India" (D-10).
var (
	indiaMen   = db.TeamSide{ClubID: 43, Name: "India", Gender: teams.GenderMale}
	indiaWomen = db.TeamSide{ClubID: 132, Name: "India", Gender: teams.GenderFemale}
)

func decodeAPIError(t *testing.T, rec *httptest.ResponseRecorder) apiError {
	t.Helper()
	var body apiError
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&body))
	return body
}

func TestValidateSideReferences(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		body     predictTeamRequest
		wantCode string
		reason   string
	}{
		{
			name:   "a club id names a side",
			body:   predictTeamRequest{Format: "T20I", Team1ID: 43, Team2ID: 7},
			reason: "the id from the options endpoint is the unambiguous reference",
		},
		{
			name: "a name with a gender names a side",
			body: predictTeamRequest{
				Format:      "T20I",
				Team1:       "India",
				Team1Gender: "female",
				Team2:       "Australia",
				Team2Gender: "female",
			},
			reason: "(name, gender) is the team identity the database is keyed by",
		},
		{
			name:   "a bare name is still a reference to resolve",
			body:   predictTeamRequest{Format: "T20I", Team1: "India", Team2: "Australia"},
			reason: "whether it names one side is a question for the database, not for this check",
		},
		{
			name:     "no format",
			body:     predictTeamRequest{Team1ID: 43, Team2ID: 7},
			wantCode: "INVALID_PARAM",
		},
		{
			name:     "a side named by nothing at all",
			body:     predictTeamRequest{Format: "T20I", Team1ID: 43},
			wantCode: "INVALID_PARAM",
		},
		{
			name:     "a gender the database cannot hold",
			body:     predictTeamRequest{Format: "T20I", Team1: "India", Team1Gender: "mixed", Team2: "Australia"},
			wantCode: "INVALID_PARAM",
			reason:   "refusing beats matching no side, which would read as 'no such team'",
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := validateSideReferences(tc.body)

			if tc.wantCode == "" {
				assert.Nil(t, got, tc.reason)
				return
			}
			require.NotNil(t, got, tc.reason)
			assert.Equal(t, tc.wantCode, got.Code)
		})
	}
}

func TestValidateSideReferences_ListsTheGendersItWouldAccept(t *testing.T) {
	t.Parallel()

	got := validateSideReferences(predictTeamRequest{
		Format: "T20I", Team1: "India", Team1Gender: "mixed", Team2: "Australia",
	})

	require.NotNil(t, got)
	assert.Equal(t, teams.Genders(), got.Available, "the error says what could have been sent")
}

// An ambiguous name is refused with both candidates named, because the caller's next move is
// to pick one. Before D-10 this request was answered with the more recently active side and
// a server-log warning nobody reads.
func TestRespondPredictErr_AmbiguousNameIs400NamingBothSides(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	err := fmt.Errorf("resolve team1: %w", &db.AmbiguousTeamNameError{
		Name:       "India",
		FormatCode: "T20I",
		Candidates: []db.TeamSide{indiaMen, indiaWomen},
	})

	respondPredictErr(rec, err)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	body := decodeAPIError(t, rec)
	assert.Equal(t, "TEAM_AMBIGUOUS", body.Code)
	assert.Equal(t, []string{"India (men)", "India (women)"}, body.Available)
	assert.Contains(t, body.Hint, "club_id")
}

func TestRespondPredictErr_CrossGenderFixtureIs400(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	err := &predictteam.CrossGenderFixtureError{Team1: indiaMen, Team2: indiaWomen}

	respondPredictErr(rec, err)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	body := decodeAPIError(t, rec)
	assert.Equal(t, "FIXTURE_CROSS_GENDER", body.Code)
	assert.Contains(t, body.Message, "India (men)")
	assert.Contains(t, body.Message, "India (women)")
}

func TestRespondPredictErr_UnknownSideIs400(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()

	respondPredictErr(rec, fmt.Errorf("resolve team2: %w", db.ErrOppositionNotFound))

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "TEAM_NOT_FOUND", decodeAPIError(t, rec).Code)
}

// Play mode's two refusals (P1-2). Both are the request being unanswerable rather than
// this service failing, and both are refused instead of quietly repaired: an eleven that
// is not an eleven, and a player the side cannot field.
func TestRespondPredictErr_APinnedElevenThatIsNotAnElevenIs400(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	err := fmt.Errorf("resolve fixture: %w", &predictteam.IncompleteXIError{
		Team: "India (men)", Size: 10, Need: 11,
	})

	respondPredictErr(rec, err)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	body := decodeAPIError(t, rec)
	assert.Equal(t, "XI_INCOMPLETE", body.Code)
	assert.Contains(t, body.Message, "10 players pinned")
	assert.Contains(t, body.Hint, "team1_xi")
}

func TestRespondPredictErr_APinnedPlayerTheSideCannotFieldIs400(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	err := &predictteam.UnknownXIPlayerError{Team: "India (men)", PlayerIDs: []int64{404}}

	respondPredictErr(rec, err)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	body := decodeAPIError(t, rec)
	assert.Equal(t, "XI_PLAYER_UNKNOWN", body.Code)
	assert.Contains(t, body.Message, "404")
	assert.Contains(t, body.Hint, "candidates")
}

// B-10: a must-include id the side cannot field is a lock nothing can satisfy, and since
// the ids are enforced the request is refused rather than answered with an eleven that
// leaves the asked-for player out.
func TestRespondPredictErr_AMustIncludeIdTheSideCannotFieldIs400(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	err := fmt.Errorf("resolve fixture: %w",
		&predictteam.UnresolvableMustIncludeError{Team: "India (men)", PlayerIDs: []int64{404}})

	respondPredictErr(rec, err)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	body := decodeAPIError(t, rec)
	assert.Equal(t, "MUST_INCLUDE_UNRESOLVABLE", body.Code)
	assert.Contains(t, body.Message, "404")
	assert.Contains(t, body.Hint, "candidates")
}

// Anything that is not one of D-10's refusals is still this service failing, and still a 500.
func TestRespondPredictErr_LeavesEveryOtherFailureAlone(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()

	respondPredictErr(rec, errors.New("the pool query timed out"))

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, "INTERNAL", decodeAPIError(t, rec).Code)
}

func TestPredictTeamSelectionHandler_RefusesARequestThatNamesNoSide(t *testing.T) {
	t.Parallel()
	app := &App{}
	req := httptest.NewRequest(http.MethodPost, "/api/predict/team-selection", strings.NewReader(
		`{"format":"T20I","team2_id":7,"match_date":"2026-09-10"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	app.predictTeamSelectionHandler(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	body := decodeAPIError(t, rec)
	assert.Equal(t, "INVALID_PARAM", body.Code)
	assert.Contains(t, body.Hint, "team1_id")
}

func TestBuildPredictInput_CarriesBothSideReferencesThrough(t *testing.T) {
	t.Parallel()
	matchDate, err := parseMatchDate("2026-09-10")
	require.NoError(t, err)

	input := buildPredictInput(predictTeamRequest{
		Format:      "T20I",
		Team1ID:     43,
		Team2:       "Australia",
		Team2Gender: "male",
	}, matchDate, availability.DefaultActor, matchDate)

	assert.Equal(t, db.TeamRef{ClubID: 43}, input.Team1)
	assert.Equal(t, db.TeamRef{Name: "Australia", Gender: "male"}, input.Team2)
	assert.Equal(t, availability.DefaultActor, input.Actor)
}

func TestParseClubID(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		raw  string
		want int64
	}{
		{name: "an id", raw: "43", want: 43},
		{name: "padded", raw: " 43 ", want: 43},
		{name: "absent", raw: "", want: 0},
		{name: "not a number", raw: "India", want: 0},
		{name: "not an id a row can have", raw: "-1", want: 0},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, parseClubID(tc.raw))
		})
	}
}

func TestParsePredictTeamRequest_ReadsSideReferencesFromTheQuery(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodGet,
		"/api/predict/team-selection?format=T20I&team1_id=43&team2=Australia&team2_gender=female", nil)

	body, err := parsePredictTeamRequest(req)

	require.NoError(t, err)
	assert.Equal(t, int64(43), body.Team1ID)
	assert.Equal(t, "Australia", body.Team2)
	assert.Equal(t, "female", body.Team2Gender)
}

package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/availability"
)

// TestActorFrom_DefaultsToTheSingleUser keeps one deployment's ledger addressable. The
// header exists because a claim is scoped to a user; its absence is not "nobody".
func TestActorFrom_DefaultsToTheSingleUser(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		header string
		want   string
	}{
		{name: "no header is the default user", want: availability.DefaultActor},
		{name: "blank header is the default user", header: "   ", want: availability.DefaultActor},
		{name: "a named user addresses their own ledger", header: "kate", want: "kate"},
	}

	for i := range testCases {
		testCase := testCases[i]
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			request := httptest.NewRequest(http.MethodGet, "/api/options/candidates", nil)
			if testCase.header != "" {
				request.Header.Set(actorHeader, testCase.header)
			}

			assert.Equal(t, testCase.want, actorFrom(request))
		})
	}
}

// TestParsePoolRequest_ZeroIsTheDefaultNotAnUnboundedPool pins the parameter's meaning.
// Leaving the window out asks for the per-format default; the all-time pool is a value a
// caller spells, because the unbounded pool is the defect.
func TestParsePoolRequest_ZeroIsTheDefaultNotAnUnboundedPool(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		windowMonths string
		allTime      string
		wantMonths   int
		wantAllTime  bool
		wantRefused  bool
	}{
		{name: "nothing asked for is the per-format default"},
		{name: "a window in months", windowMonths: "18", wantMonths: 18},
		{name: "all_time=true widens the pool", allTime: "true", wantAllTime: true},
		{name: "all_time=1 widens it too", allTime: "1", wantAllTime: true},
		{name: "all_time=false leaves the window on", allTime: "false"},
		{
			name:         "a window and a widening together",
			windowMonths: "6",
			allTime:      "true",
			wantMonths:   6,
			wantAllTime:  true,
		},
		{name: "a zero window is refused", windowMonths: "0", wantRefused: true},
		{name: "a negative window is refused", windowMonths: "-3", wantRefused: true},
		{name: "a window that is not a number is refused", windowMonths: "twelve", wantRefused: true},
	}

	for i := range testCases {
		testCase := testCases[i]
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			request, apiErr := parsePoolRequest(testCase.windowMonths, testCase.allTime, nil)

			if testCase.wantRefused {
				require.NotNil(t, apiErr)
				assert.Equal(t, "INVALID_PARAM", apiErr.Code)
				assert.Contains(t, apiErr.Hint, "all_time")
				return
			}
			require.Nil(t, apiErr)
			assert.Equal(t, testCase.wantMonths, request.WindowMonths)
			assert.Equal(t, testCase.wantAllTime, request.AllTime)
		})
	}
}

// TestParsePoolRequest_CarriesTheManualPickThrough keeps manual picking optional and
// exact: the ticked ids are the pool, and nothing here reorders or filters them.
func TestParsePoolRequest_CarriesTheManualPickThrough(t *testing.T) {
	t.Parallel()

	request, apiErr := parsePoolRequest("", "", []int64{9, 4, 7})

	require.Nil(t, apiErr)
	assert.Equal(t, []int64{9, 4, 7}, request.Manual)
}

// TestParseCandidatesDate_DefaultsToToday states what the candidate list is for: planning
// a match now, under the window that would be applied now.
func TestParseCandidatesDate_DefaultsToToday(t *testing.T) {
	t.Parallel()

	fromEmpty, apiErr := parseCandidatesDate("")
	require.Nil(t, apiErr)
	explicit, apiErr := parseCandidatesDate("2026-09-10")
	require.Nil(t, apiErr)
	_, refused := parseCandidatesDate("the tenth")

	assert.Equal(t, time.Now().UTC().Truncate(24*time.Hour), fromEmpty)
	assert.Equal(t, time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC), explicit)
	require.NotNil(t, refused)
	assert.Equal(t, "INVALID_PARAM", refused.Code)
}

// TestCandidatesHandler_RefusesARequestThatNamesNoSide keeps the D-10 discipline: a
// candidate list is a list for one side, and there is no guessing which.
func TestCandidatesHandler_RefusesARequestThatNamesNoSide(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		query string
	}{
		{name: "no format", query: "?club_id=43"},
		{name: "no club id", query: "?format=T20I"},
		{name: "a club id that is not one", query: "?format=T20I&club_id=nope"},
	}

	for i := range testCases {
		testCase := testCases[i]
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			app := &App{}
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/api/options/candidates"+testCase.query, nil)

			app.candidatesHandler(recorder, request)

			assert.Equal(t, http.StatusBadRequest, recorder.Code)
			body := decodeAPIError(t, recorder)
			assert.Equal(t, "INVALID_PARAM", body.Code)
			assert.Contains(t, body.Hint, "club_id")
		})
	}
}

// TestCandidatesHandler_RefusesAnUnreadableWindow answers a bad parameter with the
// parameter's own message rather than with a pool built on a value it did not mean.
func TestCandidatesHandler_RefusesAnUnreadableWindow(t *testing.T) {
	t.Parallel()
	app := &App{}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodGet, "/api/options/candidates?format=T20I&club_id=43&window_months=0", nil)

	app.candidatesHandler(recorder, request)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Equal(t, "INVALID_PARAM", decodeAPIError(t, recorder).Code)
}

// TestRetirementFlagHandler_RefusesAPlayerIdThatIsNotOne stops a malformed path from
// reaching the ledger at all: a claim is about a player, and there is no player here.
func TestRetirementFlagHandler_RefusesAPlayerIdThatIsNotOne(t *testing.T) {
	t.Parallel()
	app := &App{}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/players/nope/retirement", nil)

	app.retirementFlagHandler(recorder, request)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Equal(t, "INVALID_PARAM", decodeAPIError(t, recorder).Code)
}

// TestNewRetirementStatus_SaysWhetherTheClaimBecameAFact is the honest surface at the
// response level: a user who flags a player still playing is told that the exclusion is
// theirs alone, and which checks could not be made.
func TestNewRetirementStatus_SaysWhetherTheClaimBecameAFact(t *testing.T) {
	t.Parallel()
	promotedAt := time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)

	promoted := newRetirementStatus(availability.FlagResult{
		Flag: availability.Flag{
			PlayerID:   7,
			PromotedAt: promotedAt,
			Criterion:  availability.CriterionInactivity,
			Detail:     "no appearance in any format since 2015-03-01 (5-year bound)",
		},
	})
	claimOnly := newRetirementStatus(availability.FlagResult{
		Flag:      availability.Flag{PlayerID: 8},
		Unchecked: []string{availability.CriterionCareerEnd},
		Notes:     []string{"inactivity: last appeared 2026-08-20, inside the 5-year inactivity bound"},
	})

	assert.True(t, promoted.Flagged)
	assert.True(t, promoted.Promoted)
	assert.Equal(t, availability.CriterionInactivity, promoted.Criterion)
	assert.True(t, claimOnly.Flagged)
	assert.False(t, claimOnly.Promoted)
	assert.Equal(t, []string{availability.CriterionCareerEnd}, claimOnly.Unchecked)
	assert.Len(t, claimOnly.Notes, 1)
}

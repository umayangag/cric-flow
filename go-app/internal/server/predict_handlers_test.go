package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// jsonPredictRequest is a POST of the given body to the prediction endpoint, as a caller
// makes it.
func jsonPredictRequest(body string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/api/predict/team-selection", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	return r
}

// The toss has three states, and the difference between them is the whole point of the
// control: absent is "unknown", which is the marginalised behaviour the simulator has
// always had, and the two named states are not the same request (P1-1).
func TestParsePredictTeamRequest_TossHasThreeStates(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		request *http.Request
		want    *bool
	}{
		{
			name:    "absent from the body is unknown",
			request: jsonPredictRequest(`{"format":"T20I","team1_id":1,"team2_id":2,"match_date":"2026-09-10"}`),
			want:    nil,
		},
		{
			name: "team1 bats first",
			request: jsonPredictRequest(
				`{"format":"T20I","team1_id":1,"team2_id":2,"match_date":"2026-09-10","team1_bats_first":true}`),
			want: boolPtr(true),
		},
		{
			name: "team2 bats first",
			request: jsonPredictRequest(
				`{"format":"T20I","team1_id":1,"team2_id":2,"match_date":"2026-09-10","team1_bats_first":false}`),
			want: boolPtr(false),
		},
		{
			name:    "the query form says unknown by saying nothing",
			request: httptest.NewRequest(http.MethodGet, "/api/predict/team-selection?format=T20I", nil),
			want:    nil,
		},
		{
			name: "the query form names the toss",
			request: httptest.NewRequest(
				http.MethodGet, "/api/predict/team-selection?format=T20I&team1_bats_first=false", nil),
			want: boolPtr(false),
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			body, err := parsePredictTeamRequest(tc.request)

			require.NoError(t, err)
			assert.Equal(t, tc.want, body.Team1BatsFirst)
			// The parsed field is what reaches the simulation input, unchanged.
			input := buildPredictInput(body, time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC), "someone")
			assert.Equal(t, tc.want, input.Team1BatsFirst)
		})
	}
}

// A played match is a backtest whether or not the caller calls it one (GO-01): the input
// names the match date as its as-of date, which is what ml-service serves ratings strictly
// before and what turns the retirement ledger off. A match today or later is live and
// names nothing, so the through-today state -- and H-11's verdict on it -- applies.
//
// "Today" is the wall clock's UTC day, read once here and once inside the call; the two
// reads disagree only across a UTC midnight, which is the one moment the "today" cases
// could name a different day than the code did.
func TestBuildPredictInput_NamesThePastMatchDateAsAsOf(t *testing.T) {
	t.Parallel()
	today := time.Now().UTC().Truncate(24 * time.Hour)

	testCases := []struct {
		name      string
		matchDate time.Time
		wantAsOf  time.Time
	}{
		{
			name:      "a match years ago names its date",
			matchDate: time.Date(2019, 7, 14, 0, 0, 0, 0, time.UTC),
			wantAsOf:  time.Date(2019, 7, 14, 0, 0, 0, 0, time.UTC),
		},
		{
			name:      "yesterday names its date",
			matchDate: today.AddDate(0, 0, -1),
			wantAsOf:  today.AddDate(0, 0, -1),
		},
		{
			name:      "a timestamped match keeps the calendar day it was written with",
			matchDate: time.Date(2019, 7, 14, 23, 30, 0, 0, time.FixedZone("IST", 5*3600+1800)),
			wantAsOf:  time.Date(2019, 7, 14, 0, 0, 0, 0, time.UTC),
		},
		{
			name:      "today is live",
			matchDate: today,
			wantAsOf:  time.Time{},
		},
		{
			name:      "an upcoming match is live",
			matchDate: today.AddDate(0, 0, 30),
			wantAsOf:  time.Time{},
		},
		{
			name:      "a match far in the future is live",
			matchDate: time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC),
			wantAsOf:  time.Time{},
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			body := predictTeamRequest{Format: "T20I", Team1ID: 1, Team2ID: 2}

			input := buildPredictInput(body, tc.matchDate, "someone")

			assert.True(t, tc.wantAsOf.Equal(input.AsOf), "as_of: want %v, got %v", tc.wantAsOf, input.AsOf)
			assert.Equal(t, tc.matchDate, input.MatchDate, "the match date itself is untouched")
		})
	}
}

// A toss nobody can read is refused rather than taken as unknown: "bat first" and "we do
// not know" are different questions, and answering the second is a silent substitution.
func TestParsePredictTeamRequest_RefusesATossItCannotRead(t *testing.T) {
	t.Parallel()

	_, err := parsePredictTeamRequest(
		httptest.NewRequest(http.MethodGet, "/api/predict/team-selection?format=T20I&team1_bats_first=maybe", nil))

	require.Error(t, err)
	var invalid apiError
	require.ErrorAs(t, err, &invalid)
	assert.Equal(t, "INVALID_PARAM", invalid.Code)
	assert.Contains(t, invalid.Message, "team1_bats_first")
}

func boolPtr(v bool) *bool {
	return &v
}

// A match date is a day, and the day is the one the caller wrote -- not the day the same
// instant falls on in UTC. An offset-bearing value used to travel on as an instant: the
// pool cutoff was then taken off the truncated instant and came out a day early, while the
// stored prediction kept the caller's day, so the two disagreed about which day the
// fixture was (GO-09).
func TestParseMatchDate_AcceptedSpellings_ReturnTheCallersCalendarDayInUTC(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		input string
		want  time.Time
	}{
		{
			name:  "a UTC timestamp keeps its day",
			input: "2025-06-15T10:30:00Z",
			want:  time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			name:  "a positive offset keeps the day it was written with",
			input: "2025-06-15T10:30:00+05:30",
			want:  time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			name:  "an evening in the Americas is that evening's day, not the next one in UTC",
			input: "2025-03-01T22:00:00-05:00",
			want:  time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:  "a small hour in the Americas is that day, not the one truncation lands on",
			input: "2025-03-01T01:00:00-05:00",
			want:  time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:  "a morning east of UTC is that morning's day, not the previous one",
			input: "2025-03-02T05:00:00+09:00",
			want:  time.Date(2025, 3, 2, 0, 0, 0, 0, time.UTC),
		},
		{
			name:  "a bare date is already the day",
			input: "2025-06-15",
			want:  time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			name:  "surrounding whitespace is trimmed",
			input: "  2025-06-15  ",
			want:  time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC),
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseMatchDate(tc.input)

			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, time.UTC, got.Location(), "the day is carried in UTC")
		})
	}
}

func TestParseMatchDate_UnreadableValue_ReturnsError(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		input string
	}{
		{name: "empty string", input: ""},
		{name: "a day-first spelling", input: "15/06/2025"},
		{name: "a month without a day", input: "2025-06"},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := parseMatchDate(tc.input)

			require.Error(t, err)
		})
	}
}

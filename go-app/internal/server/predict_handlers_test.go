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

func TestParseMatchDate(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		input   string
		want    time.Time
		wantErr bool
	}{
		{
			name:  "rfc3339_full",
			input: "2025-06-15T10:30:00Z",
			want:  time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC),
		},
		{
			name:  "rfc3339_with_offset",
			input: "2025-06-15T10:30:00+05:30",
			want:  time.Date(2025, 6, 15, 10, 30, 0, 0, time.FixedZone("", 5*3600+30*60)),
		},
		{
			name:  "date_only_yyyy_mm_dd",
			input: "2025-06-15",
			want:  time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			name:  "whitespace_trimmed",
			input: "  2025-06-15  ",
			want:  time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			name:    "empty_string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "invalid_format",
			input:   "15/06/2025",
			wantErr: true,
		},
		{
			name:    "partial_date",
			input:   "2025-06",
			wantErr: true,
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseMatchDate(tc.input)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.True(t, tc.want.Equal(got), "expected %v, got %v", tc.want, got)
		})
	}
}

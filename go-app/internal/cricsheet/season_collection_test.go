package cricsheet_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-flow/go-app/internal/cricsheet"
)

func TestSeason_UnmarshalJSON_VariousTypes(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		in   string
		out  string
	}{
		{"string year", `"2019"`, "2019"},
		{"string range", `"2007/08"`, "2007/08"},
		{"int year", `2012`, "2012"},
		{"float intish", `2012.0`, "2012"},
		{"float non-int", `2012.5`, "2012.5"},
		{"null", `null`, ""},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			var s cricsheet.Season
			// Act
			err := s.UnmarshalJSON([]byte(tc.in))
			// Assert
			require.NoError(t, err)
			require.Equal(t, tc.out, string(s))
		})
	}
}

func TestCollection_UnmarshalJSON_Forms(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		in   string
		out  []string
	}{
		{"array strings", `["A","B"]`, []string{"A", "B"}},
		{"array objects", `[{"name":"A"},{"name":"B"}]`, []string{"A", "B"}},
		{"single object", `{"name":"A"}`, []string{"A"}},
		{"single string", `"A"`, []string{"A"}},
		{"mixed array", `[{"name":"A"},"B",{"name":""},123]`, []string{"A", "B"}},
		{"null", `null`, nil},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			var c cricsheet.Collection
			err := c.UnmarshalJSON([]byte(tc.in))
			require.NoError(t, err)
			require.Equal(t, len(tc.out), len(c))
			for i := range tc.out {
				require.Equal(t, tc.out[i], c[i])
			}
		})
	}
}

func TestCollection_UnmarshalJSON_GarbageFallback(t *testing.T) {
	t.Parallel()

	var c cricsheet.Collection
	err := c.UnmarshalJSON([]byte(`{"foo":"bar"}`))
	require.NoError(t, err)
	require.Len(t, c, 0)
}

func TestInfo_UnmarshalJSON_Dates(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		in       string
		wantDate string
		wantErr  bool
	}{
		{
			name:     "standard dates field",
			in:       `{"dates": ["2025-11-07"]}`,
			wantDate: "2025-11-07",
		},
		{
			name:     "match_date field",
			in:       `{"match_date": ["2025-12-25"]}`,
			wantDate: "2025-12-25",
		},
		{
			name:     "both fields - dates takes precedence",
			in:       `{"dates": ["2025-11-07"], "match_date": ["2025-12-25"]}`,
			wantDate: "2025-11-07",
		},
		{
			// IMPORT-17: a file with no date is refused, not placed at 1970-01-01
			// ahead of every real match in the archive.
			name:    "neither field is a parse error",
			in:      `{"venue": "Some Stadium"}`,
			wantErr: true,
		},
		{
			name:     "multiple dates",
			in:       `{"dates": ["2025-11-07", "2025-11-08"]}`,
			wantDate: "2025-11-07",
		},
		{
			name:    "empty dates array is a parse error",
			in:      `{"dates": []}`,
			wantErr: true,
		},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			var info cricsheet.Info
			err := json.Unmarshal([]byte(tc.in), &info)
			require.NoError(t, err)

			date, err := info.MatchDate()

			if tc.wantErr {
				require.Error(t, err)
				require.Empty(t, date)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantDate, date)
		})
	}
}

// FEAT-09: the last listed day is the day the match ended; a file with no date is refused
// as MatchDate refuses it.
func TestInfo_MatchEndDate(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		in       string
		wantDate string
		wantErr  bool
	}{
		{name: "one day is its own end", in: `{"dates": ["2025-11-07"]}`, wantDate: "2025-11-07"},
		{
			name:     "a Test ends on its last listed day",
			in:       `{"dates": ["2025-11-07", "2025-11-08", "2025-11-09"]}`,
			wantDate: "2025-11-09",
		},
		{name: "match_date field", in: `{"match_date": ["2025-12-25", "2025-12-26"]}`, wantDate: "2025-12-26"},
		{name: "no date is a parse error", in: `{"venue": "Some Stadium"}`, wantErr: true},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var info cricsheet.Info
			require.NoError(t, json.Unmarshal([]byte(tc.in), &info))

			endDate, err := info.MatchEndDate()

			if tc.wantErr {
				require.Error(t, err)
				require.Empty(t, endDate)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantDate, endDate)
		})
	}
}

func TestParse_DoesNotPanicOnMinimalJSON(t *testing.T) {
	t.Parallel()

	data := []byte(`{"info":{"teams":["A","B"],"match_type":"T20","team_type":"club","season":"2019"},"innings":[]}`)
	m, err := cricsheet.Parse(bytes.NewReader(data))
	require.NoError(t, err)
	require.NotNil(t, m)
}

func TestParse_ErrorOnBadJSON(t *testing.T) {
	t.Parallel()

	_, err := cricsheet.Parse(bytes.NewReader([]byte("not-json")))
	require.Error(t, err)
}

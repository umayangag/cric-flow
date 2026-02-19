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

	cases := []struct {
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
	for _, tc := range cases {
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

	cases := []struct {
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
	for _, tc := range cases {
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

	cases := []struct {
		name     string
		in       string
		wantDate string
	}{
		{
			"standard dates field",
			`{"dates": ["2025-11-07"]}`,
			"2025-11-07",
		},
		{
			"match_date field",
			`{"match_date": ["2025-12-25"]}`,
			"2025-12-25",
		},
		{
			"both fields - dates takes precedence",
			`{"dates": ["2025-11-07"], "match_date": ["2025-12-25"]}`,
			"2025-11-07",
		},
		{
			"neither field",
			`{"venue": "Some Stadium"}`,
			"1970-01-01",
		},
		{
			"multiple dates",
			`{"dates": ["2025-11-07", "2025-11-08"]}`,
			"2025-11-07",
		},
		{
			"empty dates array",
			`{"dates": []}`,
			"1970-01-01",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var info cricsheet.Info
			err := json.Unmarshal([]byte(tc.in), &info)
			require.NoError(t, err)
			require.Equal(t, tc.wantDate, info.MatchDate())
		})
	}
}

func TestParse_DoesNotPanicOnMinimalJSON(t *testing.T) {
	t.Parallel()

	data := []byte(`{"info":{"teams":["A","B"],"match_type":"T20","season":"2019"},"innings":[]}`)
	m, err := cricsheet.Parse(bytes.NewReader(data))
	require.NoError(t, err)
	require.NotNil(t, m)
}

func TestParse_ErrorOnBadJSON(t *testing.T) {
	t.Parallel()

	_, err := cricsheet.Parse(bytes.NewReader([]byte("not-json")))
	require.Error(t, err)
}

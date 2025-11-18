package etlimporter_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/services/etlimporter"
)

// TestParseBattingCSV follows the gold-standard table-driven style with
// explicit Arrange → Act → Assert and fail-fast require assertions.
func TestParseBattingCSV(t *testing.T) {
	t.Parallel()

	// Shared deterministic fixtures
	good := "player_name,season,format,runs,balls,fours,sixes,position\nA,2019,T20,10,8,1,0,3\nB,2019,odi,20,15,2,1,4\n"
	badHeader := "foo,bar\n1,2\n"
	badRow := "player_name,season,format,runs,balls,fours,sixes,position\nA,2019,T20,abc,8,1,0,3\n"

	type arrangeFn func() *strings.Reader
	type assertFn func(t *testing.T, rowsLen int, err error)

	cases := []struct {
		name    string
		arrange arrangeFn
		assert  assertFn
	}{
		{
			name: "ok two rows",
			arrange: func() *strings.Reader {
				return strings.NewReader(good)
			},
			assert: func(t *testing.T, rowsLen int, err error) {
				require.NoError(t, err)
				require.Equal(t, 2, rowsLen)
			},
		},
		{
			name: "bad header",
			arrange: func() *strings.Reader {
				return strings.NewReader(badHeader)
			},
			assert: func(t *testing.T, _ int, err error) {
				require.Error(t, err)
				require.ErrorContains(t, err, "unexpected batting header")
			},
		},
		{
			name: "bad value",
			arrange: func() *strings.Reader {
				return strings.NewReader(badRow)
			},
			assert: func(t *testing.T, _ int, err error) {
				require.Error(t, err)
				require.ErrorContains(t, err, "runs")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			r := tc.arrange()

			// Act
			rows, err := etlimporter.ParseBattingCSV(r)

			// Assert
			tc.assert(t, len(rows), err)
		})
	}
}

// TestParseBowlingCSV follows the same standard as above.
func TestParseBowlingCSV(t *testing.T) {
	t.Parallel()

	good := "player_name,season,format,overs,balls,maidens,runs,wickets,economy\nA,2019,T20,4,24,0,20,2,5.0\n"
	badHeader := "x,y\n1,2\n"
	badRow := "player_name,season,format,overs,balls,maidens,runs,wickets,economy\nA,2019,T20,xx,24,0,20,2,5.0\n"

	type arrangeFn func() *strings.Reader
	type assertFn func(t *testing.T, rowsLen int, err error)

	cases := []struct {
		name    string
		arrange arrangeFn
		assert  assertFn
	}{
		{
			name: "ok one row",
			arrange: func() *strings.Reader {
				return strings.NewReader(good)
			},
			assert: func(t *testing.T, rowsLen int, err error) {
				require.NoError(t, err)
				require.Equal(t, 1, rowsLen)
			},
		},
		{
			name: "bad header",
			arrange: func() *strings.Reader {
				return strings.NewReader(badHeader)
			},
			assert: func(t *testing.T, _ int, err error) {
				require.Error(t, err)
				require.ErrorContains(t, err, "unexpected bowling header")
			},
		},
		{
			name: "bad value",
			arrange: func() *strings.Reader {
				return strings.NewReader(badRow)
			},
			assert: func(t *testing.T, _ int, err error) {
				require.Error(t, err)
				require.ErrorContains(t, err, "overs")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			r := tc.arrange()

			// Act
			rows, err := etlimporter.ParseBowlingCSV(r)

			// Assert
			tc.assert(t, len(rows), err)
		})
	}
}

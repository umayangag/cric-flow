package cricsheet_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/cricsheet"
)

func TestSourceRef(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		path string
		want string
	}{
		{name: "a bare file name", path: "1130677.json", want: "1130677"},
		{name: "a path", path: "../data/go-app/cricsheet/1130677.json", want: "1130677"},
		{name: "a prefixed Cricsheet id", path: "/tmp/wi_211824.json", want: "wi_211824"},
		{name: "no extension", path: "/tmp/1130677", want: "1130677"},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := cricsheet.SourceRef(tc.path)

			assert.Equal(t, tc.want, got)
		})
	}
}

func TestMatchIDFromSource_IsTheCricsheetMatchID(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		sourceRef string
		want      int64
	}{
		{name: "a seven-digit id", sourceRef: "1130677", want: 1130677},
		{name: "a six-digit id", sourceRef: "211824", want: 211824},
		{name: "the lowest usable id", sourceRef: "1", want: 1},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := cricsheet.MatchIDFromSource(tc.sourceRef, "2018-01-09", "Vidarbha", "Railways")

			assert.Equal(t, tc.want, got, "the source's own id is the identity; nothing is invented")
		})
	}
}

func TestMatchIDFromSource_TwoMatchesOnOneDayBetweenTheSameSidesAreTwoMatches(t *testing.T) {
	t.Parallel()

	// The defect this replaces. 1452624 and 1452625 are Gibraltar vs Serbia on
	// 2024-09-30, a two-match day, and 309 of the 22,734 files share a
	// (date, team, team) with another file. Keyed on content they were one match row
	// holding whichever file's transaction committed last.
	first := cricsheet.MatchIDFromSource("1452624", "2024-09-30", "Serbia", "Gibraltar")
	second := cricsheet.MatchIDFromSource("1452625", "2024-09-30", "Serbia", "Gibraltar")

	assert.NotEqual(t, first, second)
}

func TestMatchIDFromSource_IsDeterministic(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		sourceRef string
	}{
		{name: "a Cricsheet id", sourceRef: "1130677"},
		{name: "a prefixed id, which is derived", sourceRef: "wi_211824"},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			first := cricsheet.MatchIDFromSource(tc.sourceRef, "2022-06-07", "Barbados", "Guyana")
			second := cricsheet.MatchIDFromSource(tc.sourceRef, "2022-06-07", "Barbados", "Guyana")

			assert.Equal(t, first, second, "two imports of one directory must agree")
		})
	}
}

func TestMatchIDFromSource_FallsBackAboveEveryCricsheetIDWhenTheNameIsNotOne(t *testing.T) {
	t.Parallel()

	// 25 files in the dataset are named "wi_211824" and cannot be a bigint. Their derived
	// ids live above the Cricsheet range, so an id below the bound is always a number the
	// match can be looked up by and the two spaces can never be confused.
	testCases := []struct {
		name      string
		sourceRef string
	}{
		{name: "a prefixed Cricsheet id", sourceRef: "wi_211824"},
		{name: "a hand-named file", sourceRef: "match-copy"},
		{name: "an empty name", sourceRef: ""},
		{name: "zero", sourceRef: "0"},
		{name: "a negative number", sourceRef: "-5"},
		{name: "a number past the Cricsheet range", sourceRef: "100000000000"},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := cricsheet.MatchIDFromSource(tc.sourceRef, "2022-06-07", "Barbados", "Guyana")

			assert.GreaterOrEqual(t, got, int64(100000000000))
			assert.Less(t, got, int64(1000000000000))
		})
	}
}

func TestMatchIDFromSource_TheFallbackStillSeparatesTwoFilesThatShareADateAndTeams(t *testing.T) {
	t.Parallel()

	// The derived key includes the file identifier, so it does not reintroduce the
	// collision it exists to survive.
	first := cricsheet.MatchIDFromSource("wi_211824", "2022-06-07", "Barbados", "Guyana")
	second := cricsheet.MatchIDFromSource("wi_211825", "2022-06-07", "Barbados", "Guyana")

	assert.NotEqual(t, first, second)
}

func TestMatchIDFromSource_TheFallbackStillDistinguishesDateAndBattingOrder(t *testing.T) {
	t.Parallel()

	base := cricsheet.MatchIDFromSource("wi_211824", "2022-06-07", "Barbados", "Guyana")
	swapped := cricsheet.MatchIDFromSource("wi_211824", "2022-06-07", "Guyana", "Barbados")
	otherDate := cricsheet.MatchIDFromSource("wi_211824", "2022-06-08", "Barbados", "Guyana")

	require.NotEqual(t, base, swapped)
	require.NotEqual(t, base, otherDate)
}

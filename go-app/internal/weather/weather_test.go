package weather

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-flow/go-app/internal/db"
)

func TestNormalizeVenue(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "empty string", input: "", expected: ""},
		{name: "trim spaces", input: "  Lord's  ", expected: "lord's"},
		{name: "lowercase conversion", input: "Melbourne Cricket Ground", expected: "melbourne cricket ground"},
		{name: "collapse whitespace", input: "  Lords   Stadium  ", expected: "lords stadium"},
		{name: "single word", input: "SCG", expected: "scg"},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Act
			got := NormalizeVenue(tc.input)

			// Assert
			assert.Equal(t, tc.expected, got)
		})
	}
}

func TestEnqueueJob_EmptyVenueReturnsNil(t *testing.T) {
	t.Parallel()

	// Act
	err := EnqueueJob(context.Background(), 1, "", "", 2)

	// Assert
	assert.NoError(t, err)
}

func TestEnqueueJob_DBErrorPropagated(t *testing.T) {
	t.Parallel()

	oldPool := db.Pool
	defer func() { db.Pool = oldPool }()
	db.Pool = nil

	// Act
	err := EnqueueJob(context.Background(), 42, "Melbourne", "MCG", 2)

	// Assert
	require.Error(t, err)
}

func TestBuildSessions(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		inningsCount int
		wantLabels   []string
	}{
		{name: "zero defaults to 2", inningsCount: 0, wantLabels: []string{"inning1", "inning2"}},
		{name: "negative defaults to 2", inningsCount: -1, wantLabels: []string{"inning1", "inning2"}},
		{name: "one inning", inningsCount: 1, wantLabels: []string{"inning1"}},
		{name: "two innings", inningsCount: 2, wantLabels: []string{"inning1", "inning2"}},
		{name: "four innings", inningsCount: 4, wantLabels: []string{"inning1", "inning2", "inning3", "inning4"}},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Act
			got := buildSessions(tc.inningsCount)

			// Assert
			require.Len(t, got, len(tc.wantLabels))
			for j, want := range tc.wantLabels {
				assert.Equal(t, want, got[j].Label, "session[%d].Label", j)
			}
		})
	}
}

func TestFirstNonEmpty(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    []string
		expected string
	}{
		{name: "empty slice", input: []string{}, expected: ""},
		{name: "single empty", input: []string{""}, expected: ""},
		{name: "first non empty", input: []string{"a", "b", "c"}, expected: "a"},
		{name: "skip leading empty", input: []string{"", "", "c"}, expected: "c"},
		{name: "all empty", input: []string{"", "  ", ""}, expected: ""},
		{name: "whitespace only ignored", input: []string{"  ", "\t", "x"}, expected: "x"},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Act
			got := firstNonEmpty(tc.input...)

			// Assert
			assert.Equal(t, tc.expected, got)
		})
	}
}

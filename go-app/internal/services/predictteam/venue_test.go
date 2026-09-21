package predictteam

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResolveVenue_KeepsTheThreeOutcomesApart is GO-08 stated as a test. The lookup used
// to be `if id, err := GetVenueID(...); err == nil { venueID = id }`, which folded a found
// venue, a venue nobody named, an unknown one and a database failure into a single
// answer -- and, because the lookup created what it did not find, an unknown name came
// back as a venue id with no history behind it.
func TestResolveVenue_KeepsTheThreeOutcomesApart(t *testing.T) {
	t.Parallel()
	lookupFailure := errors.New("connection reset by peer")

	testCases := []struct {
		name        string
		venue       string
		lookup      VenueLookup
		wantSummary VenueSummary
		wantLookups int
		wantErrText string
		wantUnknown bool
	}{
		{
			name:  "a venue that is held is used and named",
			venue: "Lord's",
			lookup: func(context.Context, string) (int64, bool, error) {
				return 42, true, nil
			},
			wantSummary: VenueSummary{Resolved: true, VenueID: 42, Name: "Lord's"},
			wantLookups: 1,
		},
		{
			name:        "no venue named is answered without one, and says so",
			venue:       "",
			lookup:      func(context.Context, string) (int64, bool, error) { return 0, false, nil },
			wantSummary: VenueSummary{Note: venueBlindNote},
			wantLookups: 0,
		},
		{
			name:  "a venue nobody holds is refused, not dropped",
			venue: "Lords",
			lookup: func(context.Context, string) (int64, bool, error) {
				return 0, false, nil
			},
			wantLookups: 1,
			wantUnknown: true,
			wantErrText: `no venue named "Lords" is known`,
		},
		{
			name:  "a lookup failure is a failure, never an absent venue",
			venue: "Lord's",
			lookup: func(context.Context, string) (int64, bool, error) {
				return 0, false, lookupFailure
			},
			wantLookups: 1,
			wantErrText: "connection reset by peer",
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			lookups := 0
			counted := func(ctx context.Context, name string) (int64, bool, error) {
				lookups++
				return tc.lookup(ctx, name)
			}

			summary, err := resolveVenue(context.Background(), tc.venue, counted)

			assert.Equal(t, tc.wantLookups, lookups)
			assert.Equal(t, tc.wantSummary, summary)
			assertVenueError(t, err, tc.wantErrText, tc.wantUnknown)
		})
	}
}

// assertVenueError checks the error against what the case expects, so no case needs a
// branch of its own.
func assertVenueError(t *testing.T, err error, wantText string, wantUnknown bool) {
	t.Helper()
	var unknown *UnknownVenueError
	assert.Equal(t, wantUnknown, errors.As(err, &unknown),
		"an unknown venue is reported as UnknownVenueError so the handler can answer 400")
	if wantText == "" {
		require.NoError(t, err)
		return
	}
	require.Error(t, err)
	assert.Contains(t, err.Error(), wantText)
}

// A lookup failure keeps its cause, so an operator reading the 500 sees what broke rather
// than a venue name alone.
func TestResolveVenue_LookupFailure_WrapsTheCause(t *testing.T) {
	t.Parallel()
	cause := errors.New("db pool not initialized")

	_, err := resolveVenue(context.Background(), "Lord's",
		func(context.Context, string) (int64, bool, error) { return 0, false, cause })

	require.Error(t, err)
	assert.ErrorIs(t, err, cause)
	assert.Contains(t, err.Error(), `resolve venue "Lord's"`)
}

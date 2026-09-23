package cricsheet_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/cricsheet"
	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// spyTxForMatchVenue implements db.CopyFromTx and captures the venue_id the match upsert
// writes. It is the seventh argument of upsertMatchSQL (repo_match.go, upsertMatchArgs).
type spyTxForMatchVenue struct {
	sawMatch bool
	venueID  *int64
}

const matchVenueArgIndex = 6

func (t *spyTxForMatchVenue) Exec(_ context.Context, sql string, args ...any) error {
	if strings.Contains(sql, "INSERT INTO match ") && len(args) > matchVenueArgIndex {
		t.sawMatch = true
		if id, ok := args[matchVenueArgIndex].(*int64); ok {
			t.venueID = id
		}
	}
	return nil
}

func (t *spyTxForMatchVenue) Query(_ context.Context, _ string, _ ...any) (db.Rows, error) {
	return nopRows{}, nil
}

func (t *spyTxForMatchVenue) QueryRow(_ context.Context, _ string, _ ...any) db.Row {
	return nopRow{}
}

func (t *spyTxForMatchVenue) CopyFrom(
	_ context.Context,
	_ pgx.Identifier,
	_ []string,
	_ pgx.CopyFromSource,
) (int64, error) {
	return 0, nil
}

func (t *spyTxForMatchVenue) Commit(_ context.Context) error   { return nil }
func (t *spyTxForMatchVenue) Rollback(_ context.Context) error { return nil }

// matchFileWithVenueAndCity is one legal over of T20, parameterised on the two fields this
// test is about.
func matchFileWithVenueAndCity(venue, city string) string {
	return `{
      "info": {
        "balls_per_over": 6,
        "dates": ["2025-11-07"],
        "match_type": "T20",
        "team_type": "club",
        "teams": ["Alpha", "Beta"],
        "venue": "` + venue + `",
        "city": "` + city + `",
        "season": "2025"
      },
      "innings": [
        {"team":"Alpha","overs":[
          {"over":0,"deliveries":[
            {"batter":"A1","bowler":"B1","non_striker":"A2","runs":{"batter":0,"extras":0,"total":0}}
          ]}
        ]}
      ]
    }`
}

// TestImportMatchFile_VenueIDFromVenueFieldOnly pins the city fallback out of existence.
//
// The importer used to take firstNonEmpty(info.venue, info.city), so a match file that
// named no ground was recorded at a venue named after its city -- a row that accumulated
// real familiarity and scoring history under a name no XI ever played at (IMPORT-08).
// match.venue_id is nullable, so the honest record of an unnamed ground is no ground, and
// so is a name that carries no identity once folded.
func TestImportMatchFile_VenueIDFromVenueFieldOnly(t *testing.T) {
	// Not parallel: the import path uses package-level singletons and RunInTxFn.
	testCases := []struct {
		name          string
		venue         string
		city          string
		wantVenueHeld bool
	}{
		{
			name:          "a named ground is recorded",
			venue:         "M.Chinnaswamy Stadium",
			city:          "Bengaluru",
			wantVenueHeld: true,
		},
		{
			name:          "a blank venue with a city is no venue, not a venue named after the city",
			venue:         "",
			city:          "Dublin",
			wantVenueHeld: false,
		},
		{
			name:          "a whitespace venue with a city is no venue either",
			venue:         "   ",
			city:          "Sharjah",
			wantVenueHeld: false,
		},
		{
			name:          "a venue name that folds to nothing identifies no ground",
			venue:         "...",
			city:          "Hambantota",
			wantVenueHeld: false,
		},
		{
			name:          "neither named is no venue",
			venue:         "",
			city:          "",
			wantVenueHeld: false,
		},
	}

	for i := range testCases {
		testCase := testCases[i]
		t.Run(testCase.name, func(t *testing.T) {
			// Arrange
			ctx := context.Background()
			spyTx := &spyTxForMatchVenue{}
			cricsheet.SetRunInTxFn(func(ctx context.Context, inner func(context.Context, db.CopyFromTx) error) error {
				return inner(ctx, spyTx)
			})
			t.Cleanup(func() { cricsheet.SetRunInTxFn(nil) })
			dir := t.TempDir()
			file := writeJSON(t, dir, "match.json", matchFileWithVenueAndCity(testCase.venue, testCase.city))

			// Act
			err := cricsheet.ImportMatchFile(ctx, file, &cricsheet.Options{})

			// Assert
			require.NoError(t, err)
			require.True(t, spyTx.sawMatch, "the match upsert was not reached")
			assertVenueRecorded(t, spyTx.venueID, testCase.wantVenueHeld)
		})
	}
}

// assertVenueRecorded says whether the match upsert carried a venue at all. The id itself
// is 0 in a unit test -- there is no pool behind the entity cache -- so what is under test
// is the pointer, which is the difference between "played at a ground" and "no ground
// named".
func assertVenueRecorded(t *testing.T, venueID *int64, want bool) {
	t.Helper()
	if want {
		require.NotNil(t, venueID, "a named ground must be recorded")
		return
	}
	require.Nil(t, venueID, "no ground was named, so no venue may be recorded")
}

package db

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/db/dbtest"
)

// seedClubsAtTheirLevels builds the smallest world the level lookup can be asked about:
// two national sides, two club sides one of which renamed, one side that has played at
// both levels, one match whose level was never recorded -- and, after the fixture day the
// tests read at, a later match that would change what two of the sides read as.
func seedClubsAtTheirLevels(ctx context.Context, t *testing.T) {
	t.Helper()
	require.NoError(t, Exec(ctx, `INSERT INTO opposition (id, opposition_name, gender, canonical_id) VALUES
		(1, 'India', 'male', NULL),
		(2, 'Australia', 'male', NULL),
		(3, 'Mumbai Indians', 'male', NULL),
		(4, 'Barbados', 'female', NULL),
		(5, 'Delhi Daredevils', 'male', 6),
		(6, 'Delhi Capitals', 'male', NULL)`))
	require.NoError(
		t,
		Exec(
			ctx,
			`INSERT INTO match (match_id, format_id, match_date, original_match_type, gender, competition_level) VALUES
		(1, 2, DATE '2024-01-01', 'ODI', 'male', 'international'),
		(2, 3, DATE '2024-01-02', 'T20', 'male', 'club'),
		(3, 3, DATE '2024-01-03', 'T20', 'female', 'club'),
		(4, 3, DATE '2024-01-04', 'T20', 'female', 'international'),
		(5, 3, DATE '2024-01-05', 'T20', 'male', NULL),
		(6, 3, DATE '2024-02-01', 'T20', 'male', 'club')`,
		),
	)
	require.NoError(
		t,
		Exec(
			ctx,
			`INSERT INTO match_inning (match_id, inning_number, batting_team_opposition_id, bowling_team_opposition_id) VALUES
		(1, 1, 1, 2), (1, 2, 2, 1),
		(2, 1, 3, 5), (2, 2, 5, 3),
		(3, 1, 4, 3),
		(4, 1, 4, 1),
		(5, 1, 3, 6),
		(6, 1, 1, 6)`,
		),
	)
}

func TestCompetitionLevelsByClub_ReadsOneLevelPerClubBeforeTheFixtureDay_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	ctx := connectAndMigrateForBiography(t)
	seedClubsAtTheirLevels(ctx, t)
	clubs := []int64{1, 2, 3, 4, 6, 99}

	testCases := []struct {
		name   string
		before time.Time
		want   map[int64]string
	}{
		{
			name:   "at a fixture before India's club outing, India is international and Delhi has one club match",
			before: time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC),
			want: map[int64]string{
				1: "international",
				2: "international",
				// Mumbai's unrecorded match 5 is not a level; its recorded matches are all club.
				3: "club",
				// Delhi renamed: the lookup folds the old row onto the canonical club.
				6: "club",
				// Barbados (4) has played at both levels and 99 has played nothing: both absent.
			},
		},
		{
			name:   "at a fixture after it, India has no single level and nothing earlier changed",
			before: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
			want:   map[int64]string{2: "international", 3: "club", 6: "club"},
		},
		{
			name:   "on the day of the first match, nothing has been played yet",
			before: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			want:   map[int64]string{},
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			levels, err := CompetitionLevelsByClub(ctx, clubs, tc.before)

			require.NoError(t, err)
			assert.Equal(t, tc.want, levels)
		})
	}
}

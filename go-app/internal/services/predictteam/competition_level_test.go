package predictteam

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/formats"
)

// levelsOf is a lookup answering from a fixed table, for the resolution to be tested
// without a database.
func levelsOf(table map[int64]string) CompetitionLevelLookup {
	return func(_ context.Context, clubIDs []int64) (map[int64]string, error) {
		out := make(map[int64]string, len(clubIDs))
		for _, id := range clubIDs {
			if level, ok := table[id]; ok {
				out[id] = level
			}
		}
		return out, nil
	}
}

func TestResolveCompetitionLevel_ReadsTheLevelOffBothSidesHistoryOrSaysWhyNot(t *testing.T) {
	t.Parallel()
	india := db.TeamSide{ClubID: 1, Name: "India", Gender: "male"}
	australia := db.TeamSide{ClubID: 2, Name: "Australia", Gender: "male"}
	mumbai := db.TeamSide{ClubID: 3, Name: "Mumbai Indians", Gender: "male"}
	barbados := db.TeamSide{ClubID: 4, Name: "Barbados", Gender: "female"}
	table := map[int64]string{
		1: formats.CompetitionInternational,
		2: formats.CompetitionInternational,
		3: formats.CompetitionClub,
		// 4 has played at both levels and so has no single one on record.
	}

	testCases := []struct {
		name        string
		team1       db.TeamSide
		team2       db.TeamSide
		wantLevel   string
		wantReading string
		wantNote    string
	}{
		{
			name:        "two national sides read international",
			team1:       india,
			team2:       australia,
			wantLevel:   formats.CompetitionInternational,
			wantReading: CompetitionLevelReadingSidesHistory,
		},
		{
			name:        "two club sides read club",
			team1:       mumbai,
			team2:       db.TeamSide{ClubID: 3, Name: "Mumbai Indians", Gender: "male"},
			wantLevel:   formats.CompetitionClub,
			wantReading: CompetitionLevelReadingSidesHistory,
		},
		{
			name:        "a side with no single level on record is averaged over both",
			team1:       india,
			team2:       barbados,
			wantReading: CompetitionLevelReadingMarginalised,
			wantNote:    "Barbados (women) has played at no single competition level on record",
		},
		{
			name:        "two sides whose levels disagree are averaged over both",
			team1:       india,
			team2:       mumbai,
			wantReading: CompetitionLevelReadingMarginalised,
			wantNote:    "India (men) plays international cricket and Mumbai Indians (men) plays club cricket",
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			summary, err := resolveCompetitionLevel(context.Background(), tc.team1, tc.team2, levelsOf(table))

			require.NoError(t, err)
			assert.Equal(t, tc.wantLevel, summary.Level)
			assert.Equal(t, tc.wantReading, summary.Reading)
			assert.Contains(t, summary.Note, tc.wantNote)
			assert.Equal(t, tc.wantNote == "", summary.Note == "", "a note only where the level was averaged")
		})
	}
}

func TestResolveCompetitionLevel_ReturnsALookupFailureAsItself(t *testing.T) {
	t.Parallel()
	failing := func(context.Context, []int64) (map[int64]string, error) { return nil, errors.New("connection reset") }

	_, err := resolveCompetitionLevel(context.Background(), indiaMen, australiaMen, failing)

	require.Error(t, err, "a database fault is not served as a level nobody could read")
	assert.Contains(t, err.Error(), "connection reset")
}

func TestRefuseCompetitionLevelMismatch_RefusesAnAnswerReadAtTheWrongLevel(t *testing.T) {
	t.Parallel()
	named := CompetitionLevelSummary{Level: formats.CompetitionClub, Reading: CompetitionLevelReadingSidesHistory}
	unnamed := CompetitionLevelSummary{Reading: CompetitionLevelReadingMarginalised}

	testCases := []struct {
		name         string
		summary      CompetitionLevelSummary
		marginalised bool
		wantErr      bool
	}{
		{name: "a named level read as named", summary: named, marginalised: false, wantErr: false},
		{name: "no level, averaged as asked", summary: unnamed, marginalised: true, wantErr: false},
		{name: "a named level averaged over anyway", summary: named, marginalised: true, wantErr: true},
		{name: "no level sent yet read at one", summary: unnamed, marginalised: false, wantErr: true},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := refuseCompetitionLevelMismatch("win probability", tc.summary, tc.marginalised)

			assert.Equal(t, tc.wantErr, err != nil, "%v", err)
		})
	}
}

func TestCompetitionLevelReadings_AreTheTwoTheContractPublishes(t *testing.T) {
	t.Parallel()
	assert.Equal(t, []string{CompetitionLevelReadingSidesHistory, CompetitionLevelReadingMarginalised},
		CompetitionLevelReadings())
}

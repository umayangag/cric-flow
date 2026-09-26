package predictteam

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/formats"
)

var fixtureDay = time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)

// levelsOf is a lookup answering from a fixed table, for the resolution to be tested
// without a database; it records the day it was asked to read before.
func levelsOf(table map[int64]string, askedBefore *time.Time) CompetitionLevelLookup {
	return func(_ context.Context, clubIDs []int64, before time.Time) (map[int64]string, error) {
		if askedBefore != nil {
			*askedBefore = before
		}
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
	chennai := db.TeamSide{ClubID: 5, Name: "Chennai Super Kings", Gender: "male"}
	barbados := db.TeamSide{ClubID: 4, Name: "Barbados", Gender: "female"}
	table := map[int64]string{
		1: formats.CompetitionInternational,
		2: formats.CompetitionInternational,
		3: formats.CompetitionClub,
		5: formats.CompetitionClub,
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
			team2:       chennai,
			wantLevel:   formats.CompetitionClub,
			wantReading: CompetitionLevelReadingSidesHistory,
		},
		{
			name:        "a side with no single level on record is averaged over both",
			team1:       india,
			team2:       barbados,
			wantReading: CompetitionLevelReadingMarginalised,
			wantNote:    "Barbados (women) has played at no single competition level on record before 2024-06-01",
		},
		{
			name:        "a side never seen before is averaged over both, and the answer says so",
			team1:       india,
			team2:       db.TeamSide{ClubID: 99, Name: "Newland", Gender: "male"},
			wantReading: CompetitionLevelReadingMarginalised,
			wantNote:    "Newland (men) has played at no single competition level on record before 2024-06-01",
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

			summary, err := resolveCompetitionLevel(context.Background(), tc.team1, tc.team2, fixtureDay, levelsOf(table, nil))

			require.NoError(t, err)
			assert.Equal(t, tc.wantLevel, summary.Level)
			assert.Equal(t, tc.wantReading, summary.Reading)
			assert.Contains(t, summary.Note, tc.wantNote)
			assert.Equal(t, tc.wantNote == "", summary.Note == "", "a note only where the level was averaged")
		})
	}
}

func TestResolveCompetitionLevel_ReadsTheHistoryBeforeTheFixturesDayOnly(t *testing.T) {
	t.Parallel()
	var askedBefore time.Time

	_, err := resolveCompetitionLevel(context.Background(), indiaMen, australiaMen, fixtureDay,
		levelsOf(map[int64]string{}, &askedBefore))

	require.NoError(t, err)
	assert.Equal(t, fixtureDay, askedBefore, "the lookup is bounded by the fixture's day (H-21)")
}

func TestResolveCompetitionLevel_ReturnsALookupFailureAsItself(t *testing.T) {
	t.Parallel()
	failing := func(context.Context, []int64, time.Time) (map[int64]string, error) {
		return nil, errors.New("connection reset")
	}

	_, err := resolveCompetitionLevel(context.Background(), indiaMen, australiaMen, fixtureDay, failing)

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

func TestApplyXISimulation_SendsTheCompetitionLevelAndRefusesAnAnswerReadAtAnotherOne(t *testing.T) {
	t.Parallel()
	read := CompetitionLevelSummary{Level: formats.CompetitionClub, Reading: CompetitionLevelReadingSidesHistory}
	unread := CompetitionLevelSummary{Reading: CompetitionLevelReadingMarginalised, Note: "no single level"}

	testCases := []struct {
		name         string
		level        CompetitionLevelSummary
		marginalised bool
		wantErr      string
	}{
		{name: "a level read is sent and answered at", level: read, marginalised: false},
		{name: "no level is sent as none and averaged", level: unread, marginalised: true},
		{name: "a level sent but averaged over is refused", level: read, marginalised: true,
			wantErr: "competition level club was sent but ml-service reports competition_level_marginalised=true"},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sim := simulationResult(winProbabilitySourceDisplay)
			sim.CompetitionLevelMarginalised = tc.marginalised
			simulator := &fakeSimulator{result: sim}
			fix := twoSidedFixture("T20")
			fix.competitionLevel = tc.level

			err := applyXISimulation(context.Background(), simulator, fix, []string{"a1"}, []string{"b1"},
				resultWithOnePlayerEachSide())

			assert.Equal(t, tc.level.Level, simulator.req.CompetitionLevel, "the level reaches /simulate")
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}

func TestNewWinRequest_CarriesTheCompetitionLevelTheSidesHistoryGave(t *testing.T) {
	t.Parallel()
	fix := twoSidedFixture("ODI")
	fix.competitionLevel = CompetitionLevelSummary{
		Level: formats.CompetitionInternational, Reading: CompetitionLevelReadingSidesHistory,
	}

	req := newWinRequest(fix, xiSelection{Team1Keys: []string{"a1"}, Team2Keys: []string{"b1"}})

	assert.Equal(t, formats.CompetitionInternational, req.CompetitionLevel, "the level reaches /xi/predict-win")
}

func TestCompetitionLevelReadings_AreTheTwoTheContractPublishes(t *testing.T) {
	t.Parallel()
	assert.Equal(t, []string{CompetitionLevelReadingSidesHistory, CompetitionLevelReadingMarginalised},
		CompetitionLevelReadings())
}

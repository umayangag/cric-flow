package cricsheet_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/cricsheet"
	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/formats"
)

// Positions in upsertMatchSQL's argument list: the competition level and the ICC match
// number follow original_match_type.
const (
	matchArgCompetitionLevel = 4
	matchArgMatchTypeNumber  = 5
)

// matchFileForCompetition is one legal over of cricket, parameterised on the three fields
// this file's tests are about. teamType "" leaves `team_type` out of the file altogether;
// matchTypeNumber 0 leaves `match_type_number` out.
func matchFileForCompetition(matchType, teamType, team1, team2 string, matchTypeNumber int) string {
	teamTypeField := ""
	if teamType != "" {
		teamTypeField = `"team_type": "` + teamType + `",`
	}
	numberField := ""
	if matchTypeNumber != 0 {
		numberField = `"match_type_number": ` + strconv.Itoa(matchTypeNumber) + `,`
	}
	return `{
      "info": {
        "balls_per_over": 6,
        "dates": ["2024-06-04"],
        "match_type": "` + matchType + `",
        ` + teamTypeField + `
        ` + numberField + `
        "gender": "male",
        "teams": ["` + team1 + `", "` + team2 + `"],
        "venue": "Grand Prairie Stadium",
        "city": "Dallas",
        "season": "2024"
      },
      "innings": [
        {"team":"` + team1 + `","overs":[
          {"over":0,"deliveries":[
            {"batter":"A1","bowler":"B1","non_striker":"A2","runs":{"batter":1,"extras":0,"total":1}}
          ]}
        ]}
      ]
    }`
}

// TestImportMatchFile_RecordsTheCompetitionLevelTheFileStates pins IMPORT-09's record: the
// match upsert carries Cricsheet's team_type verbatim and the ICC's match number where the
// file has one, for sides no hand list ever named.
func TestImportMatchFile_RecordsTheCompetitionLevelTheFileStates(t *testing.T) {
	// Not parallel: uses package-level singletons (db.PoolAPI, SetRunInTxFn).
	testCases := []struct {
		name            string
		file            string
		wantLevel       string
		wantMatchNumber *int
	}{
		{
			name:            "a T20 between two national sides is international, with its T20I number",
			file:            matchFileForCompetition("T20", "international", "Netherlands", "Nepal", 2411),
			wantLevel:       formats.CompetitionInternational,
			wantMatchNumber: intPtr(2411),
		},
		{
			name:            "a franchise T20 is club, with no number",
			file:            matchFileForCompetition("T20", "club", "Alpha", "Beta", 0),
			wantLevel:       formats.CompetitionClub,
			wantMatchNumber: nil,
		},
		{
			name:            "a first-class round is club even though it is rated as TEST",
			file:            matchFileForCompetition("MDM", "club", "Victoria", "Tasmania", 0),
			wantLevel:       formats.CompetitionClub,
			wantMatchNumber: nil,
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			ctx := context.Background()
			spy := arrangeMatchUpsertSpy(t)
			file := writeJSON(t, t.TempDir(), "9000030.json", tc.file)

			// Act
			err := cricsheet.ImportMatchFile(ctx, file, &cricsheet.Options{})

			// Assert
			require.NoError(t, err)
			require.Len(
				t,
				spy.matchArgs,
				matchArgCount,
				"the match upsert carries the competition level and match number",
			)
			assert.Equal(t, tc.wantLevel, spy.matchArgs[matchArgCompetitionLevel])
			assert.Equal(t, tc.wantMatchNumber, spy.matchArgs[matchArgMatchTypeNumber])
		})
	}
}

// TestImportMatchFile_RefusesAFileWithoutACompetitionLevel: a T20 with no team_type cannot be
// placed as T20 or T20I, and guessing from the team names is the list this replaced. The
// file is refused before anything is written, and the error names the field.
func TestImportMatchFile_RefusesAFileWithoutACompetitionLevel(t *testing.T) {
	// Not parallel: uses package-level singletons (db.PoolAPI, SetRunInTxFn).
	testCases := []struct {
		name      string
		file      string
		wantError string
	}{
		{
			name:      "no team_type at all",
			file:      matchFileForCompetition("T20", "", "Netherlands", "Nepal", 0),
			wantError: "missing team_type",
		},
		{
			name:      "a team_type Cricsheet does not write",
			file:      matchFileForCompetition("T20", "franchise", "Alpha", "Beta", 0),
			wantError: `unsupported team_type: "franchise"`,
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			ctx := context.Background()
			spy := arrangeMatchUpsertSpy(t)
			file := writeJSON(t, t.TempDir(), "9000031.json", tc.file)

			// Act
			err := cricsheet.ImportMatchFile(ctx, file, &cricsheet.Options{})

			// Assert
			require.ErrorContains(t, err, tc.wantError)
			assert.Empty(t, spy.matchArgs, "a refused file must not reach the match upsert")
		})
	}
}

// arrangeMatchUpsertSpy routes the import's transaction through a spy that captures the
// match upsert, with no database behind it.
func arrangeMatchUpsertSpy(t *testing.T) *matchUpsertSpyTx {
	t.Helper()
	prevPool := db.PoolAPI
	db.SetPoolAPI(nopPool{})
	t.Cleanup(func() { db.SetPoolAPI(prevPool) })
	spy := &matchUpsertSpyTx{}
	cricsheet.SetRunInTxFn(func(ctx context.Context, inner func(context.Context, db.CopyFromTx) error) error {
		return inner(ctx, spy)
	})
	t.Cleanup(func() { cricsheet.SetRunInTxFn(nil) })
	return spy
}

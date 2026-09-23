package cricsheet_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/cricsheet"
	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/db/dbtest"
	"github.com/umayangag/cric-flow/go-app/internal/formats"
)

// The acceptance test for IMPORT-09 on the real tables: a T20 between two national sides
// that no hand list named lands under T20I, and every match carries the competition level
// Cricsheet states and the ICC number where it has one, beside the format code it is
// pooled into.
//
// Gated on a scratch database like every other DB suite: `make -C go-app test-db`.

// storedCompetition is what the match row says about the competition it belonged to.
type storedCompetition struct {
	formatCode       string
	competitionLevel *string
	matchTypeNumber  *int
}

func TestImportMatchFile_Competition_PlacesTheMatchByTheLevelTheFileStates(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)

	// Arrange (shared)
	ctx := context.Background()
	_, err := db.Connect(ctx)
	require.NoError(t, err)
	t.Cleanup(db.Close)
	require.NoError(t, db.RunMigrations(ctx, filepath.Join("..", "..", "migrations")))
	require.NoError(t, db.Exec(ctx, `TRUNCATE ball_event_wicket, ball_event, fielding_event, batting_data,
		bowling_data, fielding_data, match_inning, match_player, match`))
	dir := t.TempDir()

	testCases := []struct {
		name    string
		matchID int64
		file    string
		want    storedCompetition
	}{
		{
			name:    "a T20 between two associate national sides is a T20I, by its level alone",
			matchID: 9000040,
			file:    matchFileForCompetition("T20", "international", "Netherlands", "Nepal", 2411),
			want: storedCompetition{
				formatCode:       formats.CodeT20I,
				competitionLevel: strPtr(formats.CompetitionInternational),
				matchTypeNumber:  intPtr(2411),
			},
		},
		{
			name:    "a franchise T20 is a T20, club, with no number",
			matchID: 9000041,
			file:    matchFileForCompetition("T20", "club", "Alpha", "Beta", 0),
			want: storedCompetition{
				formatCode:       formats.CodeT20,
				competitionLevel: strPtr(formats.CompetitionClub),
			},
		},
		{
			name:    "a first-class round is rated as TEST and recorded as club",
			matchID: 9000042,
			file:    matchFileForCompetition("MDM", "club", "Victoria", "Tasmania", 0),
			want: storedCompetition{
				formatCode:       formats.CodeTest,
				competitionLevel: strPtr(formats.CompetitionClub),
			},
		},
		{
			name:    "an international one-day match without ODI status is rated as ODI and recorded as international",
			matchID: 9000043,
			file:    matchFileForCompetition("ODM", "international", "Namibia", "Oman", 0),
			want: storedCompetition{
				formatCode:       formats.CodeODI,
				competitionLevel: strPtr(formats.CompetitionInternational),
			},
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			path := filepath.Join(dir, fmt.Sprintf("%d.json", tc.matchID))
			require.NoError(t, os.WriteFile(path, []byte(tc.file), 0o600))

			// Act
			require.NoError(t, cricsheet.ImportMatchFile(ctx, path, &cricsheet.Options{}))

			// Assert
			assert.Equal(t, tc.want, readStoredCompetition(ctx, t, tc.matchID))
		})
	}
}

// readStoredCompetition reads the match row's format code, competition level and number.
func readStoredCompetition(ctx context.Context, t *testing.T, matchID int64) storedCompetition {
	t.Helper()
	var got storedCompetition
	require.NoError(t, db.QueryRow(ctx, `
		SELECT mf.code, m.competition_level, m.match_type_number
		FROM match m
		JOIN match_format mf ON mf.id = m.format_id
		WHERE m.match_id = $1`, matchID).Scan(&got.formatCode, &got.competitionLevel, &got.matchTypeNumber))
	return got
}

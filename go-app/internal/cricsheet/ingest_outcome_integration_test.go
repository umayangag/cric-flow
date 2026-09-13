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
)

// The acceptance test for IMPORT-02 on the real tables: a tie decided by a super over
// lands with the eliminator as the match's winner and result 'tie' beside it, a genuine
// no-result stays with no winner and result 'no result', and neither is mistaken for the
// other. The archive's longest method must land too, since a column too narrow for it
// would refuse that file whole.
//
// Gated on a scratch database like every other DB suite: `make -C go-app test-db`.

// storedOutcome is what the match row says about how the match was decided.
type storedOutcome struct {
	winner       *string // opposition_name of the winner, nil for none
	result       *string
	resultMethod *string
}

func TestImportMatchFile_Outcome_RecordsHowTheMatchWasDecided(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)

	// Arrange (shared)
	ctx := context.Background()
	_, err := db.Connect(ctx)
	require.NoError(t, err)
	t.Cleanup(db.Close)
	require.NoError(t, db.RunMigrations(ctx, filepath.Join("..", "..", "migrations")))
	require.NoError(t, db.Exec(ctx, `TRUNCATE ball_event, fielding_event, batting_data,
		bowling_data, fielding_data, match_inning, match_player, match`))
	dir := t.TempDir()

	testCases := []struct {
		name    string
		matchID int64
		outcome string
		want    storedOutcome
	}{
		{
			name:    "a tie decided by a super over goes to the eliminator, and stays a tie",
			matchID: 9000020,
			outcome: `{"result": "tie", "eliminator": "Beta"}`,
			want:    storedOutcome{winner: strPtr("Beta"), result: strPtr("tie")},
		},
		{
			name:    "a tie decided by a bowl-out goes to the bowl_out side, and stays a tie",
			matchID: 9000021,
			outcome: `{"result": "tie", "bowl_out": "Alpha"}`,
			want:    storedOutcome{winner: strPtr("Alpha"), result: strPtr("tie")},
		},
		{
			name:    "a no-result has no winner and says so",
			matchID: 9000022,
			outcome: `{"result": "no result"}`,
			want:    storedOutcome{result: strPtr("no result")},
		},
		{
			name:    "a tie nobody broke has no winner and says so",
			matchID: 9000023,
			outcome: `{"result": "tie"}`,
			want:    storedOutcome{result: strPtr("tie")},
		},
		{
			name:    "an outright win has no result and keeps its method",
			matchID: 9000024,
			outcome: `{"winner": "Alpha", "by": {"runs": 12}, "method": "D/L"}`,
			want:    storedOutcome{winner: strPtr("Alpha"), resultMethod: strPtr("D/L")},
		},
		{
			name:    "the archive's longest method fits the column",
			matchID: 9000025,
			outcome: `{"winner": "Beta", "method": "Lost fewer wickets"}`,
			want:    storedOutcome{winner: strPtr("Beta"), resultMethod: strPtr("Lost fewer wickets")},
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			path := filepath.Join(dir, fmt.Sprintf("%d.json", tc.matchID))
			file := matchFileWithOutcomeJSON(tc.outcome,
				inningsOf("Alpha", "A1", "A2", "B1", []int{4, 1, 6}, ""),
				inningsOf("Beta", "B2", "B3", "A1", []int{4, 6, 1}, ""),
			)
			require.NoError(t, os.WriteFile(path, []byte(file), 0o600))

			// Act
			require.NoError(t, cricsheet.ImportMatchFile(ctx, path, &cricsheet.Options{}))

			// Assert
			assert.Equal(t, tc.want, readStoredOutcome(ctx, t, tc.matchID))
		})
	}
}

// readStoredOutcome reads the match row's winner (by name), result and method.
func readStoredOutcome(ctx context.Context, t *testing.T, matchID int64) storedOutcome {
	t.Helper()
	var got storedOutcome
	require.NoError(t, db.QueryRow(ctx, `
		SELECT o.opposition_name, m.result, m.result_method
		FROM match m
		LEFT JOIN opposition o ON o.id = m.outcome_winner_opposition_id
		WHERE m.match_id = $1`, matchID).Scan(&got.winner, &got.result, &got.resultMethod))
	return got
}

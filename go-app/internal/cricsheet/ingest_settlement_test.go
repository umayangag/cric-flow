package cricsheet_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/cricsheet"
	tmocks "github.com/umayangag/cric-flow/go-app/internal/cricsheet/mocks"
	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// oneMatchFile is the smallest file the importer accepts, so these cases are about what
// runs after the files rather than about what is in them.
const oneMatchFile = `{"info":{"match_type":"T20","teams":["A","B"],"dates":["2024-01-01"]},` +
	`"innings":[{"team":"A","overs":[{"over":1,"deliveries":[{"batter":"A1","bowler":"B1",` +
	`"non_striker":"A2","runs":{"batter":1,"extras":0,"total":1}}]}]}]}`

// lineageMapping is one real rename, written to a file the loader is pointed at, so the
// pass has something to apply in a unit test.
const lineageMapping = `{"version":"1","renames":[` +
	`{"from":"Delhi Daredevils","to":"Delhi Capitals","gender":"male"}]}`

// settlementHarness is the wiring these cases share: a mock database, a nop pool for the
// entity cache, and a directory holding match files.
type settlementHarness struct {
	database *tmocks.MockCricsheetDB
	dir      string
}

// newSettlementHarness points the importer at a mock database and a temporary dataset, and
// puts the lineage mapping where the loader will find it.
func newSettlementHarness(t *testing.T, files map[string]string) *settlementHarness {
	t.Helper()
	previousPool := db.PoolAPI
	db.SetPoolAPI(ingestErrorTestNopPool{})
	t.Cleanup(func() { db.SetPoolAPI(previousPool) })

	database := tmocks.NewMockCricsheetDB(t)
	previousDB := cricsheet.GetCricsheetDB()
	cricsheet.SetCricsheetDB(database)
	t.Cleanup(func() { cricsheet.SetCricsheetDB(previousDB) })

	dir := t.TempDir()
	for name, body := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600))
	}
	mappingPath := filepath.Join(t.TempDir(), "team_lineage.json")
	require.NoError(t, os.WriteFile(mappingPath, []byte(lineageMapping), 0o600))
	t.Setenv("GO_APP_TEAM_LINEAGE", mappingPath)

	return &settlementHarness{database: database, dir: dir}
}

// failEveryTransaction makes every per-file write fail, which is how an import aborts.
func failEveryTransaction(t *testing.T) {
	t.Helper()
	failing := &failingTx{err: errors.New("db error")}
	cricsheet.SetRunInTxFn(func(ctx context.Context, inner func(context.Context, db.CopyFromTx) error) error {
		return inner(ctx, failing)
	})
	t.Cleanup(func() { cricsheet.SetRunInTxFn(nil) })
}

// TestImportDir_SettlesNamesAndLineageAfterAFailedRun is IMPORT-07. A fail-fast run that
// stops on a bad file used to return before both settlement steps, and the files that had
// already committed stayed committed -- so a club that renamed kept two opposition rows,
// two Elo histories and two form series, and the next import was the only thing that could
// ever join them. The abort does not undo what landed, so settlement has to run over it.
func TestImportDir_SettlesNamesAndLineageAfterAFailedRun(t *testing.T) {
	testCases := []struct {
		name        string
		options     *cricsheet.Options
		expectError bool
	}{
		{
			name:        "fail fast aborts the run",
			options:     &cricsheet.Options{FailFast: true},
			expectError: true,
		},
		{
			name:        "without fail fast every file is skipped",
			options:     &cricsheet.Options{},
			expectError: false,
		},
	}

	for i := range testCases {
		testCase := testCases[i]
		t.Run(testCase.name, func(t *testing.T) {
			harness := newSettlementHarness(t, map[string]string{"match1.json": oneMatchFile})
			failEveryTransaction(t)
			harness.database.EXPECT().
				UpdatePlayerDisplayNames(mock.Anything, mock.Anything).
				Return(nil).
				Once()
			harness.database.EXPECT().
				ApplyTeamLineage(mock.Anything, []db.TeamRename{
					{FromName: "Delhi Daredevils", ToName: "Delhi Capitals", Gender: "male"},
				}).
				Return(db.TeamLineageReport{}, nil).
				Once()

			imported, err := cricsheet.ImportDir(context.Background(), harness.dir, testCase.options, 1)

			assert.Zero(t, imported)
			assert.Equal(t, testCase.expectError, err != nil, "the run's own verdict is unchanged")
			harness.database.AssertExpectations(t)
		})
	}
}

// TestImportDir_ReportsBothTheFailedFilesAndAFailedSettlement pins the join: an early
// return let whichever error came first hide the other, and an operator reading the log
// could not tell that a settlement step had also failed. Either step can be the one that
// fails, and neither may hide the file that stopped the run.
func TestImportDir_ReportsBothTheFailedFilesAndAFailedSettlement(t *testing.T) {
	testCases := []struct {
		name            string
		namesError      error
		lineageError    error
		expectedInError string
	}{
		{
			name:            "display-name settlement fails",
			namesError:      errors.New("name settlement is down"),
			expectedInError: "name settlement is down",
		},
		{
			name:            "the lineage pass fails",
			lineageError:    errors.New("lineage pass is down"),
			expectedInError: "lineage pass is down",
		},
	}

	for i := range testCases {
		testCase := testCases[i]
		t.Run(testCase.name, func(t *testing.T) {
			harness := newSettlementHarness(t, map[string]string{"match1.json": oneMatchFile})
			failEveryTransaction(t)
			harness.database.EXPECT().
				UpdatePlayerDisplayNames(mock.Anything, mock.Anything).
				Return(testCase.namesError).
				Once()
			harness.database.EXPECT().
				ApplyTeamLineage(mock.Anything, mock.Anything).
				Return(db.TeamLineageReport{}, testCase.lineageError).
				Once()

			_, err := cricsheet.ImportDir(
				context.Background(),
				harness.dir,
				&cricsheet.Options{FailFast: true},
				1,
			)

			require.Error(t, err)
			assert.Contains(t, err.Error(), "db error", "the file that stopped the run")
			assert.Contains(t, err.Error(), testCase.expectedInError, "and the step that failed after it")
		})
	}
}

// TestImportMatchFile_AppliesTheClubLineage covers the single-file path, which applied no
// lineage at all. One file can create the opposition row that completes a rename -- lookup
// rows are written on the pool, not inside the file's transaction -- so the link has to be
// made here too. Display names are deliberately not settled: one file has no other
// spelling to weigh its own against.
func TestImportMatchFile_AppliesTheClubLineage(t *testing.T) {
	harness := newSettlementHarness(t, map[string]string{"match1.json": oneMatchFile})
	failEveryTransaction(t)
	harness.database.EXPECT().
		ApplyTeamLineage(mock.Anything, mock.Anything).
		Return(db.TeamLineageReport{}, nil).
		Once()

	err := cricsheet.ImportMatchFile(
		context.Background(),
		filepath.Join(harness.dir, "match1.json"),
		&cricsheet.Options{},
	)

	require.Error(t, err, "the file itself still failed")
	harness.database.AssertExpectations(t)
	harness.database.AssertNotCalled(t, "UpdatePlayerDisplayNames", mock.Anything, mock.Anything)
}

// TestImportDir_ARenameLeftUnlinkedIsReportedAndDoesNotFailTheRun covers the state that
// should be unreachable after the pass whose job is to remove it: both rows of a rename in
// the archive and no link between them. It is logged rather than returned, because the
// import that just ran is not the thing that can fix it -- but silence there is what made
// IMPORT-07 invisible in the first place.
func TestImportDir_ARenameLeftUnlinkedIsReportedAndDoesNotFailTheRun(t *testing.T) {
	harness := newSettlementHarness(t, map[string]string{"match1.json": oneMatchFile})
	failEveryTransaction(t)
	harness.database.EXPECT().
		UpdatePlayerDisplayNames(mock.Anything, mock.Anything).
		Return(nil).
		Once()
	harness.database.EXPECT().
		ApplyTeamLineage(mock.Anything, mock.Anything).
		Return(db.TeamLineageReport{Renames: []db.TeamLineageRename{{
			Rename: db.TeamRename{FromName: "Delhi Daredevils", ToName: "Delhi Capitals", Gender: "male"},
			State:  db.TeamLineageUnlinked,
		}}}, nil).
		Once()

	_, err := cricsheet.ImportDir(context.Background(), harness.dir, &cricsheet.Options{}, 1)

	require.NoError(t, err, "an unlinked rename is a fact about the archive, not a failure of this run")
	harness.database.AssertExpectations(t)
}

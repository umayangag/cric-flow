package cricsheet_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-flow/go-app/internal/cricsheet"
	tmocks "github.com/umayangag/cric-flow/go-app/internal/cricsheet/mocks"
	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// ingestErrorTestNopPool implements db.PoolIface for cache lookups.
type ingestErrorTestNopPool struct{}

func (ingestErrorTestNopPool) Exec(_ context.Context, _ string, _ ...any) error { return nil }
func (ingestErrorTestNopPool) Query(_ context.Context, _ string, _ ...any) (db.Rows, error) {
	return ingestErrorTestNopRows{}, nil
}

func (ingestErrorTestNopPool) QueryRow(_ context.Context, _ string, _ ...any) db.Row {
	return ingestErrorTestNopRow{}
}

func (ingestErrorTestNopPool) Begin(_ context.Context) (db.CopyFromTx, error) {
	return ingestErrorTestNopTx{}, nil
}

type ingestErrorTestNopRows struct{}

func (ingestErrorTestNopRows) Next() bool          { return false }
func (ingestErrorTestNopRows) Scan(_ ...any) error { return nil }
func (ingestErrorTestNopRows) Close()              {}
func (ingestErrorTestNopRows) Err() error          { return nil }

type ingestErrorTestNopRow struct{}

func (ingestErrorTestNopRow) Scan(_ ...any) error { return nil }

type ingestErrorTestNopTx struct{}

func (ingestErrorTestNopTx) Exec(_ context.Context, _ string, _ ...any) error { return nil }
func (ingestErrorTestNopTx) Query(_ context.Context, _ string, _ ...any) (db.Rows, error) {
	return ingestErrorTestNopRows{}, nil
}

func (ingestErrorTestNopTx) QueryRow(_ context.Context, _ string, _ ...any) db.Row {
	return ingestErrorTestNopRow{}
}

func (ingestErrorTestNopTx) CopyFrom(
	_ context.Context,
	_ pgx.Identifier,
	_ []string,
	_ pgx.CopyFromSource,
) (int64, error) {
	return 0, nil
}
func (ingestErrorTestNopTx) Commit(_ context.Context) error   { return nil }
func (ingestErrorTestNopTx) Rollback(_ context.Context) error { return nil }

// failingTx implements db.CopyFromTx with Exec returning a fixed error.
type failingTx struct {
	err error
}

func (t *failingTx) Exec(_ context.Context, _ string, _ ...any) error { return t.err }
func (t *failingTx) Query(_ context.Context, _ string, _ ...any) (db.Rows, error) {
	return ingestErrorTestNopRows{}, nil
}

func (t *failingTx) QueryRow(_ context.Context, _ string, _ ...any) db.Row {
	return ingestErrorTestNopRow{}
}

func (t *failingTx) CopyFrom(_ context.Context, _ pgx.Identifier, _ []string, _ pgx.CopyFromSource) (int64, error) {
	return 0, t.err
}
func (t *failingTx) Commit(_ context.Context) error   { return nil }
func (t *failingTx) Rollback(_ context.Context) error { return nil }

func TestImportDir_ErrorHandling(t *testing.T) {
	ctx := context.Background()

	// Set nopPool so cache lookups (GetFormatID etc) succeed and flow reaches UpsertMatch/UpsertMatchInning
	prevPool := db.PoolAPI
	db.SetPoolAPI(ingestErrorTestNopPool{})
	t.Cleanup(func() { db.SetPoolAPI(prevPool) })

	// Setup mocks (use struct directly so we can reset expectations per subtest)
	mdb := &tmocks.MockCricsheetDB{}

	prevDB := cricsheet.GetCricsheetDB()
	cricsheet.SetCricsheetDB(mdb)
	t.Cleanup(func() {
		cricsheet.SetCricsheetDB(prevDB)
	})

	// Prepare temp dir with two files
	tmpDir, err := os.MkdirTemp("", "import-dir-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	file1 := filepath.Join(tmpDir, "match1.json")
	file2 := filepath.Join(tmpDir, "match2.json")

	require.NoError(
		t,
		os.WriteFile(
			file1,
			[]byte(
				`{"info":{"match_type":"T20","teams":["A","B"],"dates":["2024-01-01"]},"innings":[{"team":"A","overs":[{"over":1,"deliveries":[{"batter":"A1","bowler":"B1","non_striker":"A2","runs":{"batter":1,"extras":0,"total":1}}]}]}]}`,
			),
			0o600,
		),
	)
	require.NoError(
		t,
		os.WriteFile(
			file2,
			[]byte(
				`{"info":{"match_type":"T20","teams":["C","D"],"dates":["2024-01-02"]},"innings":[{"team":"C","overs":[{"over":1,"deliveries":[{"batter":"C1","bowler":"D1","non_striker":"C2","runs":{"batter":1,"extras":0,"total":1}}]}]}]}`,
			),
			0o600,
		),
	)

	t.Run("stops_at_first_DB_error", func(t *testing.T) {
		// Settlement runs after the failure now, so the run still asks for the display
		// names to be written (IMPORT-07); what it must not do is keep importing files.
		mdb.EXPECT().UpdatePlayerDisplayNames(mock.Anything, mock.Anything).Return(nil).Maybe()
		// Ingest uses db.RunInTx for writes; inject a tx that fails on first Exec (UpsertMatch).
		failingTx := &failingTx{err: fmt.Errorf("db error")}
		cricsheet.SetRunInTxFn(func(ctx context.Context, inner func(context.Context, db.CopyFromTx) error) error {
			return inner(ctx, failingTx)
		})
		t.Cleanup(func() { cricsheet.SetRunInTxFn(nil) })

		_, err := cricsheet.ImportDir(ctx, tmpDir, &cricsheet.Options{FailFast: true}, 1)

		require.Error(t, err, "ImportDir must stop and return on first DB error when FailFast is true")
	})
}

// TestImportDir_EmptyDirectoryIsAFailure guards the bug that made a mis-defaulted data
// directory invisible: the API handler read "../data" while the match files were in
// "../data/go-app/cricsheet", and since ImportDir does not recurse it found nothing —
// then reported success. Zero files is now a loud failure naming the directory.
func TestImportDir_EmptyDirectoryIsAFailure(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		prepare func(t *testing.T, dir string)
	}{
		{
			name:    "no files at all",
			prepare: func(*testing.T, string) {},
		},
		{
			name: "files, but none the importer reads",
			prepare: func(t *testing.T, dir string) {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("x"), 0o600))
			},
		},
		{
			name: "match files only in a subdirectory, which is not recursed into",
			prepare: func(t *testing.T, dir string) {
				nested := filepath.Join(dir, "go-app", "cricsheet")
				require.NoError(t, os.MkdirAll(nested, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(nested, "1.json"), []byte("{}"), 0o600))
			},
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			tc.prepare(t, dir)

			n, err := cricsheet.ImportDir(context.Background(), dir, &cricsheet.Options{}, 1)
			require.ErrorIs(t, err, cricsheet.ErrNoMatchFiles)
			require.Zero(t, n)
			require.Contains(t, err.Error(), dir, "the error must name the directory it looked in")
			require.Contains(t, err.Error(), "GO_APP_CRICSHEET_DIR", "and how to point it elsewhere")
		})
	}
}

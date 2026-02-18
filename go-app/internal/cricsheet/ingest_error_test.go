package cricsheet_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/cricsheet"
	tmocks "github.com/umayangag/cric-info-scrapers/go-app/internal/cricsheet/mocks"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
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
	mweather := &tmocks.MockWeatherClient{}

	prevDB := cricsheet.GetCricsheetDB()
	prevWeather := cricsheet.GetWeatherClient()
	cricsheet.SetCricsheetDB(mdb)
	cricsheet.SetWeatherClient(mweather)
	t.Cleanup(func() {
		cricsheet.SetCricsheetDB(prevDB)
		cricsheet.SetWeatherClient(prevWeather)
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

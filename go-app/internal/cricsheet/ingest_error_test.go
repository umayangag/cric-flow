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
	"github.com/umayangag/cric-info-scrapers/go-app/internal/cricsheet"
	tmocks "github.com/umayangag/cric-info-scrapers/go-app/internal/cricsheet/mocks"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

// matchInsertMatcher returns a mock.Matcher that accepts any *db.MatchInsert with OriginalMatchType set.
func matchInsertMatcher(matchType string) func(*db.MatchInsert) bool {
	return func(m *db.MatchInsert) bool {
		return m != nil && m.OriginalMatchType == matchType
	}
}

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

	anyCtx := mock.MatchedBy(func(c context.Context) bool { return c != nil })
	t.Run("FailFast_Enabled", func(t *testing.T) {
		// Ingest uses db cache for format/venue/season/opposition/players; only UpsertMatch (and below) hit cricDB
		mdb.On("UpsertMatch", anyCtx, mock.MatchedBy(matchInsertMatcher("T20"))).Return(fmt.Errorf("db error"))

		opts := &cricsheet.Options{FailFast: true}
		_, err := cricsheet.ImportDir(ctx, tmpDir, opts)

		require.Error(t, err, "FailFast should propagate UpsertMatch error")
		mdb.AssertExpectations(t)
	})

	t.Run("FailFast_Disabled", func(t *testing.T) {
		mdb.ExpectedCalls = nil
		mdb.On("GetMatchFormatIDByCode", anyCtx, "T20").Return(int64(1), nil).Maybe()
		mdb.On("GetOrCreateVenue", anyCtx, mock.MatchedBy(func(_ string) bool { return true })).Return(int64(1), nil).Maybe()
		mdb.On("GetOrCreateSeason", anyCtx, mock.MatchedBy(func(_ string) bool { return true })).Return(int64(1), nil).Maybe()
		mdb.On("GetOrCreateOpposition", anyCtx, mock.MatchedBy(func(_ string) bool { return true })).Return(int64(1), nil).Maybe()
		mdb.On("GetOrCreateByName", anyCtx, mock.MatchedBy(func(_ string) bool { return true })).Return(int64(1), nil).Maybe()
		mdb.On("UpsertMatch", anyCtx, mock.MatchedBy(matchInsertMatcher("T20"))).Return(nil).Twice()
		mdb.On("UpsertMatchInning", anyCtx, mock.MatchedBy(func(mi *db.MatchInningInsert) bool { return mi != nil })).Return(nil).Maybe()
		mdb.On("UpsertBattingBatch", anyCtx, mock.MatchedBy(func(_ []db.Batting) bool { return true })).Return(nil).Maybe()
		mdb.On("UpsertBowlingBatch", anyCtx, mock.MatchedBy(func(_ []db.Bowling) bool { return true })).Return(nil).Maybe()

		opts := &cricsheet.Options{FailFast: false}
		count, err := cricsheet.ImportDir(ctx, tmpDir, opts)

		require.NoError(t, err, "Should not return error when FailFast is disabled")
		require.Equal(t, 2, count, "Both files should complete successfully")
		mdb.AssertExpectations(t)
	})
}

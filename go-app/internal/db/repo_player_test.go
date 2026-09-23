package db_test

import (
	"context"
	"regexp"
	"testing"

	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// mockPoolAPI adapts pgxmock's pool to db.PoolIface, for repository functions that read
// db.PoolAPI directly rather than taking a transaction parameter.
type mockPoolAPI struct{ p pgxmock.PgxPoolIface }

func (m mockPoolAPI) Exec(ctx context.Context, sql string, args ...any) error {
	_, err := m.p.Exec(ctx, sql, args...)
	return err
}

func (m mockPoolAPI) Query(ctx context.Context, sql string, args ...any) (db.Rows, error) {
	r, err := m.p.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return rowsWrap{r}, nil
}

func (m mockPoolAPI) QueryRow(ctx context.Context, sql string, args ...any) db.Row {
	return rowWrap{m.p.QueryRow(ctx, sql, args...)}
}

func (m mockPoolAPI) Begin(_ context.Context) (db.CopyFromTx, error) {
	return mockTxAPI(m), nil
}

// TestGetOrCreatePlayer_NoExternalID_NameAlreadyKnownWithIdentity_ReusesIt pins IMPORT-15:
// a name missing from one file's registry must not mint a second person for someone whose
// identity another file already established. When the caller has no external id, the
// lookup checks for an existing row under this exact name that already carries a real
// Cricsheet identifier and reuses it -- the insert-or-get-by-name-only statement never
// runs.
func TestGetOrCreatePlayer_NoExternalID_NameAlreadyKnownWithIdentity_ReusesIt(t *testing.T) {
	// Arrange
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()
	mock.ExpectQuery(regexp.QuoteMeta(
		`SELECT id, player_name FROM player WHERE player_name = $1 AND external_id IS NOT NULL`,
	)).WithArgs("SR Taylor").
		WillReturnRows(pgxmock.NewRows([]string{"id", "player_name"}).AddRow(int64(42), "SR Taylor"))
	prev := db.PoolAPI
	db.SetPoolAPI(mockPoolAPI{mock})
	t.Cleanup(func() { db.SetPoolAPI(prev) })

	// Act
	id, storedName, err := db.GetOrCreatePlayer(context.Background(), "", "SR Taylor", "2024-01-01")

	// Assert
	require.NoError(t, err)
	require.Equal(t, int64(42), id)
	require.Equal(t, "SR Taylor", storedName)
	require.NoError(t, mock.ExpectationsWereMet(),
		"the name-only insert must never run once an identified row is found")
}

// TestGetOrCreatePlayer_NoExternalID_NameNotKnown_FallsBackToNameOnlyIdentity pins the
// other half: a genuinely new name, unknown under any external id, still takes the
// pre-identity name-keyed path.
func TestGetOrCreatePlayer_NoExternalID_NameNotKnown_FallsBackToNameOnlyIdentity(t *testing.T) {
	// Arrange
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()
	mock.ExpectQuery(regexp.QuoteMeta(
		`SELECT id, player_name FROM player WHERE player_name = $1 AND external_id IS NOT NULL`,
	)).WithArgs("New Player").
		WillReturnRows(pgxmock.NewRows([]string{"id", "player_name"})) // no matching row
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO player(player_name, name_as_of)`)).
		WithArgs("New Player", "2024-01-01").
		WillReturnRows(pgxmock.NewRows([]string{"id", "player_name"}).AddRow(int64(99), "New Player"))
	prev := db.PoolAPI
	db.SetPoolAPI(mockPoolAPI{mock})
	t.Cleanup(func() { db.SetPoolAPI(prev) })

	// Act
	id, storedName, err := db.GetOrCreatePlayer(context.Background(), "", "New Player", "2024-01-01")

	// Assert
	require.NoError(t, err)
	require.Equal(t, int64(99), id)
	require.Equal(t, "New Player", storedName)
	require.NoError(t, mock.ExpectationsWereMet())
}

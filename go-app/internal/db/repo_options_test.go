package db

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// DBMock locally defined to avoid import cycle
type DBMock struct{ mock.Mock }

func (m *DBMock) Exec(ctx context.Context, sql string, args ...any) error {
	return m.Called(ctx, sql, args).Error(0)
}

func (m *DBMock) Query(ctx context.Context, sql string, args ...any) (Rows, error) {
	argsList := make([]interface{}, 0, 2+len(args))
	argsList = append(argsList, ctx, sql)
	argsList = append(argsList, args...)
	ret := m.Called(argsList...)
	r0, _ := ret.Get(0).(Rows)
	return r0, ret.Error(1)
}

func (m *DBMock) QueryRow(ctx context.Context, sql string, args ...any) Row {
	argsList := make([]interface{}, 0, 2+len(args))
	argsList = append(argsList, ctx, sql)
	argsList = append(argsList, args...)
	ret := m.Called(argsList...)
	r0, _ := ret.Get(0).(Row)
	return r0
}

func (m *DBMock) Begin(ctx context.Context) (Tx, error) {
	ret := m.Called(ctx)
	r0, _ := ret.Get(0).(Tx)
	return r0, ret.Error(1)
}

// RowsMock locally defined
type RowsMock struct {
	mock.Mock
	vals    []string
	idx     int
	err     error
	scanErr error
}

func NewRowsMock(vals []string) *RowsMock { return &RowsMock{vals: vals} }

func (r *RowsMock) SetErr(err error) { r.err = err }

func (r *RowsMock) SetScanErr(err error) { r.scanErr = err }

func (r *RowsMock) Next() bool {
	if r.idx < len(r.vals) {
		r.idx++
		return true
	}
	return false
}

func (r *RowsMock) Scan(dest ...any) error {
	if r.scanErr != nil {
		return r.scanErr
	}
	if r.idx == 0 || r.idx > len(r.vals) {
		return errors.New("scan called out of range")
	}
	if len(dest) == 0 {
		return errors.New("no destination")
	}
	if p, ok := dest[0].(*string); ok {
		*p = r.vals[r.idx-1]
		return nil
	}
	return errors.New("invalid destination type")
}

func (r *RowsMock) Close() { r.Called() }

func (r *RowsMock) Err() error { return r.err }

func TestGetUniqueTeams(t *testing.T) {
	// Restore defaultDB after test
	originalDB := defaultDB
	defer func() { defaultDB = originalDB }()

	t.Run("success", func(t *testing.T) {
		mockDB := new(DBMock)
		SetDB(mockDB)

		expectedTeams := []string{"Australia", "India", "England"}
		rows := NewRowsMock(expectedTeams)
		// We expect Close to be called
		rows.On("Close").Return()

		mockDB.On("Query", mock.Anything, "SELECT opposition_name FROM opposition ORDER BY opposition_name").
			Return(rows, nil)

		teams, err := GetUniqueTeams(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, expectedTeams, teams)
		mockDB.AssertExpectations(t)
	})

	t.Run("query error", func(t *testing.T) {
		mockDB := new(DBMock)
		SetDB(mockDB)

		mockDB.On("Query", mock.Anything, "SELECT opposition_name FROM opposition ORDER BY opposition_name").
			Return(nil, errors.New("query failed"))

		teams, err := GetUniqueTeams(context.Background())
		assert.Error(t, err)
		assert.Nil(t, teams)
		assert.Equal(t, "query failed", err.Error())
		mockDB.AssertExpectations(t)
	})

	t.Run("rows error", func(t *testing.T) {
		mockDB := new(DBMock)
		SetDB(mockDB)

		rows := NewRowsMock(nil)
		rows.SetErr(errors.New("rows error"))
		rows.On("Close").Return()

		mockDB.On("Query", mock.Anything, "SELECT opposition_name FROM opposition ORDER BY opposition_name").
			Return(rows, nil)

		_, err := GetUniqueTeams(context.Background())
		assert.Error(t, err)
		assert.Equal(t, "rows error", err.Error())
		mockDB.AssertExpectations(t)
	})

	t.Run("scan error", func(t *testing.T) {
		mockDB := new(DBMock)
		SetDB(mockDB)

		rows := NewRowsMock([]string{"Australia"})
		rows.SetScanErr(errors.New("scan failed"))
		rows.On("Close").Return()

		mockDB.On("Query", mock.Anything, "SELECT opposition_name FROM opposition ORDER BY opposition_name").
			Return(rows, nil)

		_, err := GetUniqueTeams(context.Background())
		assert.Error(t, err)
		assert.Equal(t, "scan failed", err.Error())
		mockDB.AssertExpectations(t)
	})
}

func TestGetUniqueFormats(t *testing.T) {
	// Restore defaultDB after test
	originalDB := defaultDB
	defer func() { defaultDB = originalDB }()

	t.Run("success", func(t *testing.T) {
		mockDB := new(DBMock)
		SetDB(mockDB)

		expectedFormats := []string{"ODI", "T20I", "Test"}
		rows := NewRowsMock(expectedFormats)
		rows.On("Close").Return()

		mockDB.On("Query", mock.Anything, "SELECT code FROM match_format ORDER BY code").
			Return(rows, nil)

		formats, err := GetUniqueFormats(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, expectedFormats, formats)
		mockDB.AssertExpectations(t)
	})

	t.Run("query error", func(t *testing.T) {
		mockDB := new(DBMock)
		SetDB(mockDB)

		mockDB.On("Query", mock.Anything, "SELECT code FROM match_format ORDER BY code").
			Return(nil, errors.New("query failed"))

		formats, err := GetUniqueFormats(context.Background())
		assert.Error(t, err)
		assert.Nil(t, formats)
		assert.Equal(t, "query failed", err.Error())
		mockDB.AssertExpectations(t)
	})

	t.Run("rows error", func(t *testing.T) {
		mockDB := new(DBMock)
		SetDB(mockDB)

		rows := NewRowsMock(nil)
		rows.SetErr(errors.New("rows error"))
		rows.On("Close").Return()

		mockDB.On("Query", mock.Anything, "SELECT code FROM match_format ORDER BY code").
			Return(rows, nil)

		_, err := GetUniqueFormats(context.Background())
		assert.Error(t, err)
		assert.Equal(t, "rows error", err.Error())
		mockDB.AssertExpectations(t)
	})

	t.Run("scan error", func(t *testing.T) {
		mockDB := new(DBMock)
		SetDB(mockDB)

		rows := NewRowsMock([]string{"ODI"})
		rows.SetScanErr(errors.New("scan failed"))
		rows.On("Close").Return()

		mockDB.On("Query", mock.Anything, "SELECT code FROM match_format ORDER BY code").
			Return(rows, nil)

		_, err := GetUniqueFormats(context.Background())
		assert.Error(t, err)
		assert.Equal(t, "scan failed", err.Error())
		mockDB.AssertExpectations(t)
	})
}

func TestGetTeamsByFormat(t *testing.T) {
	originalDB := defaultDB
	defer func() { defaultDB = originalDB }()

	t.Run("success", func(t *testing.T) {
		mockDB := new(DBMock)
		SetDB(mockDB)

		expectedTeams := []string{"Australia", "India"}
		rows := NewRowsMock(expectedTeams)
		rows.On("Close").Return()

		mockDB.On("Query", mock.Anything, mock.Anything, "T20").
			Return(rows, nil)

		teams, err := GetTeamsByFormat(context.Background(), "T20")
		assert.NoError(t, err)
		assert.Equal(t, expectedTeams, teams)
		mockDB.AssertExpectations(t)
	})

	t.Run("db pool not initialized", func(t *testing.T) {
		SetDB(nil)
		_, err := GetTeamsByFormat(context.Background(), "T20")
		assert.Error(t, err)
		assert.Equal(t, "db pool not initialized", err.Error())
	})
}

func TestGetOpponentsByFormatAndTeam(t *testing.T) {
	originalDB := defaultDB
	defer func() { defaultDB = originalDB }()

	t.Run("success", func(t *testing.T) {
		mockDB := new(DBMock)
		SetDB(mockDB)

		expectedOpponents := []string{"England", "New Zealand"}
		rows := NewRowsMock(expectedOpponents)
		rows.On("Close").Return()

		mockDB.On("Query", mock.Anything, mock.Anything, "ODI", "India").
			Return(rows, nil)

		opponents, err := GetOpponentsByFormatAndTeam(context.Background(), "ODI", "India")
		assert.NoError(t, err)
		assert.Equal(t, expectedOpponents, opponents)
		mockDB.AssertExpectations(t)
	})

	t.Run("db pool not initialized", func(t *testing.T) {
		SetDB(nil)
		_, err := GetOpponentsByFormatAndTeam(context.Background(), "ODI", "India")
		assert.Error(t, err)
		assert.Equal(t, "db pool not initialized", err.Error())
	})
}

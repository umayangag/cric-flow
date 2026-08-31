package db_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/db/mocks"
)

func TestGetUniqueTeams(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mockDB := &mocks.MockDB{}
		db.SetDB(mockDB)
		t.Cleanup(func() { db.SetDB(nil) })

		expectedTeams := []string{"Australia", "India", "England"}
		rows := stringRowsForOptions(expectedTeams)

		mockDB.On("Query", mock.Anything, "SELECT DISTINCT opposition_name FROM opposition ORDER BY opposition_name").
			Return(rows, nil)

		teams, err := db.GetUniqueTeams(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, expectedTeams, teams)
		mockDB.AssertExpectations(t)
	})

	t.Run("query error", func(t *testing.T) {
		mockDB := &mocks.MockDB{}
		db.SetDB(mockDB)
		t.Cleanup(func() { db.SetDB(nil) })

		mockDB.On("Query", mock.Anything, "SELECT DISTINCT opposition_name FROM opposition ORDER BY opposition_name").
			Return(nil, errors.New("query failed"))

		teams, err := db.GetUniqueTeams(context.Background())
		assert.Error(t, err)
		assert.Nil(t, teams)
		assert.Equal(t, "query failed", err.Error())
		mockDB.AssertExpectations(t)
	})

	t.Run("rows error", func(t *testing.T) {
		mockDB := &mocks.MockDB{}
		db.SetDB(mockDB)
		t.Cleanup(func() { db.SetDB(nil) })

		rows := stringRowsForOptions(nil).(*optionsStringRows)
		rows.err = errors.New("rows error")

		mockDB.On("Query", mock.Anything, "SELECT DISTINCT opposition_name FROM opposition ORDER BY opposition_name").
			Return(rows, nil)

		_, err := db.GetUniqueTeams(context.Background())
		assert.Error(t, err)
		assert.Equal(t, "rows error", err.Error())
		mockDB.AssertExpectations(t)
	})

	t.Run("scan error", func(t *testing.T) {
		mockDB := &mocks.MockDB{}
		db.SetDB(mockDB)
		t.Cleanup(func() { db.SetDB(nil) })

		rows := stringRowsForOptions([]string{"Australia"}).(*optionsStringRows)
		rows.scanErr = errors.New("scan failed")

		mockDB.On("Query", mock.Anything, "SELECT DISTINCT opposition_name FROM opposition ORDER BY opposition_name").
			Return(rows, nil)

		_, err := db.GetUniqueTeams(context.Background())
		assert.Error(t, err)
		assert.Equal(t, "scan failed", err.Error())
		mockDB.AssertExpectations(t)
	})
}

func TestGetUniqueFormats(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mockDB := &mocks.MockDB{}
		db.SetDB(mockDB)
		t.Cleanup(func() { db.SetDB(nil) })

		expectedFormats := []string{"ODI", "T20I", "Test"}
		rows := stringRowsForOptions(expectedFormats)

		mockDB.On("Query", mock.Anything, "SELECT code FROM match_format ORDER BY code").
			Return(rows, nil)

		formats, err := db.GetUniqueFormats(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, expectedFormats, formats)
		mockDB.AssertExpectations(t)
	})

	t.Run("query error", func(t *testing.T) {
		mockDB := &mocks.MockDB{}
		db.SetDB(mockDB)
		t.Cleanup(func() { db.SetDB(nil) })

		mockDB.On("Query", mock.Anything, "SELECT code FROM match_format ORDER BY code").
			Return(nil, errors.New("query failed"))

		formats, err := db.GetUniqueFormats(context.Background())
		assert.Error(t, err)
		assert.Nil(t, formats)
		assert.Equal(t, "query failed", err.Error())
		mockDB.AssertExpectations(t)
	})

	t.Run("rows error", func(t *testing.T) {
		mockDB := &mocks.MockDB{}
		db.SetDB(mockDB)
		t.Cleanup(func() { db.SetDB(nil) })

		rows := stringRowsForOptions(nil).(*optionsStringRows)
		rows.err = errors.New("rows error")

		mockDB.On("Query", mock.Anything, "SELECT code FROM match_format ORDER BY code").
			Return(rows, nil)

		_, err := db.GetUniqueFormats(context.Background())
		assert.Error(t, err)
		assert.Equal(t, "rows error", err.Error())
		mockDB.AssertExpectations(t)
	})

	t.Run("scan error", func(t *testing.T) {
		mockDB := &mocks.MockDB{}
		db.SetDB(mockDB)
		t.Cleanup(func() { db.SetDB(nil) })

		rows := stringRowsForOptions([]string{"ODI"}).(*optionsStringRows)
		rows.scanErr = errors.New("scan failed")

		mockDB.On("Query", mock.Anything, "SELECT code FROM match_format ORDER BY code").
			Return(rows, nil)

		_, err := db.GetUniqueFormats(context.Background())
		assert.Error(t, err)
		assert.Equal(t, "scan failed", err.Error())
		mockDB.AssertExpectations(t)
	})
}

func TestGetTeamsByFormat(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mockDB := &mocks.MockDB{}
		db.SetDB(mockDB)
		t.Cleanup(func() { db.SetDB(nil) })

		expectedTeams := []string{"Australia", "India"}
		rows := stringRowsForOptions(expectedTeams)

		mockDB.On("Query", mock.Anything, mock.Anything, mock.Anything).
			Return(rows, nil)

		teams, err := db.GetTeamsByFormat(context.Background(), "T20")
		assert.NoError(t, err)
		assert.Equal(t, expectedTeams, teams)
		mockDB.AssertExpectations(t)
	})

	t.Run("db pool not initialized", func(t *testing.T) {
		db.SetDB(nil)
		_, err := db.GetTeamsByFormat(context.Background(), "T20")
		assert.Error(t, err)
		assert.Equal(t, "db pool not initialized", err.Error())
	})
}

func TestGetOpponentsByFormatAndTeam(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mockDB := &mocks.MockDB{}
		db.SetDB(mockDB)
		t.Cleanup(func() { db.SetDB(nil) })

		expectedOpponents := []string{"England", "New Zealand"}
		rows := stringRowsForOptions(expectedOpponents)

		mockDB.On("Query", mock.Anything, mock.Anything, mock.Anything).
			Return(rows, nil)

		opponents, err := db.GetOpponentsByFormatAndTeam(context.Background(), "ODI", "India")
		assert.NoError(t, err)
		assert.Equal(t, expectedOpponents, opponents)
		mockDB.AssertExpectations(t)
	})

	t.Run("db pool not initialized", func(t *testing.T) {
		db.SetDB(nil)
		_, err := db.GetOpponentsByFormatAndTeam(context.Background(), "ODI", "India")
		assert.Error(t, err)
		assert.Equal(t, "db pool not initialized", err.Error())
	})
}

func TestGetVenuesByQuery(t *testing.T) {
	t.Run("short_query_returns_nil_nil", func(t *testing.T) {
		got, err := db.GetVenuesByQuery(context.Background(), "ab")
		assert.NoError(t, err)
		assert.Nil(t, got)
	})
	t.Run("short_query_after_trim_returns_nil_nil", func(t *testing.T) {
		got, err := db.GetVenuesByQuery(context.Background(), "  x  ")
		assert.NoError(t, err)
		assert.Nil(t, got)
	})
	t.Run("success", func(t *testing.T) {
		mockDB := &mocks.MockDB{}
		db.SetDB(mockDB)
		t.Cleanup(func() { db.SetDB(nil) })

		rows := stringRowsForOptions([]string{"Lords", "MCG"})
		mockDB.On("Query", mock.Anything, mock.Anything, mock.Anything).
			Return(rows, nil)

		got, err := db.GetVenuesByQuery(context.Background(), "lords")
		assert.NoError(t, err)
		assert.Equal(t, []string{"Lords", "MCG"}, got)
		mockDB.AssertExpectations(t)
	})
}

func TestBuildDSN(t *testing.T) {
	got := db.BuildDSN("u", "p", "h", "5432", "db", "disable")
	assert.Equal(t, "postgres://u:p@h:5432/db?sslmode=disable", got)
}

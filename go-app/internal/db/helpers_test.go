package db_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/umayangag/cric-flow/go-app/internal/db"
)

func TestAvailable(t *testing.T) {
	db.SetDB(nil)
	assert.False(t, db.Available())
}

func TestExec_WhenDBNil_ReturnsErrDBNotSet(t *testing.T) {
	db.SetDB(nil)
	err := db.Exec(context.Background(), "SELECT 1")
	assert.True(t, errors.Is(err, db.ErrDBNotSet))
}

func TestQuery_WhenDBNil_ReturnsErrDBNotSet(t *testing.T) {
	db.SetDB(nil)
	rows, err := db.Query(context.Background(), "SELECT 1")
	assert.True(t, errors.Is(err, db.ErrDBNotSet))
	assert.Nil(t, rows)
}

func TestQueryRow_WhenDBNil_ReturnsErrRow(t *testing.T) {
	db.SetDB(nil)
	row := db.QueryRow(context.Background(), "SELECT 1")
	var x int
	err := row.Scan(&x)
	assert.True(t, errors.Is(err, db.ErrDBNotSet))
}

func TestBegin_WhenDBNil_ReturnsErrDBNotSet(t *testing.T) {
	db.SetDB(nil)
	tx, err := db.Begin(context.Background())
	assert.True(t, errors.Is(err, db.ErrDBNotSet))
	assert.Nil(t, tx)
}

func TestRunInTx_WhenPoolAPINil_ReturnsError(t *testing.T) {
	orig := db.PoolAPI
	db.PoolAPI = nil
	t.Cleanup(func() { db.PoolAPI = orig })

	err := db.RunInTx(context.Background(), func(context.Context, db.CopyFromTx) error { return nil })
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "pool not initialized")
}

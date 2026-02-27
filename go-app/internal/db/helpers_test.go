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

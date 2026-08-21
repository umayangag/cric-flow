package db_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-flow/go-app/internal/db"
)

func TestComputePendingMigrations_SortsAndSkipsApplied(t *testing.T) {
	// Arrange
	files := []string{"002_b.sql", "001_a.sql", "readme.md", "003_c.SQL"}
	applied := []string{"001_a.sql"}

	// Act
	got := db.ComputePendingMigrations(files, applied)

	// Assert
	require.Equal(t, []string{"002_b.sql", "003_c.SQL"}, got)
}

func TestComputePendingMigrations_EmptyOrNoSQL(t *testing.T) {
	// Arrange & Act & Assert
	require.Empty(t, db.ComputePendingMigrations(nil, nil))

	files := []string{"notes.txt", "migrate.sh"}
	require.Empty(t, db.ComputePendingMigrations(files, nil))
}

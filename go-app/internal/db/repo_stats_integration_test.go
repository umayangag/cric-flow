package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/db/dbtest"
)

func TestGetTableStats_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)

	ctx := context.Background()
	pool, err := Connect(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })

	// Run migrations to ensure we have tables
	require.NoError(t, RunMigrations(ctx, migrationsDir()))

	stats, err := GetTableStats(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, stats, "expected some tables")

	foundMigrations := false
	for _, s := range stats {
		t.Logf("Table: %s, Count: %d, Last: %v", s.TableName, s.RowCount, s.LastRecord)
		if s.TableName == "schema_migrations" {
			foundMigrations = true
		}
	}

	require.True(t, foundMigrations, "schema_migrations table not found in stats")
}

package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetTableStats_Integration(t *testing.T) {
	if !guardIntegration(t) {
		t.Skip("integration test skipped; set RUN_DB_TESTS=1 to run")
	}

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

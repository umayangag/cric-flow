package db

import (
	"context"
	"testing"
)

func TestGetTableStats_Integration(t *testing.T) {
	if !guardIntegration(t) {
		t.Skip("integration test skipped; set RUN_DB_TESTS=1 to run")
	}

	ctx := context.Background()
	pool, err := Connect(ctx)
	if err != nil {
		t.Fatalf("Connect error: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	// Run migrations to ensure we have tables
	if err := RunMigrations(ctx, migrationsDir()); err != nil {
		t.Fatalf("migrations failed: %v", err)
	}

	stats, err := GetTableStats(ctx)
	if err != nil {
		t.Fatalf("GetTableStats failed: %v", err)
	}

	if len(stats) == 0 {
		t.Fatalf("Expected some tables, got 0")
	}

	foundMigrations := false
	for _, s := range stats {
		t.Logf("Table: %s, Count: %d, Last: %v", s.TableName, s.RowCount, s.LastRecord)
		if s.TableName == "schema_migrations" {
			foundMigrations = true
		}
	}

	if !foundMigrations {
		t.Errorf("schema_migrations table not found in stats")
	}
}

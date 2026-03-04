package precomputefeatures_test

import (
	"flag"
	"testing"

	svc "github.com/umayangag/cric-flow/go-app/internal/services/precomputefeatures"
)

func TestMigrationsDefaultFromEnv(t *testing.T) {
	t.Setenv("MIGRATIONS_DIR", "/tmp/env-migs")
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	opts, err := svc.ParseArgs(fs, []string{"-format", "T20"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.MigrationsDir != "/tmp/env-migs" {
		t.Fatalf("migrations dir default from env not applied: got=%q want=/tmp/env-migs", opts.MigrationsDir)
	}
}

func TestMigrationsFlagOverridesEnv(t *testing.T) {
	t.Setenv("MIGRATIONS_DIR", "/tmp/env-migs")
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	opts, err := svc.ParseArgs(fs, []string{"-format", "ODI", "-migrations", "/opt/flag-migs"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.MigrationsDir != "/opt/flag-migs" {
		t.Fatalf("flag should override env: got=%q want=/opt/flag-migs", opts.MigrationsDir)
	}
}

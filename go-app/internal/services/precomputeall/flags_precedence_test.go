package precomputeall_test

import (
	"flag"
	"testing"

	svc "github.com/umayangag/cric-flow/go-app/internal/services/precomputeall"

	"github.com/stretchr/testify/require"
)

func TestMigrationsDefaultFromEnv(t *testing.T) {
	t.Setenv("MIGRATIONS_DIR", "/tmp/env-migs")
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	opts, err := svc.ParseArgs(fs, []string{"-format", "T20"})
	require.NoError(t, err)
	if opts.MigrationsDir != "/tmp/env-migs" {
		t.Fatalf("migrations dir default from env not applied: got=%q want=/tmp/env-migs", opts.MigrationsDir)
	}
}

func TestMigrationsFlagOverridesEnv(t *testing.T) {
	t.Setenv("MIGRATIONS_DIR", "/tmp/env-migs")
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	opts, err := svc.ParseArgs(fs, []string{"-format", "ODI", "-migrations", "/opt/flag-migs"})
	require.NoError(t, err)
	if opts.MigrationsDir != "/opt/flag-migs" {
		t.Fatalf("flag should override env: got=%q want=/opt/flag-migs", opts.MigrationsDir)
	}
}

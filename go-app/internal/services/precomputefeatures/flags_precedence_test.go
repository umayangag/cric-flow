package precomputefeatures_test

import (
	"flag"
	"testing"

	svc "github.com/umayangag/cric-flow/go-app/internal/services/precomputefeatures"

	"github.com/stretchr/testify/require"
)

func TestMigrationsDefaultFromEnv(t *testing.T) {
	t.Setenv("MIGRATIONS_DIR", "/tmp/env-migs")
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	opts, err := svc.ParseArgs(fs, []string{"-format", "T20"})
	require.NoError(t, err)
	require.Equal(t, "/tmp/env-migs", opts.MigrationsDir)
}

func TestMigrationsFlagOverridesEnv(t *testing.T) {
	t.Setenv("MIGRATIONS_DIR", "/tmp/env-migs")
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	opts, err := svc.ParseArgs(fs, []string{"-format", "ODI", "-migrations", "/opt/flag-migs"})
	require.NoError(t, err)
	require.Equal(t, "/opt/flag-migs", opts.MigrationsDir)
}

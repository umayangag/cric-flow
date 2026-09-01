package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunMigrationsAtStartup_DefaultTrue(t *testing.T) {
	os.Unsetenv("RUN_MIGRATIONS_AT_STARTUP")
	defer os.Unsetenv("RUN_MIGRATIONS_AT_STARTUP")
	require.True(t, runMigrationsAtStartup())
}

func TestRunMigrationsAtStartup_DisabledByEnv(t *testing.T) {
	for _, v := range []string{"0", "false", "no"} {
		t.Run(v, func(t *testing.T) {
			os.Setenv("RUN_MIGRATIONS_AT_STARTUP", v)
			defer os.Unsetenv("RUN_MIGRATIONS_AT_STARTUP")
			require.False(t, runMigrationsAtStartup())
		})
	}
}

func TestRunTrackingReconciliationAtStartup_DefaultTrue(t *testing.T) {
	os.Unsetenv("RUN_TRACKING_CANCEL_STALE_AT_STARTUP")
	defer os.Unsetenv("RUN_TRACKING_CANCEL_STALE_AT_STARTUP")
	require.True(t, runTrackingCancelStaleAtStartup())
}

func TestRunTrackingReconciliationAtStartup_DisabledByEnv(t *testing.T) {
	for _, v := range []string{"0", "false", "no"} {
		t.Run(v, func(t *testing.T) {
			os.Setenv("RUN_TRACKING_CANCEL_STALE_AT_STARTUP", v)
			defer os.Unsetenv("RUN_TRACKING_CANCEL_STALE_AT_STARTUP")
			require.False(t, runTrackingCancelStaleAtStartup())
		})
	}
}

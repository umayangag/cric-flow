package resources

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-flow/go-app/internal/config"
)

func resetObservationsState(t *testing.T) {
	t.Helper()

	observationsMu.Lock()
	observations = make(map[Kind]int)
	observationsPath = ""
	observationsLoaded = sync.Once{}
	observationsMu.Unlock()

	require.NoError(t, os.Unsetenv("USE_RESOURCE_OBSERVATIONS"))
	require.NoError(t, os.Unsetenv("RESOURCE_OBSERVATIONS_PATH"))
}

func TestUseObservationsEnvToggle(t *testing.T) {
	resetObservationsState(t)

	// Default: enabled
	require.True(t, UseObservations())

	require.NoError(t, os.Setenv("USE_RESOURCE_OBSERVATIONS", "false"))
	require.False(t, UseObservations())

	require.NoError(t, os.Setenv("USE_RESOURCE_OBSERVATIONS", "0"))
	require.False(t, UseObservations())

	require.NoError(t, os.Setenv("USE_RESOURCE_OBSERVATIONS", "true"))
	require.True(t, UseObservations())
}

func TestObservationsFilePathEnvSanitized(t *testing.T) {
	resetObservationsState(t)

	defDir := config.DefaultExportDir()

	require.NoError(t, os.Setenv("RESOURCE_OBSERVATIONS_PATH", "/tmp/../custom/observations.json"))
	// Reset any cached path
	observationsPath = ""

	path := observationsFilePath()
	require.Equal(t, filepath.Join(defDir, "observations.json"), path)
}

func TestObservedMBPerWorkerLoadsFromFile(t *testing.T) {
	resetObservationsState(t)

	dir := t.TempDir()
	path := filepath.Join(dir, "obs.json")

	data := `{"precompute": 42, "ignore_zero": 0}`
	require.NoError(t, os.WriteFile(path, []byte(data), 0o600))

	SetObservationsPathForTest(path)
	require.NoError(t, os.Setenv("USE_RESOURCE_OBSERVATIONS", "true"))

	// Triggers loadObservations via ObservedMBPerWorker.
	got := ObservedMBPerWorker(KindPrecompute)
	require.Equal(t, 42, got)

	// Unknown kind returns zero.
	require.Equal(t, 0, ObservedMBPerWorker(KindImport))
}

func TestRecordWorkerMemorySamplePersistsAndIsReadable(t *testing.T) {
	resetObservationsState(t)

	dir := t.TempDir()
	path := filepath.Join(dir, "obs.json")
	SetObservationsPathForTest(path)
	require.NoError(t, os.Setenv("USE_RESOURCE_OBSERVATIONS", "true"))

	// concurrency < 1 is ignored
	RecordWorkerMemorySample(KindPrecompute, 0)
	require.Equal(t, 0, ObservedMBPerWorker(KindPrecompute))

	// Valid concurrency records an observation and saves it.
	RecordWorkerMemorySample(KindPrecompute, 4)

	v := ObservedMBPerWorker(KindPrecompute)
	require.GreaterOrEqual(t, v, 1)

	// File should exist and be non-empty.
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.True(t, info.Size() > 0)
}

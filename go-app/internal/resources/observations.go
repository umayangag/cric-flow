// Package resources: dynamic per-worker memory observations from actual runs.
// When enabled, MB-per-worker estimates are learned from process heap usage
// (heap at end of run / concurrency) and used for memory-based concurrency
// instead of config/constants. Set USE_RESOURCE_OBSERVATIONS=false to disable.
package resources

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/umayangag/cric-flow/go-app/internal/config"
)

const observationsFilename = "resource_observations.json"

var (
	observationsMu    sync.RWMutex
	observations      = make(map[Kind]int) // MB per worker, last observed
	observationsPath  string               // set once from env or config
	observationsLoaded sync.Once
)

func observationsFilePath() string {
	if observationsPath != "" {
		return observationsPath
	}
	if p := strings.TrimSpace(os.Getenv("RESOURCE_OBSERVATIONS_PATH")); p != "" {
		observationsPath = p
		return p
	}
	observationsPath = filepath.Join(config.DefaultExportDir(), observationsFilename)
	return observationsPath
}

// UseObservations returns whether to use observed MB-per-worker when available.
// Set env USE_RESOURCE_OBSERVATIONS=false to disable (use config/constants only).
func UseObservations() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("USE_RESOURCE_OBSERVATIONS")))
	if v == "0" || v == "false" || v == "no" {
		return false
	}
	return true
}

// ObservedMBPerWorker returns the last observed MB per worker for the kind (0 if none).
func ObservedMBPerWorker(kind Kind) int {
	if !UseObservations() {
		return 0
	}
	observationsLoaded.Do(loadObservations)
	observationsMu.RLock()
	defer observationsMu.RUnlock()
	return observations[kind]
}

// loadObservations reads the observations file into memory. Safe to call multiple times.
func loadObservations() {
	path := observationsFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Debug("resources: could not load observations", slog.String("path", path), slog.Any("err", err))
		}
		return
	}
	var m map[string]int
	if err := json.Unmarshal(data, &m); err != nil {
		slog.Debug("resources: invalid observations file", slog.String("path", path), slog.Any("err", err))
		return
	}
	observationsMu.Lock()
	defer observationsMu.Unlock()
	for k, v := range m {
		if v > 0 {
			observations[Kind(k)] = v
		}
	}
}

func saveObservations() {
	path := observationsFilePath()
	observationsMu.RLock()
	m := make(map[string]int, len(observations))
	for k, v := range observations {
		if v > 0 {
			m[string(k)] = v
		}
	}
	observationsMu.RUnlock()
	if len(m) == 0 {
		return
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		slog.Debug("resources: marshal observations failed", slog.Any("err", err))
		return
	}
	dir := filepath.Dir(path)
	_ = os.MkdirAll(dir, 0755)
	if err := os.WriteFile(path, data, 0644); err != nil {
		slog.Warn("resources: could not save observations", slog.String("path", path), slog.Any("err", err))
		return
	}
	slog.Debug("resources: saved observations", slog.String("path", path), slog.Any("mb_per_worker", m))
}

// RecordWorkerMemorySample records process heap usage at the end of a pipeline run
// to estimate MB per worker: heapAllocMB / concurrency. Call once per run when
// the pipeline has finished its main work (before GC may shrink heap). Concurrency
// must be the actual number of workers used this run.
// If concurrency < 1, no observation is recorded.
func RecordWorkerMemorySample(kind Kind, concurrency int) {
	if concurrency < 1 {
		return
	}
	observationsLoaded.Do(loadObservations)
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	heapMB := int(ms.HeapAlloc / (1024 * 1024))
	perWorker := heapMB / concurrency
	if perWorker < 1 {
		perWorker = 1
	}
	observationsMu.Lock()
	observations[kind] = perWorker
	observationsMu.Unlock()
	slog.Info("resources: observed MB per worker",
		slog.String("kind", string(kind)),
		slog.Int("concurrency", concurrency),
		slog.Int("heap_mb", heapMB),
		slog.Int("mb_per_worker", perWorker),
	)
	saveObservations()
}


// SetObservationsPathForTest sets the observations file path (for tests). Call with "" to reset.
func SetObservationsPathForTest(path string) {
	observationsPath = path
}

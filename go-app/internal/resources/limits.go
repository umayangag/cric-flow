// Package resources provides resource-aware concurrency limits for pipelines
// (precompute, import, export, seqcalc) to avoid OOM while using available CPU/memory.
// Use more workers when resources are available; use fewer when memory or CPU is limited.
package resources

import (
	"log/slog"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/umayangag/cric-flow/go-app/internal/config"
)

// Kind identifies the pipeline or operation for env/config overrides.
type Kind string

const (
	KindPrecompute Kind = "precompute"
	KindImport     Kind = "import"
	KindExport     Kind = "export"
	KindSeqCalc    Kind = "seqcalc"
	KindFielding   Kind = "fielding"
)

// Default estimated memory per concurrent worker (MB) for memory-heavy tasks.
// Precompute: each worker holds batting/bowling history + opposition/venue variants + form/consistency for one player.
// Use 450 MB per worker (was 250); observed OOM when 250 was too low (multiple history slices per player).
// Import: each worker holds one parsed match JSON + DB buffers.
const (
	DefaultPrecomputeMBPerWorker = 450
	DefaultImportMBPerWorker     = 150
	DefaultExportMBPerWorker     = 100
	DefaultSeqCalcMBPerWorker    = 200
	DefaultFieldingMBPerWorker   = 50
)

// defaultPrecomputeConcurrencyWhenNoLimit is used when no memory limit is detected (no GOMEMLIMIT/cgroup)
// to avoid spawning too many workers and causing OOM (e.g. on bare metal or older k8s).
const defaultPrecomputeConcurrencyWhenNoLimit = 2

// ConcurrencyLimit returns a safe concurrency limit for the given pipeline kind.
// Order of precedence: env override (e.g. PRECOMPUTE_CONCURRENCY) > configLimit > config callback >
// memory-based limit (from GOMEMLIMIT or cgroup) > CPU-based default.
// Floor 1, ceiling is kind-specific (e.g. NumCPU*2 for import).
func ConcurrencyLimit(kind Kind, configLimit int, getConfigLimit func() int) int {
	envKey := envKeyForKind(kind)
	if v := os.Getenv(envKey); v != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n >= 1 {
			return clampToCeiling(n, ceiling(kind))
		}
	}
	if configLimit > 0 {
		return clampToCeiling(configLimit, ceiling(kind))
	}
	if getConfigLimit != nil {
		if n := getConfigLimit(); n > 0 {
			return clampToCeiling(n, ceiling(kind))
		}
	}
	n := memoryBasedLimit(kind)
	if n <= 0 {
		// No memory limit detected (no GOMEMLIMIT/cgroup): use conservative default for memory-heavy kinds to avoid OOM.
		if kind == KindPrecompute {
			n = defaultPrecomputeConcurrencyWhenNoLimit
		} else {
			n = runtime.NumCPU()
		}
		if n < 1 {
			n = 1
		}
	}
	return clampToCeiling(n, ceiling(kind))
}

// GetLimit returns the resource-aware concurrency limit for the given kind,
// using config.Pipeline when set and ConcurrencyLimit for env/memory/CPU fallback.
func GetLimit(kind Kind) int {
	return ConcurrencyLimit(kind, 0, func() int {
		cfg := config.Load()
		if cfg == nil {
			return 0
		}
		switch kind {
		case KindPrecompute:
			return cfg.Pipeline.PrecomputeConcurrency
		case KindImport:
			return cfg.Pipeline.ImportConcurrency
		case KindSeqCalc:
			return cfg.Pipeline.SeqCalcConcurrency
		case KindExport:
			return cfg.Pipeline.ExportConcurrency
		case KindFielding:
			return cfg.Pipeline.FieldingConcurrency
		default:
			return 0
		}
	})
}

func envKeyForKind(kind Kind) string {
	switch kind {
	case KindPrecompute:
		return "PRECOMPUTE_CONCURRENCY"
	case KindImport:
		return "IMPORT_CONCURRENCY"
	case KindExport:
		return "EXPORT_CONCURRENCY"
	case KindSeqCalc:
		return "SEQCALC_CONCURRENCY"
	case KindFielding:
		return "FIELDING_CONCURRENCY"
	default:
		return "PIPELINE_CONCURRENCY"
	}
}

func ceiling(kind Kind) int {
	cpu := runtime.NumCPU()
	if cpu < 1 {
		cpu = 1
	}
	switch kind {
	case KindImport:
		return cpu * 4
	case KindPrecompute, KindExport, KindSeqCalc:
		return cpu * 2
	default:
		return cpu
	}
}

func memoryBasedLimit(kind Kind) int {
	limitBytes := detectMemoryLimitBytes()
	if limitBytes <= 0 {
		return 0
	}
	perWorkerMB := defaultMBPerWorker(kind)
	perWorkerBytes := int64(perWorkerMB) * 1024 * 1024
	if perWorkerBytes <= 0 {
		return 0
	}
	// Use at most 70% of limit for workers to leave room for runtime, DB, etc.
	usable := (limitBytes * 70) / 100
	n := int(usable / perWorkerBytes)
	if n < 1 {
		n = 1
	}
	slog.Debug("resources: memory-based limit",
		slog.String("kind", string(kind)),
		slog.Int64("limit_mb", limitBytes/(1024*1024)),
		slog.Int("mb_per_worker", perWorkerMB),
		slog.Int("workers", n))
	return n
}

func defaultMBPerWorker(kind Kind) int {
	switch kind {
	case KindPrecompute:
		return DefaultPrecomputeMBPerWorker
	case KindImport:
		return DefaultImportMBPerWorker
	case KindExport:
		return DefaultExportMBPerWorker
	case KindSeqCalc:
		return DefaultSeqCalcMBPerWorker
	case KindFielding:
		return DefaultFieldingMBPerWorker
	default:
		return DefaultImportMBPerWorker
	}
}

// clampToCeiling clamps n to [1, hi].
func clampToCeiling(n, hi int) int {
	if n < 1 {
		return 1
	}
	if n > hi {
		return hi
	}
	return n
}

// detectMemoryLimitBytes returns process memory limit in bytes (GOMEMLIMIT or cgroup v2).
// Returns 0 if unknown so callers fall back to CPU-based concurrency.
// Reassignable in tests for table-driven memoryBasedLimit tests.
var detectMemoryLimitBytes = func() int64 {
	if b := parseGOMEMLIMIT(os.Getenv("GOMEMLIMIT")); b > 0 {
		return b
	}
	if b := readCgroupMemoryMax(); b > 0 {
		return b
	}
	return 0
}

// gomemlimitUnits is the list of size suffixes for parseGOMEMLIMIT (longest first for correct parsing).
var gomemlimitUnits = []struct {
	suffix string
	mult   int64
}{
	{"TIB", 1024 * 1024 * 1024 * 1024},
	{"GIB", 1024 * 1024 * 1024},
	{"MIB", 1024 * 1024},
	{"KIB", 1024},
	{"TB", 1000 * 1000 * 1000 * 1000},
	{"GB", 1000 * 1000 * 1000},
	{"MB", 1000 * 1000},
	{"KB", 1000},
	{"B", 1},
}

// parseGOMEMLIMIT parses Go 1.19+ GOMEMLIMIT values like "512MiB", "8GiB", "1e9".
func parseGOMEMLIMIT(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	s = strings.ToUpper(s)
	var mult int64 = 1
	for _, unit := range gomemlimitUnits {
		if strings.HasSuffix(s, unit.suffix) {
			mult, s = unit.mult, strings.TrimSuffix(s, unit.suffix)
			break
		}
	}
	s = strings.TrimSpace(s)
	n, err := strconv.ParseFloat(s, 64)
	if err != nil || n <= 0 {
		return 0
	}
	return int64(n * float64(mult))
}

// readCgroupMemoryMax reads cgroup v2 memory.max (or v1 memory.limit_in_bytes) in bytes.
func readCgroupMemoryMax() int64 {
	// cgroup v2: /sys/fs/cgroup/memory.max (or under slice)
	for _, path := range []string{
		"/sys/fs/cgroup/memory.max",
		"/sys/fs/cgroup/memory/memory.limit_in_bytes",
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		s := strings.TrimSpace(string(data))
		if s == "max" || s == "" {
			continue
		}
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil || n <= 0 {
			continue
		}
		return n
	}
	return 0
}

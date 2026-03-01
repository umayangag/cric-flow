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

// Memory per worker and concurrency knobs come from config (see config.Resources*); fallbacks in config/constants.

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
		// No memory limit detected (no GOMEMLIMIT/cgroup): use CPU-based concurrency for optimum throughput.
		cfg := config.Load()
		if kind == KindPrecompute {
			n = config.ResourcesPrecomputeConcurrencyWhenNoLimit(cfg)
			if n <= 0 {
				n = runtime.NumCPU()
			}
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
	case KindPrecompute, KindExport, KindSeqCalc, KindFielding:
		return cpu * 2
	default:
		return cpu
	}
}

func memoryBasedLimit(kind Kind) int {
	cfg := config.Load()
	limitBytes := detectMemoryLimitBytes()
	if limitBytes <= 0 {
		return 0
	}
	perWorkerMB := effectiveMBPerWorker(kind)
	perWorkerBytes := int64(perWorkerMB) * 1024 * 1024
	if perWorkerBytes <= 0 {
		return 0
	}
	frac := config.ResourcesMemoryUsageFractionPercent(cfg)
	if frac <= 0 {
		frac = config.DefaultMemoryUsageFractionPercent
	}
	// Use up to frac% of limit for workers; rest for runtime, DB, and spikes.
	usable := (limitBytes * int64(frac)) / 100
	n := int(usable / perWorkerBytes)
	if n < 1 {
		n = 1
	}
	// SeqCalc workers run full-format ball_event scans and can each use more than the per-worker estimate.
	// In low-memory containers, cap at 1 so we don't run multiple such workers.
	seqCalcLowGiB := config.ResourcesSeqCalcLowMemoryLimitGiB(cfg)
	if seqCalcLowGiB <= 0 {
		seqCalcLowGiB = config.DefaultSeqCalcLowMemoryLimitGiB
	}
	seqcalcLowMemoryLimitBytes := int64(seqCalcLowGiB) * 1024 * 1024 * 1024
	if kind == KindSeqCalc && limitBytes <= seqcalcLowMemoryLimitBytes && n > 1 {
		n = 1
	}
	slog.Debug("resources: memory-based limit",
		slog.String("kind", string(kind)),
		slog.Int64("limit_mb", limitBytes/(1024*1024)),
		slog.Int("mb_per_worker", perWorkerMB),
		slog.Int("workers", n))
	return n
}

// effectiveMBPerWorker returns MB per worker for the kind: observed from last run when
// UseObservations() and ObservedMBPerWorker(kind) > 0, otherwise config or default constant.
func effectiveMBPerWorker(kind Kind) int {
	if observed := ObservedMBPerWorker(kind); observed > 0 {
		return observed
	}
	return defaultMBPerWorker(kind)
}

func defaultMBPerWorker(kind Kind) int {
	cfg := config.Load()
	switch kind {
	case KindPrecompute:
		return config.ResourcesPrecomputeMBPerWorker(cfg)
	case KindImport:
		return config.ResourcesImportMBPerWorker(cfg)
	case KindExport:
		return config.ResourcesExportMBPerWorker(cfg)
	case KindSeqCalc:
		return config.ResourcesSeqCalcMBPerWorker(cfg)
	case KindFielding:
		return config.ResourcesFieldingMBPerWorker(cfg)
	default:
		return config.ResourcesImportMBPerWorker(cfg)
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
// In containers (Docker/K8s) the limit is usually on the process's cgroup, not the root;
// we read /proc/self/cgroup to get the cgroup path and then memory.max under it.
func readCgroupMemoryMax() int64 {
	// 1) Try cgroup v2 process path (e.g. Docker/K8s: /sys/fs/cgroup/<slice>/memory.max)
	if b := readCgroupV2MemoryMaxFromProc(); b > 0 {
		return b
	}
	// 2) Root / flat paths (host or older cgroup layout)
	for _, path := range []string{
		"/sys/fs/cgroup/memory.max",
		"/sys/fs/cgroup/memory/memory.limit_in_bytes",
	} {
		if b := readMemoryMaxFile(path); b > 0 {
			return b
		}
	}
	return 0
}

// readMemoryMaxFile reads a memory.max or memory.limit_in_bytes file; returns 0 on any failure or "max".
func readMemoryMaxFile(path string) int64 {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	s := strings.TrimSpace(string(data))
	if s == "max" || s == "" {
		return 0
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

// readCgroupV2MemoryMaxFromProc reads /proc/self/cgroup for the unified cgroup path (v2: "0::/path")
// and then /sys/fs/cgroup/<path>/memory.max. Used so containers with a 2GB limit are detected.
func readCgroupV2MemoryMaxFromProc() int64 {
	data, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		return 0
	}
	// Unified cgroup v2: line like "0::/system.slice/docker-<id>.scope"
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Format "0::/path" or "0::path"
		if strings.HasPrefix(line, "0::") {
			path := strings.TrimPrefix(line, "0::")
			if path == "" {
				path = "/"
			} else if path[0] != '/' {
				path = "/" + path
			}
			fullPath := "/sys/fs/cgroup" + path + "/memory.max"
			return readMemoryMaxFile(fullPath)
		}
	}
	return 0
}

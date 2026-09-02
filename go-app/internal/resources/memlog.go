// Package resources: memory and goroutine logging for pipeline diagnostics.
// Use LogMemoryAndGoroutines at key points (e.g. inside the importer) to capture
// heap and goroutine counts so the last log before a crash shows actual usage.

package resources

import (
	"context"
	"log/slog"
	"runtime"
)

// LogMemoryAndGoroutines logs current heap stats and goroutine count with a consistent
// set of attributes (heap_alloc_mb, heap_inuse_mb, heap_sys_mb, num_goroutine).
// Extra attrs are appended for context (e.g. format, phase). Use at pipeline checkpoints
// to observe usage and to capture the last known state before an OOM/crash.
func LogMemoryAndGoroutines(msg string, extra ...slog.Attr) {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	n := runtime.NumGoroutine()
	attrs := make([]slog.Attr, 0, 4+len(extra))
	attrs = append(
		attrs,
		slog.Uint64("heap_alloc_mb", mem.Alloc/(1024*1024)),
		slog.Uint64("heap_inuse_mb", mem.HeapInuse/(1024*1024)),
		slog.Uint64("heap_sys_mb", mem.HeapSys/(1024*1024)),
		slog.Int("num_goroutine", n),
	)
	attrs = append(attrs, extra...)
	slog.Default().LogAttrs(context.TODO(), slog.LevelInfo, msg, attrs...)
}

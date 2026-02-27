package resources

import (
	"log/slog"
	"testing"
)

func TestLogMemoryAndGoroutines(t *testing.T) {
	t.Parallel()
	// Verifies LogMemoryAndGoroutines does not panic and completes.
	// Capturing actual log output would require redirecting slog; we only assert it runs.
	LogMemoryAndGoroutines("test checkpoint")
}

func TestLogMemoryAndGoroutines_WithExtraAttrs(t *testing.T) {
	t.Parallel()
	// With extra attrs appended
	LogMemoryAndGoroutines("pipeline phase", slog.String("format", "T20"), slog.Int("phase", 1))
}

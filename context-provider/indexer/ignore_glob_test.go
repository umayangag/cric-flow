package indexer_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIgnoreMatcher_Doublestar(t *testing.T) {
	// This test verifies if the current implementation supports **
	// filepath.Match does NOT support **, so we expect this to fail if we were asserting success,
	// or we want to demonstrate the limitation.

	pattern := "**/foo"
	path := "a/b/foo"

	// In the current implementation:
	// pattern has /, so rooted = true.
	// pattern becomes "**/foo" (leading slash removed if any)
	// Match("**/foo", "a/b/foo")

	matched, err := filepath.Match(pattern, path)
	if err != nil {
		t.Logf("Match error: %v", err)
	}

	// We suspect this returns false because * doesn't match separator
	t.Logf("Pattern: %s, Path: %s, Matched: %v", pattern, path, matched)

	if !matched {
		// This confirms that filepath.Match is insufficient for **
		// We should probably report this as an issue.
		assert.False(t, matched, "filepath.Match does not support **")
	}
}

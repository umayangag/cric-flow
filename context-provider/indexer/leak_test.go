package indexer_test

import (
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-info-scrapers/context-provider/indexer"
)

func TestScanProject_ResourceLeak(t *testing.T) {
	// Create a temp directory
	tmpDir := t.TempDir()

	// Create 500 small text files
	// If the file descriptor limit is lowered to ~256, this should fail if files aren't closed.
	for i := 0; i < 500; i++ {
		fname := filepath.Join(tmpDir, "file"+strconv.Itoa(i)+".txt")
		err := os.WriteFile(fname, []byte("some content"), 0o600)
		require.NoError(t, err)
	}

	// Get current limits
	var rLimit syscall.Rlimit
	err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit)
	require.NoError(t, err)

	// Save old limit to restore later
	oldLimit := rLimit

	// Set soft limit to 256 (or slightly higher if needed by runtime, but 256 is standard low)
	// We want to trigger "too many open files"
	rLimit.Cur = 256
	// Ensure max is at least 256 (it usually is)
	if rLimit.Max < 256 {
		t.Skip("Hard limit is too low to run this test")
	}

	err = syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rLimit)
	if err != nil {
		t.Logf("Failed to set rlimit: %v. Skipping leak test.", err)
		t.Skip("Skipping due to inability to set rlimit")
	}

	defer func() {
		// Restore limit
		if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &oldLimit); err != nil {
			t.Logf("Failed to restore rlimit: %v", err)
		}
	}()

	// Run ScanProject
	// This calls walkDir, which has the leak.
	ctx, err := indexer.ScanProject(tmpDir)
	require.NoError(t, err, "ScanProject failed, possibly due to resource leak")

	// Verify that ALL files were read successfully.
	// If we ran out of FDs, many files would have empty content and error logs (checked implicitly by missing content).
	count := 0
	var checkNodes func([]indexer.FileNode)
	checkNodes = func(nodes []indexer.FileNode) {
		for _, node := range nodes {
			if node.Type == "file" {
				count++
				// Check if content matches what we wrote
				if node.Content != "some content" {
					t.Errorf(
						"File %s content mismatch. Content: %q, Expected: %q. (Possibly due to 'too many open files')",
						node.Name,
						node.Content,
						"some content",
					)
				}
			}
			if len(node.Children) > 0 {
				checkNodes(node.Children)
			}
		}
	}
	checkNodes(ctx.Structure)

	if count != 500 {
		t.Errorf("Expected 500 files, found %d", count)
	}
}

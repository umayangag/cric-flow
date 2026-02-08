package indexer_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-info-scrapers/context-provider/indexer"
)

func TestScanProject_SymlinkDirectory(t *testing.T) {
	// Create a temp directory structure:
	// tmpDir/
	//   target/       (the "external" directory)
	//     secret.txt
	//   project/      (the root for scanning)
	//     link_to_target -> ../target

	tmpDir := t.TempDir()
	targetDir := filepath.Join(tmpDir, "target")
	err := os.Mkdir(targetDir, 0o755)
	require.NoError(t, err)

	secretFile := filepath.Join(targetDir, "secret.txt")
	err = os.WriteFile(secretFile, []byte("sensitive info"), 0o600)
	require.NoError(t, err)

	projectDir := filepath.Join(tmpDir, "project")
	err = os.Mkdir(projectDir, 0o755)
	require.NoError(t, err)

	// Create symlink: project/link_to_target -> ../target
	linkPath := filepath.Join(projectDir, "link_to_target")
	err = os.Symlink("../target", linkPath)
	require.NoError(t, err)

	// Scan projectDir
	ctx, err := indexer.ScanProject(projectDir)
	require.NoError(t, err)

	// Helper to search for secret.txt
	foundSecret := false
	var checkNodes func([]indexer.FileNode)
	checkNodes = func(nodes []indexer.FileNode) {
		for _, node := range nodes {
			if node.Name == "secret.txt" {
				foundSecret = true
			}
			if len(node.Children) > 0 {
				checkNodes(node.Children)
			}
		}
	}
	checkNodes(ctx.Structure)

	assert.False(t, foundSecret, "Should not follow symlinks to external directories")
}

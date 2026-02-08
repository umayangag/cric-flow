package indexer_test

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-info-scrapers/context-provider/indexer"
)

func TestScanProject_LogsErrors(t *testing.T) {
	// Create a temp directory for the project
	tmpDir := t.TempDir()

	// 1. Create a malformed Go file
	badGoFile := filepath.Join(tmpDir, "bad.go")
	err := os.WriteFile(badGoFile, []byte("package"), 0o600) // Incomplete package decl
	require.NoError(t, err, "Failed to create bad go file")

	// 2. Create a malformed Python file
	badPyFile := filepath.Join(tmpDir, "bad.py")
	err = os.WriteFile(badPyFile, []byte("def"), 0o600) // Incomplete def
	require.NoError(t, err, "Failed to create bad py file")

	// Capture logs
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer func() {
		log.SetOutput(os.Stderr) // Restore
	}()

	// Run ScanProject
	_, err = indexer.ScanProject(tmpDir)
	require.NoError(t, err, "ScanProject failed unexpectedly")

	// Check logs
	output := buf.String()

	assert.Contains(t, output, "warn: failed to parse Go file", "Expected log warning for Go file")
	assert.Contains(t, output, "warn: failed to parse Python file", "Expected log warning for Python file")
}

func TestScanProject_LargeFileTruncation(t *testing.T) {
	// Create a temp directory
	tmpDir := t.TempDir()

	// 1. Create a large file (> maxContentSize)
	// maxContentSize is 20KB (20 * 1024). Let's create 25KB.
	largeContent := make([]byte, indexer.MaxContentSize+5*1024)
	for i := range largeContent {
		largeContent[i] = 'a'
	}
	largeFile := filepath.Join(tmpDir, "large.txt")
	err := os.WriteFile(largeFile, largeContent, 0o600)
	require.NoError(t, err, "Failed to create large file")

	// 2. Create a small file
	smallContent := []byte("small content")
	smallFile := filepath.Join(tmpDir, "small.txt")
	err = os.WriteFile(smallFile, smallContent, 0o600)
	require.NoError(t, err, "Failed to create small file")

	// Run ScanProject
	ctx, err := indexer.ScanProject(tmpDir)
	require.NoError(t, err, "ScanProject failed")

	// Find the files in the result
	var foundLarge, foundSmall bool

	// Helper to walk the result nodes
	var checkNodes func([]indexer.FileNode)
	checkNodes = func(nodes []indexer.FileNode) {
		for _, node := range nodes {
			if node.Type == "file" {
				switch node.Name {
				case "large.txt":
					foundLarge = true
					assert.True(
						t,
						strings.HasSuffix(node.Content, "... (truncated)"),
						"Expected large file to be truncated",
					)
					assert.LessOrEqual(
						t,
						len(node.Content),
						indexer.MaxContentSize+len("\n... (truncated)"),
						"Content size exceeds expected limit",
					)
				case "small.txt":
					foundSmall = true
					assert.Equal(t, string(smallContent), node.Content, "Expected small file content match")
				}
			}
			if len(node.Children) > 0 {
				checkNodes(node.Children)
			}
		}
	}

	checkNodes(ctx.Structure)

	assert.True(t, foundLarge, "large.txt not found in scan results")
	assert.True(t, foundSmall, "small.txt not found in scan results")
}

func TestScanProject_SensitiveFiles(t *testing.T) {
	// Create a temp directory
	tmpDir := t.TempDir()

	// Create sensitive files
	sensitive := []string{"secrets.json", ".env", "id_rsa"}
	for _, name := range sensitive {
		path := filepath.Join(tmpDir, name)
		err := os.WriteFile(path, []byte("secret"), 0o600)
		require.NoError(t, err, "Failed to create %s", name)
	}

	// Create a normal file
	normalFile := filepath.Join(tmpDir, "normal.txt")
	err := os.WriteFile(normalFile, []byte("normal"), 0o600)
	require.NoError(t, err, "Failed to create normal file")

	// Run ScanProject
	ctx, err := indexer.ScanProject(tmpDir)
	require.NoError(t, err, "ScanProject failed")

	// Check results
	foundNormal := false
	var checkNodes func([]indexer.FileNode)
	checkNodes = func(nodes []indexer.FileNode) {
		for _, node := range nodes {
			if node.Name == "normal.txt" {
				foundNormal = true
			}
			for _, s := range sensitive {
				if node.Name == s {
					t.Errorf(
						"Found sensitive file %s in scan results",
						s,
					) // keeping this as it loops inside checkNodes, but could be asserting not equal
				}
			}
			if len(node.Children) > 0 {
				checkNodes(node.Children)
			}
		}
	}
	checkNodes(ctx.Structure)

	assert.True(t, foundNormal, "normal.txt not found")
}

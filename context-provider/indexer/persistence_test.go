package indexer_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-info-scrapers/context-provider/indexer"
)

func TestSaveAndLoadContext(t *testing.T) {
	// Create a temporary directory
	tempDir, err := os.MkdirTemp("", "indexer_test")
	require.NoError(t, err, "Failed to create temp dir")
	defer os.RemoveAll(tempDir)

	// Create a dummy context
	originalCtx := &indexer.ProjectContext{
		Root: tempDir,
		Stats: indexer.Stats{
			Files:   10,
			GoFiles: 5,
			PyFiles: 5,
		},
		Structure: []indexer.FileNode{
			{Name: "main.go", Type: "file"},
		},
	}

	// Test SaveContext
	err = indexer.SaveContext(tempDir, originalCtx)
	require.NoError(t, err, "SaveContext failed")

	// Verify file exists
	indexPath := indexer.GetIndexPath(tempDir)
	_, err = os.Stat(indexPath)
	assert.False(t, os.IsNotExist(err), "Index file was not created at %s", indexPath)

	// Test LoadContext
	loadedCtx, err := indexer.LoadContext(tempDir)
	require.NoError(t, err, "LoadContext failed")

	// Verify content
	assert.Equal(t, originalCtx.Stats.Files, loadedCtx.Stats.Files, "Expected Files mismatch")
	assert.Equal(t, originalCtx.Stats.GoFiles, loadedCtx.Stats.GoFiles, "Expected GoFiles mismatch")
	require.Equal(t, len(originalCtx.Structure), len(loadedCtx.Structure), "Expected Structure len mismatch")
	assert.Equal(t, originalCtx.Structure[0].Name, loadedCtx.Structure[0].Name, "Expected first node name mismatch")
}

func TestLoadContext_NotFound(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "indexer_test_empty")
	require.NoError(t, err, "Failed to create temp dir")
	defer os.RemoveAll(tempDir)

	_, err = indexer.LoadContext(tempDir)
	assert.Error(t, err, "Expected error when loading non-existent context")
}

func TestSaveContext_Error(t *testing.T) {
	// Use a read-only directory to force error
	tempDir, err := os.MkdirTemp("", "indexer_test_ro")
	require.NoError(t, err, "Failed to create temp dir")
	defer os.RemoveAll(tempDir)

	// Create .junie directory with 000 permissions
	junieDir := filepath.Join(tempDir, ".junie")
	// If .../.junie is a file, MkdirAll should fail.

	f, _ := os.Create(junieDir)
	f.Close()

	ctx := &indexer.ProjectContext{}
	err = indexer.SaveContext(tempDir, ctx)
	assert.Error(t, err, "Expected error when saving to invalid directory structure")
}

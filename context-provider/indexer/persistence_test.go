package indexer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveAndLoadContext(t *testing.T) {
	// Create a temporary directory
	tempDir, err := os.MkdirTemp("", "indexer_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a dummy context
	originalCtx := &ProjectContext{
		Root: tempDir,
		Stats: Stats{
			Files:   10,
			GoFiles: 5,
			PyFiles: 5,
		},
		Structure: []FileNode{
			{Name: "main.go", Type: "file"},
		},
	}

	// Test SaveContext
	err = SaveContext(tempDir, originalCtx)
	if err != nil {
		t.Fatalf("SaveContext failed: %v", err)
	}

	// Verify file exists
	indexPath := GetIndexPath(tempDir)
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		t.Errorf("Index file was not created at %s", indexPath)
	}

	// Test LoadContext
	loadedCtx, err := LoadContext(tempDir)
	if err != nil {
		t.Fatalf("LoadContext failed: %v", err)
	}

	// Verify content
	if loadedCtx.Stats.Files != originalCtx.Stats.Files {
		t.Errorf("Expected Files %d, got %d", originalCtx.Stats.Files, loadedCtx.Stats.Files)
	}
	if loadedCtx.Stats.GoFiles != originalCtx.Stats.GoFiles {
		t.Errorf("Expected GoFiles %d, got %d", originalCtx.Stats.GoFiles, loadedCtx.Stats.GoFiles)
	}
	if len(loadedCtx.Structure) != len(originalCtx.Structure) {
		t.Errorf("Expected Structure len %d, got %d", len(originalCtx.Structure), len(loadedCtx.Structure))
	}
	if loadedCtx.Structure[0].Name != originalCtx.Structure[0].Name {
		t.Errorf("Expected first node name %s, got %s", originalCtx.Structure[0].Name, loadedCtx.Structure[0].Name)
	}
}

func TestLoadContext_NotFound(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "indexer_test_empty")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	_, err = LoadContext(tempDir)
	if err == nil {
		t.Error("Expected error when loading non-existent context, got nil")
	}
}

func TestSaveContext_Error(t *testing.T) {
	// Use a read-only directory to force error
	tempDir, err := os.MkdirTemp("", "indexer_test_ro")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create .junie directory with 000 permissions
	junieDir := filepath.Join(tempDir, ".junie")
	if err := os.Mkdir(junieDir, 0000); err != nil {
		t.Logf("Expected failure or ignoring failure to create dir: %v", err)
	}
	// Alternatively, use a file as directory
	// Create a file at 'dir' so MkdirAll fails?
	// MkdirAll returns nil if path exists as dir, but error if exists as file.

	// Let's create a file where the directory should be.
	// .junie is a directory.
	// If we create a file named .junie, GetIndexPath returns .../.junie/context_index.json
	// MkdirAll(path/filepath.Dir(path)) -> MkdirAll(.../.junie)
	// If .../.junie is a file, MkdirAll should fail.

	f, _ := os.Create(junieDir)
	f.Close()

	ctx := &ProjectContext{}
	err = SaveContext(tempDir, ctx)
	if err == nil {
		t.Error("Expected error when saving to invalid directory structure")
	}
}

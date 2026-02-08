package indexer

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanProject_LogsErrors(t *testing.T) {
	// Create a temp directory for the project
	tmpDir := t.TempDir()

	// 1. Create a malformed Go file
	badGoFile := filepath.Join(tmpDir, "bad.go")
	if err := os.WriteFile(badGoFile, []byte("package"), 0o600); err != nil { // Incomplete package decl
		t.Fatalf("Failed to create bad go file: %v", err)
	}

	// 2. Create a malformed Python file
	badPyFile := filepath.Join(tmpDir, "bad.py")
	if err := os.WriteFile(badPyFile, []byte("def"), 0o600); err != nil { // Incomplete def
		t.Fatalf("Failed to create bad py file: %v", err)
	}

	// Capture logs
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer func() {
		log.SetOutput(os.Stderr) // Restore
	}()

	// Run ScanProject
	_, err := ScanProject(tmpDir)
	if err != nil {
		t.Fatalf("ScanProject failed unexpectedly: %v", err)
	}

	// Check logs
	output := buf.String()

	if !strings.Contains(output, "warn: failed to parse Go file") {
		t.Errorf("Expected log warning for Go file, got: %s", output)
	}
	if !strings.Contains(output, "warn: failed to parse Python file") {
		t.Errorf("Expected log warning for Python file, got: %s", output)
	}
}

func TestScanProject_LargeFileTruncation(t *testing.T) {
	// Create a temp directory
	tmpDir := t.TempDir()

	// 1. Create a large file (> maxContentSize)
	// maxContentSize is 20KB (20 * 1024). Let's create 25KB.
	largeContent := make([]byte, maxContentSize+5*1024)
	for i := range largeContent {
		largeContent[i] = 'a'
	}
	largeFile := filepath.Join(tmpDir, "large.txt")
	if err := os.WriteFile(largeFile, largeContent, 0o600); err != nil {
		t.Fatalf("Failed to create large file: %v", err)
	}

	// 2. Create a small file
	smallContent := []byte("small content")
	smallFile := filepath.Join(tmpDir, "small.txt")
	if err := os.WriteFile(smallFile, smallContent, 0o600); err != nil {
		t.Fatalf("Failed to create small file: %v", err)
	}

	// Run ScanProject
	ctx, err := ScanProject(tmpDir)
	if err != nil {
		t.Fatalf("ScanProject failed: %v", err)
	}

	// Find the files in the result
	var foundLarge, foundSmall bool

	// Helper to walk the result nodes
	var checkNodes func([]FileNode)
	checkNodes = func(nodes []FileNode) {
		for _, node := range nodes {
			if node.Type == "file" {
				switch node.Name {
				case "large.txt":
					foundLarge = true
					if !strings.HasSuffix(node.Content, "... (truncated)") {
						t.Errorf(
							"Expected large file to be truncated, got suffix: %q",
							node.Content[len(node.Content)-20:],
						)
					}
					if len(node.Content) > maxContentSize+len("\n... (truncated)") {
						t.Errorf("Content size %d exceeds expected limit", len(node.Content))
					}
				case "small.txt":
					foundSmall = true
					if node.Content != string(smallContent) {
						t.Errorf("Expected small file content %q, got %q", string(smallContent), node.Content)
					}
				}
			}
			if len(node.Children) > 0 {
				checkNodes(node.Children)
			}
		}
	}

	checkNodes(ctx.Structure)

	if !foundLarge {
		t.Error("large.txt not found in scan results")
	}
	if !foundSmall {
		t.Error("small.txt not found in scan results")
	}
}

func TestScanProject_SensitiveFiles(t *testing.T) {
	// Create a temp directory
	tmpDir := t.TempDir()

	// Create sensitive files
	sensitive := []string{"secrets.json", ".env", "id_rsa"}
	for _, name := range sensitive {
		path := filepath.Join(tmpDir, name)
		if err := os.WriteFile(path, []byte("secret"), 0o600); err != nil {
			t.Fatalf("Failed to create %s: %v", name, err)
		}
	}

	// Create a normal file
	normalFile := filepath.Join(tmpDir, "normal.txt")
	if err := os.WriteFile(normalFile, []byte("normal"), 0o600); err != nil {
		t.Fatalf("Failed to create normal file: %v", err)
	}

	// Run ScanProject
	ctx, err := ScanProject(tmpDir)
	if err != nil {
		t.Fatalf("ScanProject failed: %v", err)
	}

	// Check results
	foundNormal := false
	var checkNodes func([]FileNode)
	checkNodes = func(nodes []FileNode) {
		for _, node := range nodes {
			if node.Name == "normal.txt" {
				foundNormal = true
			}
			for _, s := range sensitive {
				if node.Name == s {
					t.Errorf("Found sensitive file %s in scan results", s)
				}
			}
			if len(node.Children) > 0 {
				checkNodes(node.Children)
			}
		}
	}
	checkNodes(ctx.Structure)

	if !foundNormal {
		t.Error("normal.txt not found")
	}
}

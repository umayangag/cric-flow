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

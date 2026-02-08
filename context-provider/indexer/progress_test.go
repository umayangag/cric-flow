package indexer_test

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/umayangag/cric-info-scrapers/context-provider/indexer"
)

func TestScanProject_LogsProgress(t *testing.T) {
	// Create a temp directory for the project
	tmpDir := t.TempDir()

	// Create many files to trigger progress logging
	// Assuming the interval will be 100, we create 105 files.
	for i := 0; i < 105; i++ {
		fname := filepath.Join(tmpDir, fmt.Sprintf("file_%d.txt", i))
		if err := os.WriteFile(fname, []byte("content"), 0o600); err != nil {
			t.Fatalf("Failed to create file: %v", err)
		}
	}

	// Capture logs
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer func() {
		log.SetOutput(os.Stderr) // Restore
	}()

	// Run ScanProject
	_, err := indexer.ScanProject(tmpDir)
	if err != nil {
		t.Fatalf("ScanProject failed unexpectedly: %v", err)
	}

	// Check logs
	output := buf.String()
	// We expect to see a log message indicating progress.
	// The exact message depends on implementation, but let's assume "Indexed X files"
	expectedSubstring := "Indexed 100 files"
	if !strings.Contains(output, expectedSubstring) {
		t.Errorf("Expected progress log containing '%s', got:\n%s", expectedSubstring, output)
	}
}

package indexer_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/umayangag/cric-info-scrapers/context-provider/indexer"
)

func TestLargeFileProtection(t *testing.T) {
	// Save original limit and restore after test
	originalLimit := indexer.MaxParseFileSize
	defer func() { indexer.MaxParseFileSize = originalLimit }()

	// Set a small limit for testing
	indexer.MaxParseFileSize = 100

	tmpDir := t.TempDir()

	// 1. Test Large Go File
	largeGoFile := filepath.Join(tmpDir, "large.go")
	content := "package main\n"
	for i := 0; i < 200; i++ {
		content += "// some comment to fill space\n"
	}
	if err := os.WriteFile(largeGoFile, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := indexer.ParseGo(largeGoFile)
	if err == nil {
		t.Error("ParseGo should have failed for file larger than limit")
	}

	// 2. Test Large Py File
	largePyFile := filepath.Join(tmpDir, "large.py")
	pyContent := "# python file\n"
	for i := 0; i < 200; i++ {
		pyContent += "# some comment to fill space\n"
	}
	if err := os.WriteFile(largePyFile, []byte(pyContent), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err = indexer.ParsePy(largePyFile)
	if err == nil {
		t.Error("ParsePy should have failed for file larger than limit")
	}

	// 3. Test Small File (Should pass)
	smallGoFile := filepath.Join(tmpDir, "small.go")
	smallContent := "package main\nfunc Foo() {}\n"
	if err := os.WriteFile(smallGoFile, []byte(smallContent), 0o600); err != nil {
		t.Fatal(err)
	}

	// Reset limit to allow small file
	indexer.MaxParseFileSize = int64(len(smallContent) + 100)
	_, err = indexer.ParseGo(smallGoFile)
	if err != nil {
		t.Errorf("ParseGo failed for small file: %v", err)
	}
}

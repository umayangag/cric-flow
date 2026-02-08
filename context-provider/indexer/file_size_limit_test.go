package indexer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLargeFileProtection(t *testing.T) {
	// Save original limit and restore after test
	originalLimit := MaxParseFileSize
	defer func() { MaxParseFileSize = originalLimit }()

	// Set a small limit for testing
	MaxParseFileSize = 100

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

	_, err := ParseGo(largeGoFile)
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

	_, err = ParsePy(largePyFile)
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
	MaxParseFileSize = int64(len(smallContent) + 100)
	_, err = ParseGo(smallGoFile)
	if err != nil {
		t.Errorf("ParseGo failed for small file: %v", err)
	}
}

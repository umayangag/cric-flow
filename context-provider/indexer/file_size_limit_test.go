package indexer_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-info-scrapers/context-provider/indexer"
)

func TestLargeFileProtection(t *testing.T) {
	// Set a small limit for testing via parameter, not global
	limit := int64(100)

	tmpDir := t.TempDir()

	// 1. Test Large Go File
	largeGoFile := filepath.Join(tmpDir, "large.go")
	content := "package main\n"
	for i := 0; i < 200; i++ {
		content += "// some comment to fill space\n"
	}
	err := os.WriteFile(largeGoFile, []byte(content), 0o600)
	require.NoError(t, err)

	_, err = indexer.ParseGo(largeGoFile, limit)
	assert.Error(t, err, "ParseGo should have failed for file larger than limit")
	assert.ErrorContains(t, err, "file too large", "Error message should indicate file size limit")

	// 2. Test Large Py File
	largePyFile := filepath.Join(tmpDir, "large.py")
	pyContent := "# python file\n"
	for i := 0; i < 200; i++ {
		pyContent += "# some comment to fill space\n"
	}
	err = os.WriteFile(largePyFile, []byte(pyContent), 0o600)
	require.NoError(t, err)

	p, err := indexer.NewPythonBatchParser(limit)
	require.NoError(t, err)
	defer p.Close()

	_, err = p.Parse(largePyFile)
	assert.Error(t, err, "ParsePy should have failed for file larger than limit")
	assert.ErrorContains(t, err, "file too large", "Error message should indicate file size limit")

	// 3. Test Small Go File (Should pass)
	smallGoFile := filepath.Join(tmpDir, "small.go")
	smallContent := "package main\nfunc Foo() {}\n"
	err = os.WriteFile(smallGoFile, []byte(smallContent), 0o600)
	require.NoError(t, err)

	// Use a limit that allows the file
	safeLimit := int64(len(smallContent) + 100)
	_, err = indexer.ParseGo(smallGoFile, safeLimit)
	require.NoError(t, err, "ParseGo failed for small file")

	// 4. Test Small Py File (Should pass)
	smallPyFile := filepath.Join(tmpDir, "small.py")
	smallPyContent := "def foo(): pass\n"
	err = os.WriteFile(smallPyFile, []byte(smallPyContent), 0o600)
	require.NoError(t, err)

	// Reuse parser 'p' which has limit=100. smallPyContent is small enough.
	_, err = p.Parse(smallPyFile)
	require.NoError(t, err, "ParsePy failed for small file")
}

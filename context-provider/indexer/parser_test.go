package indexer_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-info-scrapers/context-provider/indexer"
)

func parsePyHelper(_ *testing.T, path string) ([]indexer.Symbol, error) {
	p, err := indexer.NewPythonBatchParser(indexer.DefaultMaxParseFileSize)
	if err != nil {
		return nil, err
	}
	defer p.Close()
	return p.Parse(path)
}

func TestParseGo(t *testing.T) {
	content := `package main

// MyFunc does something
func MyFunc() {}

type MyStruct struct {}

func (s *MyStruct) Method() {}
`
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.go")
	err := os.WriteFile(tmpFile, []byte(content), 0o600)
	require.NoError(t, err, "Failed to write temp file")

	symbols, err := indexer.ParseGo(tmpFile, indexer.DefaultMaxParseFileSize)
	require.NoError(t, err, "ParseGo failed")

	expected := []struct {
		Name string
		Kind string
	}{
		{"MyFunc", "function"},
		{"MyStruct", "type"},
		{"Method", "method"},
	}

	require.Equal(t, len(expected), len(symbols), "Expected %d symbols, got %d", len(expected), len(symbols))

	for i, sym := range symbols {
		assert.Equal(t, expected[i].Name, sym.Name, "Symbol %d: name mismatch", i)
		assert.Equal(t, expected[i].Kind, sym.Kind, "Symbol %d: kind mismatch", i)
	}
}

func TestParsePy(t *testing.T) {
	content := `
class MyClass:
    def method(self):
        pass

def my_func():
    pass
`
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.py")
	err := os.WriteFile(tmpFile, []byte(content), 0o600)
	require.NoError(t, err, "Failed to write temp file")

	symbols, err := parsePyHelper(t, tmpFile)
	require.NoError(t, err, "ParsePy failed")

	expected := []struct {
		Name string
		Kind string
	}{
		{"MyClass", "class"},
		{"method", "method"},
		{"my_func", "function"},
	}

	require.Equal(t, len(expected), len(symbols), "Expected %d symbols, got %d", len(expected), len(symbols))

	for i, sym := range symbols {
		assert.Equal(t, expected[i].Name, sym.Name, "Symbol %d: name mismatch", i)
		assert.Equal(t, expected[i].Kind, sym.Kind, "Symbol %d: kind mismatch", i)
	}
}

func TestParsePy_Symlink(t *testing.T) {
	tmpDir := t.TempDir()
	targetFile := filepath.Join(tmpDir, "target.py")
	err := os.WriteFile(targetFile, []byte("def target(): pass"), 0o600)
	require.NoError(t, err, "Failed to write target file")

	symlinkPath := filepath.Join(tmpDir, "link.py")
	if err := os.Symlink(targetFile, symlinkPath); err != nil {
		t.Skipf("Symlinks not supported on this OS: %v", err)
	}

	_, err = parsePyHelper(t, symlinkPath)
	assert.Error(t, err, "Expected error for symlink")
}

func TestParsePy_Docstrings(t *testing.T) {
	content := `
class MyClass:
    """Class docstring"""
    def method(self):
        """Method docstring"""
        pass

def my_func():
    """Function docstring"""
    pass
`
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test_doc.py")
	err := os.WriteFile(tmpFile, []byte(content), 0o600)
	require.NoError(t, err, "Failed to write temp file")

	symbols, err := parsePyHelper(t, tmpFile)
	require.NoError(t, err, "ParsePy failed")

	expected := map[string]string{
		"MyClass": "Class docstring",
		"method":  "Method docstring",
		"my_func": "Function docstring",
	}

	for _, sym := range symbols {
		if want, ok := expected[sym.Name]; ok {
			assert.Equal(t, want, sym.Doc, "Symbol %s: docstring mismatch", sym.Name)
		}
	}
}

func TestParsePy_Multiline(t *testing.T) {
	content := `
def my_func(
    arg1,
    arg2
):
    pass
`
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test_multiline.py")
	err := os.WriteFile(tmpFile, []byte(content), 0o600)
	require.NoError(t, err, "Failed to write temp file")

	symbols, err := parsePyHelper(t, tmpFile)
	require.NoError(t, err, "ParsePy failed")

	found := false
	for _, sym := range symbols {
		if sym.Name == "my_func" {
			found = true
			break
		}
	}
	assert.True(t, found, "Failed to find multiline function definition")
}

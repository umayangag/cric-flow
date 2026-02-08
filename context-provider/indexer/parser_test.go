package indexer_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/umayangag/cric-info-scrapers/context-provider/indexer"
)

func TestParseGo(t *testing.T) {
	content := `package main

// MyFunc does something
func MyFunc() {}

type MyStruct struct {}

func (s *MyStruct) Method() {}
`
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.go")
	if err := os.WriteFile(tmpFile, []byte(content), 0o600); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	symbols, err := indexer.ParseGo(tmpFile)
	if err != nil {
		t.Fatalf("ParseGo failed: %v", err)
	}

	expected := []struct {
		Name string
		Kind string
	}{
		{"MyFunc", "function"},
		{"MyStruct", "type"},
		{"Method", "method"},
	}

	if len(symbols) != len(expected) {
		t.Fatalf("Expected %d symbols, got %d", len(expected), len(symbols))
	}

	for i, sym := range symbols {
		if sym.Name != expected[i].Name {
			t.Errorf("Symbol %d: expected name %s, got %s", i, expected[i].Name, sym.Name)
		}
		if sym.Kind != expected[i].Kind {
			t.Errorf("Symbol %d: expected kind %s, got %s", i, expected[i].Kind, sym.Kind)
		}
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
	if err := os.WriteFile(tmpFile, []byte(content), 0o600); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	symbols, err := indexer.ParsePy(tmpFile)
	if err != nil {
		t.Fatalf("ParsePy failed: %v", err)
	}

	expected := []struct {
		Name string
		Kind string
	}{
		{"MyClass", "class"},
		{"method", "method"},
		{"my_func", "function"},
	}

	if len(symbols) != len(expected) {
		t.Fatalf("Expected %d symbols, got %d", len(expected), len(symbols))
	}

	for i, sym := range symbols {
		if sym.Name != expected[i].Name {
			t.Errorf("Symbol %d: expected name %s, got %s", i, expected[i].Name, sym.Name)
		}
		if sym.Kind != expected[i].Kind {
			t.Errorf("Symbol %d: expected kind %s, got %s", i, expected[i].Kind, sym.Kind)
		}
	}
}

func TestParsePy_Symlink(t *testing.T) {
	tmpDir := t.TempDir()
	targetFile := filepath.Join(tmpDir, "target.py")
	if err := os.WriteFile(targetFile, []byte("def target(): pass"), 0o600); err != nil {
		t.Fatalf("Failed to write target file: %v", err)
	}

	symlinkPath := filepath.Join(tmpDir, "link.py")
	if err := os.Symlink(targetFile, symlinkPath); err != nil {
		t.Skipf("Symlinks not supported on this OS: %v", err)
	}

	_, err := indexer.ParsePy(symlinkPath)
	if err == nil {
		t.Error("Expected error for symlink, got nil")
	}
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
	if err := os.WriteFile(tmpFile, []byte(content), 0o600); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	symbols, err := indexer.ParsePy(tmpFile)
	if err != nil {
		t.Fatalf("ParsePy failed: %v", err)
	}

	expected := map[string]string{
		"MyClass": "Class docstring",
		"method":  "Method docstring",
		"my_func": "Function docstring",
	}

	for _, sym := range symbols {
		if want, ok := expected[sym.Name]; ok {
			if sym.Doc != want {
				t.Errorf("Symbol %s: expected doc %q, got %q", sym.Name, want, sym.Doc)
			}
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
	if err := os.WriteFile(tmpFile, []byte(content), 0o600); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	symbols, err := indexer.ParsePy(tmpFile)
	if err != nil {
		t.Fatalf("ParsePy failed: %v", err)
	}

	found := false
	for _, sym := range symbols {
		if sym.Name == "my_func" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Failed to find multiline function definition")
	}
}

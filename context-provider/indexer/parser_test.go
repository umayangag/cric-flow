package indexer

import (
	"os"
	"path/filepath"
	"testing"
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
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	symbols := ParseGo(tmpFile)

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
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	symbols := ParsePy(tmpFile)

	expected := []struct {
		Name string
		Kind string
	}{
		{"MyClass", "class"},
		{"method", "function"},
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

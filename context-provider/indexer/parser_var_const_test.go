package indexer_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-info-scrapers/context-provider/indexer"
)

func TestParseGo_VarsAndConsts(t *testing.T) {
	content := `package main

// MyVar is a variable
var MyVar = 10

// MyConst is a constant
const MyConst = 20

var (
	// GroupVar1 doc
	GroupVar1 = "a"
	GroupVar2 = "b"
)

const (
	GroupConst1 = 1
	// GroupConst2 doc
	GroupConst2 = 2
)
`
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test_vars_consts.go")
	err := os.WriteFile(tmpFile, []byte(content), 0o600)
	require.NoError(t, err, "Failed to write temp file")

	symbols, err := indexer.ParseGo(tmpFile, indexer.DefaultMaxParseFileSize)
	require.NoError(t, err, "ParseGo failed")

	expected := []struct {
		Name string
		Kind string
		Doc  string
	}{
		{"MyVar", "var", "MyVar is a variable"},
		{"MyConst", "const", "MyConst is a constant"},
		{"GroupVar1", "var", "GroupVar1 doc"},
		{"GroupVar2", "var", ""},
		{"GroupConst1", "const", ""},
		{"GroupConst2", "const", "GroupConst2 doc"},
	}

	// Helper to find symbol
	findSymbol := func(name string) *indexer.Symbol {
		for _, s := range symbols {
			if s.Name == name {
				return &s
			}
		}
		return nil
	}

	for _, exp := range expected {
		sym := findSymbol(exp.Name)
		if assert.NotNil(t, sym, "Symbol %s not found", exp.Name) {
			assert.Equal(t, exp.Kind, sym.Kind, "Symbol %s: kind mismatch", exp.Name)
			if exp.Doc != "" {
				assert.Equal(t, exp.Doc, sym.Doc, "Symbol %s: doc mismatch", exp.Name)
			}
		}
	}

	// Check total count to ensure we are not missing anything or adding extras
	assert.Equal(t, len(expected), len(symbols), "Unexpected number of symbols")
}

package indexer_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-info-scrapers/context-provider/indexer"
)

func TestParsePy_NoPython(t *testing.T) {
	// Set PATH to empty to ensure no python executable is found
	t.Setenv("PATH", "")

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.py")
	err := os.WriteFile(tmpFile, []byte("print('hello')"), 0o600)
	require.NoError(t, err)

	_, err = indexer.ParsePy(tmpFile)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "neither 'python3' nor 'python' were found in PATH")
}

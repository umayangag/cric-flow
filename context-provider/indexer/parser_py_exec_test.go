package indexer_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-info-scrapers/context-provider/indexer"
)

func TestParsePy_NoPython(t *testing.T) {
	// Set PATH to empty to ensure no python executable is found
	t.Setenv("PATH", "")

	_, err := indexer.NewPythonBatchParser(indexer.DefaultMaxParseFileSize)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "neither 'python3' nor 'python' were found in PATH")
}

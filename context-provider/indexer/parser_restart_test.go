package indexer

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPythonBatchParser_RestartOnCrash(t *testing.T) {
	parser, err := NewPythonBatchParser(DefaultMaxParseFileSize)
	if err != nil {
		t.Skipf("Skipping test, python environment not set up: %v", err)
	}
	require.NoError(t, err)
	defer parser.Close()

	// 1. Create a dummy python file
	tmpDir := t.TempDir()
	pyFile := filepath.Join(tmpDir, "test.py")
	content := `
def hello():
    pass
`
	err = os.WriteFile(pyFile, []byte(content), 0o600)
	require.NoError(t, err)

	// 2. Parse successfully first
	symbols, err := parser.Parse(pyFile)
	require.NoError(t, err)
	assert.NotEmpty(t, symbols)
	assert.Equal(t, "hello", symbols[0].Name)

	// 3. Kill the python process
	require.NotNil(t, parser.cmd)
	require.NotNil(t, parser.cmd.Process)

	err = parser.cmd.Process.Kill()
	require.NoError(t, err)

	// Give it a moment to die and for OS to register it
	time.Sleep(100 * time.Millisecond)

	// We might need to Wait() to ensure ProcessState is updated,
	// but parser.cmd.Wait() might be called by the parser logic or we might need to trigger it.
	// If we call Wait() here, we might race with the parser's internal handling if it had any (it doesn't yet).
	// Just killing it is enough for the pipe write to fail or ProcessState.Exited() to be true eventually.
	// However, `p.cmd.ProcessState` is only populated after `Wait()` is called.
	// The current implementation doesn't seem to call `Wait()` until `Close()`.
	// BUT `Parse` checks `p.cmd.ProcessState != nil && p.cmd.ProcessState.Exited()`.
	// If `Wait()` hasn't been called, `ProcessState` might be nil.
	// So the failure might actually come from `fmt.Fprintln(p.stdin, path)` returning "broken pipe".

	// Let's just try to parse.
	symbols2, err2 := parser.Parse(pyFile)

	// Expectation: This should succeed after the fix (auto-restart).
	// Currently: This should fail.
	require.NoError(t, err2, "Should automatically restart and succeed")
	assert.NotEmpty(t, symbols2)
}

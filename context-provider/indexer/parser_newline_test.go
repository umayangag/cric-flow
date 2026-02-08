package indexer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse_NewlineInjection_Desync(t *testing.T) {
	// 2. Setup files
	tempDir := t.TempDir()

	// file2.py has "TargetSymbol"
	file2Path := filepath.Join(tempDir, "file2.py")
	err := os.WriteFile(file2Path, []byte("class TargetSymbol:\n    pass\n"), 0o600)
	require.NoError(t, err)

	// file1.py has "Symbol1"
	file1Path := filepath.Join(tempDir, "file1.py")
	err = os.WriteFile(file1Path, []byte("class Symbol1:\n    pass\n"), 0o600)
	require.NoError(t, err)

	// malicious filename: "file1.py\nfile2.py"
	maliciousName := "file1.py\nfile2.py"
	maliciousPath := filepath.Join(tempDir, maliciousName)

	f, err := os.Create(maliciousPath)
	if err != nil {
		t.Skipf("Skipping test because filesystem doesn't support newlines in filenames: %v", err)
	}
	f.Close()

	// 3. Trigger the injection
	wd, err := os.Getwd()
	require.NoError(t, err)
	defer func() {
		_ = os.Chdir(wd)
	}()
	err = os.Chdir(tempDir)
	require.NoError(t, err)

	// 1. Setup Parser (now it inherits CWD = tempDir)
	parser, err := NewPythonBatchParser(DefaultMaxParseFileSize)
	require.NoError(t, err)
	defer parser.Close()

	// Now we rely on local paths.
	// Verify files exist locally
	_, err = os.Stat(maliciousName)
	require.NoError(t, err, "Malicious file must exist for os.Lstat to pass")

	// Call Parse with the malicious filename
	_, err = parser.Parse(maliciousName)

	// The fix should reject the newline
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "path contains newline")
	}

	// Verify that the parser is still usable (no desync or crash)
	// Now call Parse again with a harmless file
	symbols2, err := parser.Parse("file1.py")
	require.NoError(t, err)

	require.Len(t, symbols2, 1)
	assert.Equal(t, "Symbol1", symbols2[0].Name)
}

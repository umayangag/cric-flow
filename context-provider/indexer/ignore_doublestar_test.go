package indexer_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-info-scrapers/context-provider/indexer"
)

func TestIgnoreMatcher_DoubleStar(t *testing.T) {
	tmpDir := t.TempDir()

	gitIgnoreContent := `
**/node_modules/
foo/**/bar
`
	err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(gitIgnoreContent), 0o600)
	require.NoError(t, err)

	matcher := indexer.NewIgnoreMatcher(tmpDir)

	tests := []struct {
		path   string
		isDir  bool
		ignore bool
	}{
		{"node_modules", true, true},
		{"src/node_modules", true, true},
		{"src/app/node_modules", true, true},
		{"foo/bar", false, true},
		{"foo/baz/bar", false, true},
		{"foo/a/b/c/bar", false, true},
	}

	for _, tt := range tests {
		fullPath := filepath.Join(tmpDir, tt.path)
		got := matcher.ShouldIgnore(fullPath, tt.isDir)
		assert.Equal(t, tt.ignore, got, "Path: %s, Expected Ignore: %v, Got: %v", tt.path, tt.ignore, got)
	}
}

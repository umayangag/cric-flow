package indexer_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-info-scrapers/context-provider/indexer"
)

func TestIgnoreMatcher_Negation(t *testing.T) {
	tmpDir := t.TempDir()

	// .gitignore with negation
	// Ignore all logs, but keep important.log
	gitIgnoreContent := `
*.log
!important.log
/ignore_dir/*
!/ignore_dir/keep_me.txt
`
	err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(gitIgnoreContent), 0o600)
	require.NoError(t, err)

	matcher := indexer.NewIgnoreMatcher(tmpDir)

	tests := []struct {
		path   string
		isDir  bool
		ignore bool
	}{
		{path: "app.log", isDir: false, ignore: true},
		{path: "error.log", isDir: false, ignore: true},
		{path: "important.log", isDir: false, ignore: false}, // Should be un-ignored
		{path: "subdir/app.log", isDir: false, ignore: true},
		{path: "subdir/important.log", isDir: false, ignore: false}, // Pattern !important.log matches anywhere?
		// Wait, standard gitignore: "!important.log" matches "important.log" anywhere if it doesn't have a slash?
		// Let's assume standard behavior for now. If the pattern in .gitignore is just "!important.log", it is not rooted.

		{path: "ignore_dir/stuff.txt", isDir: false, ignore: true},
		{path: "ignore_dir/keep_me.txt", isDir: false, ignore: false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			absPath := filepath.Join(tmpDir, tt.path)
			ignored := matcher.ShouldIgnore(absPath, tt.isDir)
			assert.Equal(t, tt.ignore, ignored, "Path: %s", tt.path)
		})
	}
}

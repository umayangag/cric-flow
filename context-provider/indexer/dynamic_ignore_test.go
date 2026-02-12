package indexer_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-info-scrapers/context-provider/indexer"
)

func TestDynamicIgnores(t *testing.T) {
	// 1. Setup
	tmpDir := t.TempDir()

	// Create structure:
	// root/
	//   node_modules/ (default ignored dir)
	//     pkg.json
	//   .env (default ignored file)
	//   src/
	//     main.go (should be indexed)
	//   dist/ (default ignored dir)
	//     bundle.js

	dirs := []string{"node_modules", "src", "dist"}
	for _, d := range dirs {
		require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, d), 0o755))
	}

	files := map[string]string{
		"node_modules/pkg.json": "{}",
		".env":                  "SECRET=true",
		"src/main.go":           "package main",
		"dist/bundle.js":        "console.log('hi')",
	}
	for p, content := range files {
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, p), []byte(content), 0o600))
	}

	// 2. Scan (Expect defaults to work)
	ctx, err := indexer.ScanProject(tmpDir)
	require.NoError(t, err)

	// Verify defaults
	assertFileExists(t, ctx.Structure, "src/main.go")
	assertFileNotExists(t, ctx.Structure, "node_modules/pkg.json")
	assertFileNotExists(t, ctx.Structure, ".env")
	assertFileNotExists(t, ctx.Structure, "dist/bundle.js")

	// 3. Create .gitignore to OVERRIDE defaults
	// We want to unignore node_modules and .env
	// Note: depending on how we implement defaults, we might need !node_modules/ or !node_modules
	gitIgnoreContent := `
!node_modules/
!.env
`
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(gitIgnoreContent), 0o600))

	// 4. Scan again
	ctx2, err := indexer.ScanProject(tmpDir)
	require.NoError(t, err)

	// Verify overrides
	assertFileExists(t, ctx2.Structure, "src/main.go")

	// These assertions are expected to FAIL currently because of hardcoded ignores
	// But PASS after refactor.
	// To confirm current behavior (that they fail to be unignored), I could invert assertion or just comment them out
	// but the plan says "Create reproduction/verification test".
	// I will leave them as positive assertions of the DESIRED behavior.
	// When I run this test *before* refactor, it should FAIL (which confirms the issue/lack of flexibility).
	assertFileExists(t, ctx2.Structure, "node_modules/pkg.json")
	assertFileExists(t, ctx2.Structure, ".env")

	assertFileNotExists(t, ctx2.Structure, "dist/bundle.js") // Still ignored
}

func assertFileExists(t *testing.T, nodes []indexer.FileNode, path string) {
	t.Helper()
	if !findNodeByPath(nodes, path) {
		t.Errorf("Expected file %s to exist in index, but it was not found", path)
	}
}

func assertFileNotExists(t *testing.T, nodes []indexer.FileNode, path string) {
	t.Helper()
	if findNodeByPath(nodes, path) {
		t.Errorf("Expected file %s to be ignored, but it was found in index", path)
	}
}

func findNodeByPath(nodes []indexer.FileNode, path string) bool {
	for _, node := range nodes {
		if node.Path == path {
			return true
		}
		if node.Type == "dir" {
			if findNodeByPath(node.Children, path) {
				return true
			}
		}
	}
	return false
}

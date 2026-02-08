package indexer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanProject_RespectsGitIgnore(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Create .gitignore
	gitIgnoreContent := `
# Comment
ignored_dir/
secret.txt
*.log
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(gitIgnoreContent), 0o600); err != nil {
		t.Fatalf("Failed to create .gitignore: %v", err)
	}

	// 2. Create structure
	// tmpDir/
	//   .gitignore
	//   main.go
	//   secret.txt      (should be ignored)
	//   app.log         (should be ignored)
	//   ignored_dir/    (should be ignored)
	//     nested.go
	//   included_dir/
	//     utils.go

	createFile(t, tmpDir, "main.go")
	createFile(t, tmpDir, "secret.txt")
	createFile(t, tmpDir, "app.log")
	createDir(t, tmpDir, "ignored_dir")
	createFile(t, tmpDir, "ignored_dir/nested.go")
	createDir(t, tmpDir, "included_dir")
	createFile(t, tmpDir, "included_dir/utils.go")

	// 3. Scan
	ctx, err := ScanProject(tmpDir)
	if err != nil {
		t.Fatalf("ScanProject failed: %v", err)
	}

	// 4. Verify
	// Check if secret.txt exists
	if findNode(ctx.Structure, "secret.txt") {
		t.Errorf("Found secret.txt, should be ignored")
	}
	// Check if app.log exists
	if findNode(ctx.Structure, "app.log") {
		t.Errorf("Found app.log, should be ignored")
	}
	// Check if ignored_dir exists
	if findNode(ctx.Structure, "ignored_dir") {
		t.Errorf("Found ignored_dir, should be ignored")
	}
	// Check if nested.go exists (it shouldn't if parent is ignored)
	if findNode(ctx.Structure, "nested.go") {
		t.Errorf("Found nested.go in ignored_dir, should be ignored")
	}
	// Check if main.go exists
	if !findNode(ctx.Structure, "main.go") {
		t.Errorf("Missing main.go, should be included")
	}
	// Check if included_dir exists
	if !findNode(ctx.Structure, "included_dir") {
		t.Errorf("Missing included_dir, should be included")
	}
	// Check if utils.go exists
	if !findNode(ctx.Structure, "utils.go") {
		t.Errorf("Missing utils.go, should be included")
	}
}

func createFile(t *testing.T, root, path string) {
	fullPath := filepath.Join(root, path)
	if err := os.WriteFile(fullPath, []byte("package main"), 0o600); err != nil {
		t.Fatalf("Failed to create file %s: %v", path, err)
	}
}

func createDir(t *testing.T, root, path string) {
	fullPath := filepath.Join(root, path)
	if err := os.MkdirAll(fullPath, 0o700); err != nil {
		t.Fatalf("Failed to create dir %s: %v", path, err)
	}
}

func findNode(nodes []FileNode, name string) bool {
	for _, node := range nodes {
		if node.Name == name {
			return true
		}
		if len(node.Children) > 0 {
			if findNode(node.Children, name) {
				return true
			}
		}
	}
	return false
}

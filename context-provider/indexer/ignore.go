package indexer

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
)

type IgnoreMatcher struct {
	root    string
	matcher gitignore.Matcher
}

func NewIgnoreMatcher(root string) *IgnoreMatcher {
	m := &IgnoreMatcher{
		root: root,
	}
	m.loadRules()
	return m
}

func (m *IgnoreMatcher) loadRules() {
	var patterns []gitignore.Pattern

	// 1. Defaults
	// Directories
	defaultDirs := []string{
		".git", "node_modules", "dist", "build", "__pycache__",
		".venv", "output", ".junie", ".junie_plans", ".idea", ".vscode",
	}
	for _, d := range defaultDirs {
		// Appending slash to match directories only, consistent with gitignore
		patterns = append(patterns, gitignore.ParsePattern(d+"/", nil))
	}

	// Sensitive Files
	defaultFiles := []string{
		"secrets.json",
		".env", ".env.local", ".env.development", ".env.test", ".env.production",
		"passwd", "shadow", ".htpasswd", ".netrc",
		"id_rsa", "id_dsa", "id_ed25519", "id_ecdsa",
		".pypirc", ".npmrc",
	}
	for _, f := range defaultFiles {
		patterns = append(patterns, gitignore.ParsePattern(f, nil))
	}

	// 2. .gitignore
	gitIgnorePath := filepath.Join(m.root, ".gitignore")
	file, err := os.Open(gitIgnorePath)
	if err == nil {
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			patterns = append(patterns, gitignore.ParsePattern(line, nil))
		}
	}

	m.matcher = gitignore.NewMatcher(patterns)
}

func (m *IgnoreMatcher) ShouldIgnore(path string, isDir bool) bool {
	// path is absolute path. Get relative to root.
	relPath, err := filepath.Rel(m.root, path)
	if err != nil {
		return false
	}
	if relPath == "." {
		return false
	}

	// go-git matcher expects path components split by slash
	// Ensure we use forward slashes for the split even on Windows
	slashPath := filepath.ToSlash(relPath)
	pathParts := strings.Split(slashPath, "/")

	return m.matcher.Match(pathParts, isDir)
}

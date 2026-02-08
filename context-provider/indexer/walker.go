package indexer

import (
	"io"
	"log"
	"os"
	"path/filepath"
)

const (
	maxContentSize      = 20 * 1024
	progressLogInterval = 100
)

var MaxParseFileSize int64 = 10 * 1024 * 1024 // 10MB limit for parsing

var ignoredDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"dist":         true,
	"build":        true,
	"__pycache__":  true,
	".venv":        true,
	"output":       true,
	".junie":       true,
	".junie_plans": true,
	".idea":        true,
	".vscode":      true,
}

var sensitiveFiles = map[string]bool{
	"secrets.json":     true,
	".env":             true,
	".env.local":       true,
	".env.development": true,
	".env.test":        true,
	".env.production":  true,
	"passwd":           true,
	"shadow":           true,
	".htpasswd":        true,
	".netrc":           true,
	"id_rsa":           true,
	"id_dsa":           true,
	"id_ed25519":       true,
	"id_ecdsa":         true,
	".pypirc":          true,
	".npmrc":           true,
}

var textExtensions = map[string]bool{
	".md":        true,
	".txt":       true,
	".json":      true,
	".yml":       true,
	".yaml":      true,
	".toml":      true,
	".ini":       true,
	"Dockerfile": true,
	"Makefile":   true,
	"go.mod":     true,
	"go.sum":     true,
	"go.work":    true,
}

func (s *Stats) increment(path string, isDir bool) {
	if isDir {
		s.Directories++
		return
	}
	s.Files++
	if s.Files%progressLogInterval == 0 {
		log.Printf("Indexed %d files...", s.Files)
	}

	switch filepath.Ext(path) {
	case ".go":
		s.GoFiles++
	case ".py":
		s.PyFiles++
	}
}

func ScanProject(root string) (*ProjectContext, error) {
	log.Printf("Starting scan of %s", root)
	ignoreMatcher := NewIgnoreMatcher(root)
	ctx := &ProjectContext{
		Root:  root,
		Stats: Stats{},
	}

	nodes, err := walkDir(root, root, &ctx.Stats, ignoreMatcher)
	if err != nil {
		return nil, err
	}
	ctx.Structure = nodes
	return ctx, nil
}

func walkDir(root, currentPath string, stats *Stats, matcher *IgnoreMatcher) ([]FileNode, error) {
	entries, err := os.ReadDir(currentPath)
	if err != nil {
		if currentPath == root {
			return nil, err
		}
		log.Printf("warn: skipping directory %s: %v", currentPath, err)
		return nil, nil
	}

	var nodes []FileNode

	for _, entry := range entries {
		name := entry.Name()
		if ignoredDirs[name] || sensitiveFiles[name] || (!entry.Type().IsRegular() && !entry.IsDir()) {
			continue
		}

		fullPath := filepath.Join(currentPath, name)
		relPath, err := filepath.Rel(root, fullPath)
		if err != nil {
			return nil, err
		}

		// Check .gitignore
		if matcher.ShouldIgnore(fullPath, entry.IsDir()) {
			continue
		}

		node := FileNode{
			Name: name,
			Path: relPath,
		}

		if entry.IsDir() {
			node.Type = "dir"

			log.Printf("Scanning directory: %s", relPath)

			stats.increment(fullPath, true)
			children, err := walkDir(root, fullPath, stats, matcher)
			if err != nil {
				return nil, err
			}
			node.Children = children
			// We keep empty directories if they are not ignored, to show structure
			nodes = append(nodes, node)
		} else {
			node.Type = "file"
			stats.increment(fullPath, false)

			// Parsing Logic
			switch filepath.Ext(name) {
			case ".go":
				syms, err := ParseGo(fullPath)
				if err != nil {
					log.Printf("warn: failed to parse Go file %s: %v", relPath, err)
				} else {
					node.Symbols = syms
				}
			case ".py":
				syms, err := ParsePy(fullPath)
				if err != nil {
					log.Printf("warn: failed to parse Python file %s: %v", relPath, err)
				} else {
					node.Symbols = syms
				}
			}

			// Content Logic (for config/docs)
			if shouldReadContent(name) {
f, err := os.Open(fullPath)
if err != nil {
    log.Printf("warn: could not open %s to read content: %v", relPath, err)
} else {
    defer f.Close()

    // Read maxContentSize + 1 to detect truncation necessity
    limitReader := io.LimitReader(f, int64(maxContentSize)+1)
    content, err := io.ReadAll(limitReader)
    if err != nil {
        log.Printf("warn: could not read content of %s: %v", relPath, err)
    } else if len(content) > maxContentSize {
        node.Content = string(content[:maxContentSize]) + "\n... (truncated)"
    } else {
        node.Content = string(content)
    }
}
			}
			nodes = append(nodes, node)
		}
	}
	return nodes, nil
}

func shouldReadContent(name string) bool {
	ext := filepath.Ext(name)
	if textExtensions[ext] || textExtensions[name] {
		return true
	}
	return false
}

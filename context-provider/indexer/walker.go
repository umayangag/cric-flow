package indexer

import (
	"log"
	"os"
	"path/filepath"
)

const maxContentSize = 20 * 1024

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
	switch filepath.Ext(path) {
	case ".go":
		s.GoFiles++
	case ".py":
		s.PyFiles++
	}
}

func ScanProject(root string) (*ProjectContext, error) {
	ctx := &ProjectContext{
		Root:  root,
		Stats: Stats{},
	}

	nodes, err := walkDir(root, root, &ctx.Stats)
	if err != nil {
		return nil, err
	}
	ctx.Structure = nodes
	return ctx, nil
}

func walkDir(root, currentPath string, stats *Stats) ([]FileNode, error) {
	entries, err := os.ReadDir(currentPath)
	if err != nil {
		return nil, err
	}

	var nodes []FileNode

	for _, entry := range entries {
		name := entry.Name()
		if ignoredDirs[name] {
			continue
		}

		fullPath := filepath.Join(currentPath, name)
		relPath, err := filepath.Rel(root, fullPath)
		if err != nil {
			return nil, err
		}

		node := FileNode{
			Name: name,
			Path: relPath,
		}

		if entry.IsDir() {
			if entry.Type()&os.ModeSymlink != 0 {
				continue
			}
			node.Type = "dir"
			stats.increment(fullPath, true)
			children, err := walkDir(root, fullPath, stats)
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
			if shouldReadContent(name) && entry.Type()&os.ModeSymlink == 0 {
				content, err := os.ReadFile(fullPath)
				if err == nil {
					// Truncate if too large (e.g., > 20KB)
					if len(content) > maxContentSize {
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

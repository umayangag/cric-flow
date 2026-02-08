package indexer

import (
	"bufio"
	"log"
	"os"
	"path/filepath"
	"strings"
)

type IgnoreMatcher struct {
	root  string
	rules []ignoreRule
}

type ignoreRule struct {
	pattern string
	dirOnly bool
	rooted  bool // true if pattern contains separator (implies relative to root)
	negate  bool
}

func NewIgnoreMatcher(root string) *IgnoreMatcher {
	matcher := &IgnoreMatcher{
		root: root,
	}
	matcher.loadGitIgnore()
	return matcher
}

func (m *IgnoreMatcher) loadGitIgnore() {
	path := filepath.Join(m.root, ".gitignore")
	file, err := os.Open(path)
	if err != nil {
		return // No .gitignore or can't open
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse rule
		negate := false
		if strings.HasPrefix(line, "!") {
			negate = true
			line = strings.TrimPrefix(line, "!")
		}

		rule := ignoreRule{
			pattern: line,
			negate:  negate,
		}

		if strings.HasSuffix(line, "/") {
			rule.dirOnly = true
			rule.pattern = strings.TrimSuffix(line, "/")
		}

		// If it contains a slash (and it's not just at the end which we removed), it's rooted
		if strings.Contains(rule.pattern, "/") {
			rule.rooted = true
			// Handle leading slash
			rule.pattern = strings.TrimPrefix(rule.pattern, "/")
		}

		m.rules = append(m.rules, rule)
	}
}

func (m *IgnoreMatcher) ShouldIgnore(path string, isDir bool) bool {
	// path is absolute path. Get relative to root.
	relPath, err := filepath.Rel(m.root, path)
	if err != nil {
		return false
	}

	name := filepath.Base(path)

	// Iterate backwards to support negation and overrides
	for i := len(m.rules) - 1; i >= 0; i-- {
		rule := m.rules[i]
		if rule.dirOnly && !isDir {
			continue
		}

		matched := false

		// Adjust pattern for OS
		pattern := filepath.FromSlash(rule.pattern)

		if rule.rooted {
			// Match against relPath
			if matchedPath, err := filepath.Match(pattern, relPath); err != nil {
				log.Printf("warn: malformed gitignore pattern '%s': %v", rule.pattern, err)
			} else if matchedPath {
				matched = true
			}
		} else {
			// Match against name (basename)
			if matchedName, err := filepath.Match(pattern, name); err != nil {
				log.Printf("warn: malformed gitignore pattern '%s': %v", rule.pattern, err)
			} else if matchedName {
				matched = true
			}
		}

		if matched {
			return !rule.negate
		}
	}
	return false
}

package indexer

import (
	"bufio"
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
		// Negation (!) is not supported in this simple version
		if strings.HasPrefix(line, "!") {
			continue
		}

		rule := ignoreRule{
			pattern: line,
		}

		if strings.HasSuffix(line, "/") {
			rule.dirOnly = true
			rule.pattern = strings.TrimSuffix(line, "/")
		}

		// If it contains a slash (and it's not just at the end which we removed), it's rooted
		if strings.Contains(rule.pattern, "/") {
			rule.rooted = true
			// Ensure it starts with / if we want to match from root?
			// Gitignore says: "If the pattern ends with a slash, it is removed for the purpose of the following description, but it would only find a match with a directory. In other words, foo/ will match a directory foo and paths underneath it, but will not match a regular file or a symbolic link foo (this is consistent with the way how pathspec works in general in git)."
			// "If the pattern does not contain a slash /, git treats it as a shell glob pattern and checks for a match against the pathname relative to the location of the .gitignore file (relative to the toplevel of the work tree if not from a .gitignore file)."

			// So rooted means we match against path relative to root.
			// Non-rooted means we match against basename.

			// We handle leading slash
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

	// On Windows, Rel returns backslashes. Standardize to forward slashes for matching?
	// Git uses forward slashes. Go's filepath.Match uses OS separator.
	// For simplicity, let's keep OS separator but assume .gitignore uses forward slashes.
	// We might need to convert rule pattern to OS separator.

	// Let's stick to using filepath.Match which handles OS separator on Windows if pattern has it.
	// But .gitignore usually has forward slashes.
	// I'll replace / with filepath.Separator in rule.pattern

	name := filepath.Base(path)

	for _, rule := range m.rules {
		if rule.dirOnly && !isDir {
			continue
		}

		matched := false

		// Adjust pattern for OS
		pattern := filepath.FromSlash(rule.pattern)

		if rule.rooted {
			// Match against relPath
			// Example: ignored_dir/nested -> rule "ignored_dir"
			// relPath is "ignored_dir" (if we are checking that dir)

			// Note: filepath.Match does not handle recursive matching like "dir/**"
			// But simple exact match or "dir/*" works.

			// If rule is "ignored_dir", and relPath is "ignored_dir", it matches.
			// If relPath is "ignored_dir/sub", we usually want to ignore it too if "ignored_dir" was a dir-only rule or just a dir.
			// But ShouldIgnore is called for every file.
			// If we return true for "ignored_dir", walker won't enter it.
			// So we mainly need to match the directory itself.

			// If we are checking "ignored_dir/file", and rule is "ignored_dir",
			// we are likely already inside it, so we wouldn't reach here if we ignored parent.
			// BUT, walker calls increment() which logs progress.
			// walker logic:
			// if ignoredDirs[name] continue.

			// So we only check the current node.

			if matchedPath, _ := filepath.Match(pattern, relPath); matchedPath {
				matched = true
			}
		} else {
			// Match against name (basename)
			if matchedName, _ := filepath.Match(pattern, name); matchedName {
				matched = true
			}
		}

		if matched {
			return true
		}
	}
	return false
}

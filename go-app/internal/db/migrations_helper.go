package db

import (
	"path/filepath"
	"sort"
	"strings"
)

// ComputePendingMigrations takes a list of filenames from a migrations directory
// and a list of already-applied versions, and returns the pending migration
// filenames in lexical order. Only files with a .sql suffix (case-insensitive)
// are considered. Comparison for applied versions uses the base filename.
func ComputePendingMigrations(fileNames []string, applied []string) []string {
	// Normalize applied to a set keyed by base filename
	appliedSet := make(map[string]struct{}, len(applied))
	for _, a := range applied {
		appliedSet[filepath.Base(a)] = struct{}{}
	}
	var sqlFiles []string
	for _, name := range fileNames {
		base := filepath.Base(name)
		if strings.HasSuffix(strings.ToLower(base), ".sql") {
			sqlFiles = append(sqlFiles, base)
		}
	}
	sort.Strings(sqlFiles)
	var pending []string
	for _, f := range sqlFiles {
		if _, ok := appliedSet[f]; ok {
			continue
		}
		pending = append(pending, f)
	}
	return pending
}

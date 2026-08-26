// Package dataset owns the directory Cricsheet match data lives in: where it is, and
// what is actually there.
//
// Before this package the location was a defaulted string in a handler ("../data"),
// a different default in config ("../data/go-app/cricsheet"), and an env var only the
// CLI read. Since cricsheet.ImportDir does not recurse, an import started from the ops
// console read the wrong directory, found no files, and completed successfully — the
// silent-success failure this codebase has been bitten by before. One resolver, used
// by every caller, is the fix.
package dataset

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/config"
)

// DirEnvVar overrides the configured dataset directory. Named here so the value and
// the message that tells an operator about it cannot disagree.
const DirEnvVar = "GO_APP_CRICSHEET_DIR"

// MatchFileExt is the extension of an importable Cricsheet match file.
const MatchFileExt = ".json"

// Dir returns the dataset directory: the environment override when set, then
// inputs.cricsheet_dir from config, then the built-in default.
func Dir() string {
	if p := strings.TrimSpace(os.Getenv(DirEnvVar)); p != "" {
		return p
	}
	return config.DefaultCricsheetDir()
}

// Inventory is what is actually on disk at a dataset directory. It answers the
// question the ops console exists to answer: is there any data on this box?
type Inventory struct {
	// Path is the directory inspected.
	Path string `json:"path"`
	// Exists is false when the directory is missing or unreadable.
	Exists bool `json:"exists"`
	// Readable is false when the directory exists but could not be listed.
	Readable bool `json:"readable"`
	// MatchFiles counts importable .json files directly in the directory. Import does
	// not recurse, so neither does this — a count that included nested files would
	// promise data the importer will not read.
	MatchFiles int `json:"match_files"`
	// Bytes is the total size of those files.
	Bytes int64 `json:"bytes"`
	// NewestFile and NewestModified describe the most recently modified match file.
	NewestFile     string `json:"newest_file,omitempty"`
	NewestModified string `json:"newest_modified,omitempty"`
	// Error explains why the directory could not be read, when it could not.
	Error string `json:"error,omitempty"`
}

// IsEmpty reports whether the directory holds nothing the importer would read.
func (i Inventory) IsEmpty() bool { return i.MatchFiles == 0 }

// Inspect reports what is in the directory.
//
// It returns no error: "the directory is missing" and "the directory is unreadable"
// are both states the ops console needs to display rather than fail on. Callers that
// must not proceed on empty data check IsEmpty.
func Inspect(path string) Inventory {
	inv := Inventory{Path: path}

	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		if err != nil {
			inv.Error = err.Error()
		} else {
			inv.Error = "not a directory"
		}
		return inv
	}
	inv.Exists = true

	entries, err := os.ReadDir(path)
	if err != nil {
		inv.Error = err.Error()
		return inv
	}
	inv.Readable = true

	var newest time.Time
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), MatchFileExt) {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		inv.MatchFiles++
		inv.Bytes += fi.Size()
		if fi.ModTime().After(newest) {
			newest = fi.ModTime()
			inv.NewestFile = e.Name()
		}
	}
	if !newest.IsZero() {
		inv.NewestModified = newest.UTC().Format(time.RFC3339)
	}
	return inv
}

// MatchFiles lists the importable files in the directory, sorted, using the same
// rule as Inspect so the count and the list can never disagree.
func MatchFiles(path string) ([]string, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), MatchFileExt) {
			continue
		}
		files = append(files, filepath.Join(path, e.Name()))
	}
	sort.Strings(files)
	return files, nil
}

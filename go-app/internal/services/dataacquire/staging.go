package dataacquire

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ErrNoStagedArchive is returned when the staging directory holds nothing to extract.
var ErrNoStagedArchive = errors.New("no archive in the staging directory")

// StagedArchive is one downloaded archive available to extract, with whatever
// provenance its sidecar recorded.
type StagedArchive struct {
	Filename     string `json:"filename"`
	Bytes        int64  `json:"bytes"`
	Modified     string `json:"modified"`
	SHA256       string `json:"sha256,omitempty"`
	SourceURL    string `json:"source_url,omitempty"`
	FeedID       string `json:"feed_id,omitempty"`
	FetchedAt    string `json:"fetched_at,omitempty"`
	LastModified string `json:"last_modified,omitempty"`
}

// ListStaged returns the archives in the staging directory, newest first. A missing
// directory is an empty list, not an error: "nothing has been fetched yet" is a state
// the console displays rather than fails on.
func ListStaged(stagingDir string) []StagedArchive {
	entries, err := os.ReadDir(stagingDir)
	if err != nil {
		return nil
	}

	out := make([]StagedArchive, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ArchiveExt) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		archive := StagedArchive{
			Filename: e.Name(),
			Bytes:    info.Size(),
			Modified: info.ModTime().UTC().Format(time.RFC3339),
		}
		if meta := readSidecar(filepath.Join(stagingDir, e.Name())); meta.SHA256 != "" {
			archive.SHA256 = meta.SHA256
			archive.SourceURL = meta.SourceURL
			archive.FeedID = meta.FeedID
			archive.FetchedAt = meta.FetchedAt
			archive.LastModified = meta.LastModified
		}
		out = append(out, archive)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Modified > out[j].Modified })
	return out
}

// ResolveStagedArchive turns a requested filename into a path inside the staging
// directory, defaulting to the newest archive when none is named.
//
// The filename comes from a request body, so it is checked here rather than trusted:
// a caller asking to extract "../../etc/shadow" must be refused before anything opens
// it. This is the same class of check ArchiveFilename does for a URL's last segment.
func ResolveStagedArchive(stagingDir, filename string) (string, error) {
	filename = strings.TrimSpace(filename)

	if filename == "" {
		staged := ListStaged(stagingDir)
		if len(staged) == 0 {
			return "", fmt.Errorf("%w: %s", ErrNoStagedArchive, stagingDir)
		}
		return filepath.Join(stagingDir, staged[0].Filename), nil
	}

	if filename != filepath.Base(filename) || strings.ContainsAny(filename, `/\`) ||
		strings.Contains(filename, "..") {
		return "", fmt.Errorf("%w: %q is not a plain filename", ErrUnsafeFilename, filename)
	}
	if !strings.EqualFold(filepath.Ext(filename), ArchiveExt) {
		return "", fmt.Errorf("%w: %q is not a %s archive", ErrUnsafeFilename, filename, ArchiveExt)
	}

	path := filepath.Join(stagingDir, filename)
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrNoStagedArchive, filename)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%w: %q is not a regular file", ErrUnsafeFilename, filename)
	}
	return path, nil
}

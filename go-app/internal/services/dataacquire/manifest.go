package dataacquire

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
)

// ManifestName is the file an extraction leaves in the dataset directory recording
// what produced it: entry count, bytes, the source archive's digest and URL, and when.
//
// It is the answer to "which data is this?" that survives a restart and a change of
// operator, and it is what A-3's registry and P-1's provenance stamp read from. The
// name starts with a dot so it is not mistaken for data — and it is not a .json file,
// so the importer's extension rule skips it without needing a special case.
const ManifestName = ".dataset-manifest"

// ManifestPath returns the manifest location for a dataset directory.
func ManifestPath(dir string) string { return filepath.Join(dir, ManifestName) }

// ReadManifest returns the manifest for a dataset directory, or ok=false when there
// is none. A directory populated by hand has no manifest, which is a fact to report
// rather than an error: it means the provenance is genuinely unknown.
func ReadManifest(dir string) (ExtractResult, bool) {
	data, err := os.ReadFile(ManifestPath(dir))
	if err != nil {
		return ExtractResult{}, false
	}
	var r ExtractResult
	if err := json.Unmarshal(data, &r); err != nil {
		slog.Warn("dataset: unreadable manifest",
			slog.String("path", ManifestPath(dir)), slog.Any("err", err))
		return ExtractResult{}, false
	}
	return r, true
}

// writeManifest records the extraction beside the data it produced. A failure to
// write it loses provenance, not data, so it is logged rather than returned — an
// extraction that worked should not be reported as failed because of it.
func writeManifest(dir string, r ExtractResult) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		slog.Warn("dataset extract: encoding manifest failed", slog.Any("err", err))
		return
	}
	if err := os.WriteFile(ManifestPath(dir), append(data, '\n'), 0o600); err != nil {
		slog.Warn("dataset extract: writing manifest failed",
			slog.String("path", ManifestPath(dir)), slog.Any("err", err))
	}
}

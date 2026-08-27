package exportdataset

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ManifestName is the file an export leaves beside its CSVs, recording what produced
// them.
//
// A CSV on disk says nothing about where its rows came from. Without this, a model
// trained from these files carries no way back to the dataset behind them, and
// "which data produced this model?" stays an archaeology exercise (ops plan P-1).
//
// It is read by ml-service when training, which copies the provenance into each
// model's sidecar — so the answer travels with the artifact rather than staying in a
// directory someone may later overwrite.
const ManifestName = "export-manifest.json"

// Provenance describes the dataset an export was derived from.
//
// Supplied by the caller rather than read here: which dataset is live is a question
// about the dataset directory, and an exporter that reached into it would be an
// exporter that knows about acquisition. Every field is optional because a directory
// populated by hand genuinely has no answer — and absent is the honest rendering of
// that, which is what P-2 flags.
type Provenance struct {
	DatasetSHA256    string `json:"dataset_sha256,omitempty"`
	DatasetSourceURL string `json:"dataset_source_url,omitempty"`
	DatasetFeed      string `json:"dataset_feed,omitempty"`
	DatasetExtracted string `json:"dataset_extracted_at,omitempty"`
	DatasetMatchFile int    `json:"dataset_match_files,omitempty"`
}

// Known reports whether anything is known about the dataset behind an export.
func (p Provenance) Known() bool { return p.DatasetSHA256 != "" }

// ExportedFile is one CSV an export produced.
type ExportedFile struct {
	Name  string `json:"name"`
	Bytes int64  `json:"bytes"`
}

// Manifest is what an export leaves behind about itself.
type Manifest struct {
	Version    int            `json:"v"`
	ExportedAt string         `json:"exported_at"`
	Formats    []string       `json:"formats,omitempty"`
	Unified    bool           `json:"unified"`
	Files      []ExportedFile `json:"files,omitempty"`
	Provenance Provenance     `json:"provenance"`
}

// ManifestVersion is bumped on a breaking change to the manifest shape, so a reader
// can tell what it is holding rather than guessing from which keys are present.
const ManifestVersion = 1

// ManifestPath returns the manifest location for an export directory.
func ManifestPath(dir string) string { return filepath.Join(dir, ManifestName) }

// ReadManifest returns the manifest for an export directory, or ok=false when there
// is none.
func ReadManifest(dir string) (Manifest, bool) {
	data, err := os.ReadFile(ManifestPath(dir))
	if err != nil {
		return Manifest{}, false
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		slog.Warn("exportdataset: unreadable manifest", slog.String("path", ManifestPath(dir)), slog.Any("err", err))
		return Manifest{}, false
	}
	return m, true
}

// writeManifest records what an export produced.
//
// A failure here loses provenance, not data, so it is logged rather than returned: an
// export whose CSVs are all present should not be reported as failed because a
// descriptive file could not be written.
func writeManifest(dir string, formats []string, unified bool, provenance Provenance) {
	manifest := Manifest{
		Version:    ManifestVersion,
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
		Formats:    formats,
		Unified:    unified,
		Files:      listExportedCSVs(dir),
		Provenance: provenance,
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		slog.Warn("exportdataset: encoding manifest failed", slog.Any("err", err))
		return
	}
	if err := os.WriteFile(ManifestPath(dir), append(data, '\n'), 0o600); err != nil {
		slog.Warn("exportdataset: writing manifest failed",
			slog.String("path", ManifestPath(dir)), slog.Any("err", err))
		return
	}
	slog.Info("pipeline: export-dataset wrote manifest",
		slog.String("path", ManifestPath(dir)),
		slog.Int("files", len(manifest.Files)),
		slog.Bool("dataset_known", provenance.Known()))
}

// listExportedCSVs records the CSVs actually written, with their sizes.
//
// Sizes because an empty CSV is a failed export that reported success — the shape of
// failure this repo keeps meeting — and a manifest that lists a file without saying it
// is 0 bytes would hide exactly that.
func listExportedCSVs(dir string) []ExportedFile {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	files := make([]ExportedFile, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".csv") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, ExportedFile{Name: e.Name(), Bytes: info.Size()})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
	return files
}

// describeManifest renders a manifest for logs.
func describeManifest(m Manifest) string {
	if !m.Provenance.Known() {
		return "dataset unknown"
	}
	return fmt.Sprintf("dataset %s", m.Provenance.DatasetSHA256)
}

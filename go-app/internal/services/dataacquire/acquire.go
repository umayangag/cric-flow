package dataacquire

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Acquisition says which of fetch and extract still have work to do before the
// dataset directory holds data from a given source.
//
// It exists so Import can be one action (consumer plan W6): download, extract and
// load. The alternative was to make Import always download, which turns a repeat
// import of unchanged data into a several-hundred-megabyte transfer, or to make it
// never download, which is the three-step dance across two tabs that W6 removes.
//
// A skipped step carries its reason. A step that renders as "done" without having run
// is the silent-success failure this codebase has met three times; "skipped, because
// the directory already holds this archive" is a different statement and reads as one.
type Acquisition struct {
	// SkipFetch is why the download is unnecessary, or "" when it must run.
	SkipFetch string
	// SkipExtract is why the extraction is unnecessary, or "" when it must run.
	SkipExtract string
}

// PlanAcquisition decides what Import must do first.
//
// The question it answers is deliberately narrow: *does the dataset directory already
// hold data from this source?* It does not ask whether the upstream archive has
// changed — that would need a conditional request on every import, and Cricsheet
// republishes under the same URL often enough that the answer would usually be "yes"
// anyway. `refresh` is the explicit way to say "download it again", and it is the
// honest one: re-fetching is a decision, not something inferred from a header.
func PlanAcquisition(sourceURL, stagingDir, dataDir string, matchFiles int, refresh bool) Acquisition {
	if refresh {
		return Acquisition{}
	}

	sourceURL = strings.TrimSpace(sourceURL)

	// The live manifest is the strongest evidence: it says what was extracted into
	// this directory and where it came from. Paired with a non-empty directory —
	// because a manifest beside no match files describes data somebody has since
	// deleted — it means there is nothing to acquire.
	if manifest, ok := ReadManifest(dataDir); ok && matchFiles > 0 &&
		sameSource(manifest.SourceURL, sourceURL) {
		reason := fmt.Sprintf("the dataset directory already holds %s from this source (%d match files)",
			filepath.Base(manifest.ArchivePath), matchFiles)
		return Acquisition{SkipFetch: reason, SkipExtract: reason}
	}

	// Nothing live, but the archive may already be downloaded — a previous import
	// that failed during extraction, or an operator who fetched by hand.
	for _, archive := range ListStaged(stagingDir) {
		if sameSource(archive.SourceURL, sourceURL) {
			return Acquisition{
				SkipFetch: fmt.Sprintf("%s from this source is already staged", archive.Filename),
			}
		}
	}

	return Acquisition{}
}

// sameSource compares two archive URLs. Both must be present to match: an archive
// with no recorded source is one we cannot vouch for, and treating "unknown" as "the
// configured one" would skip a download on no evidence at all.
func sameSource(recorded, configured string) bool {
	recorded = strings.TrimSpace(recorded)
	return recorded != "" && configured != "" && strings.EqualFold(recorded, configured)
}

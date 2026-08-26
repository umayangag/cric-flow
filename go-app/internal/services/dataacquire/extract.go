package dataacquire

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/services/dataset"
)

// Sentinel errors for a refused archive. Each names a property the extractor checks
// itself rather than trusting archive/zip to check on its behalf — it does not.
var (
	// ErrUnsafeEntry is returned for an entry whose path escapes the destination, is
	// absolute, or is anything other than a regular file or directory.
	ErrUnsafeEntry = errors.New("archive contains an unsafe entry")

	// ErrArchiveTooLarge is returned when the archive's uncompressed size or entry
	// count exceeds the caps. A zip bomb must be refused, not survived.
	ErrArchiveTooLarge = errors.New("archive exceeds the extraction caps")

	// ErrNoMatchFilesInArchive is returned when an archive extracts without producing
	// a single importable match file. An extract that "succeeds" into an empty
	// dataset is the silent-success failure this repo has been bitten by repeatedly.
	ErrNoMatchFilesInArchive = errors.New("archive contains no match files")
)

// Extraction caps. Cricsheet's largest archive expands to a few GiB across some tens
// of thousands of files, so these are generous — but finite, which is the point.
const (
	DefaultMaxUncompressedBytes int64 = 16 << 30 // 16 GiB
	DefaultMaxEntries           int   = 500_000
	// DefaultMaxCompressionRatio bounds a single entry's expansion. A file that
	// inflates a thousandfold is a bomb, not a dataset.
	DefaultMaxCompressionRatio int64 = 1000
)

// ExtractOptions controls one extraction.
type ExtractOptions struct {
	// ArchivePath is the staged .zip to read.
	ArchivePath string
	// DestDir is the live dataset directory the match files end up in.
	DestDir string
	// WorkDir is where the archive is inflated before anything enters DestDir.
	// It must be on the same filesystem as DestDir so the move is a rename.
	WorkDir string
	// MaxUncompressedBytes, MaxEntries and MaxCompressionRatio cap the archive.
	// Zero means the corresponding Default.
	MaxUncompressedBytes int64
	MaxEntries           int
	MaxCompressionRatio  int64
	// Progress, when set, is called as entries are written. It must not block.
	Progress func(ExtractProgress)
	// FreeSpace reports free bytes on the filesystem holding a path; a field so the
	// precheck is testable without filling a disk.
	FreeSpace func(path string) (int64, error)
}

// ExtractProgress is one live sample of an extraction.
type ExtractProgress struct {
	// Entries written so far, and the archive's declared total.
	Entries      int   `json:"entries"`
	EntriesTotal int   `json:"entries_total,omitempty"`
	Bytes        int64 `json:"bytes"`
	// ETASec is seconds remaining at the current rate, or nil when unknowable.
	ETASec *int64 `json:"eta_sec,omitempty"`
}

// ExtractResult is the manifest of a completed extraction.
type ExtractResult struct {
	ArchivePath   string `json:"archive_path"`
	ArchiveSHA256 string `json:"archive_sha256,omitempty"`
	SourceURL     string `json:"source_url,omitempty"`
	FeedID        string `json:"feed_id,omitempty"`
	DestDir       string `json:"dest_dir"`
	// Entries is every regular file written; MatchFiles is the subset the importer
	// will actually read. They differ when an archive carries a README or a
	// checksum file, and the difference is worth showing rather than hiding.
	Entries    int   `json:"entries"`
	MatchFiles int   `json:"match_files"`
	Bytes      int64 `json:"bytes"`
	// ReplacedInto names where the previous contents of DestDir were moved, or "" when
	// the directory was empty. Keeping them makes a mistaken replace recoverable.
	ReplacedInto string `json:"replaced_into,omitempty"`
	ExtractedAt  string `json:"extracted_at"`
}

// Extract inflates a staged archive and replaces the live dataset directory with it.
//
// Nothing enters DestDir until the whole archive has been inflated and checked. That
// ordering is the guarantee: the risky part — reading paths and inflating bytes from
// an archive we did not create — happens entirely inside WorkDir, so an archive that
// is malicious, truncated or simply wrong fails with the live directory untouched.
// The final step is same-filesystem renames of already-verified files.
func Extract(ctx context.Context, opts ExtractOptions) (ExtractResult, error) {
	opts = opts.withDefaults()

	reader, err := zip.OpenReader(opts.ArchivePath)
	if err != nil {
		return ExtractResult{}, fmt.Errorf("open archive %s: %w", opts.ArchivePath, err)
	}
	defer func() { _ = reader.Close() }()

	if err := opts.checkCaps(reader); err != nil {
		return ExtractResult{}, err
	}

	staged, err := os.MkdirTemp(opts.WorkDir, "extract-*")
	if err != nil {
		return ExtractResult{}, fmt.Errorf("create extraction workspace: %w", err)
	}
	// Cleared unconditionally: on the success path the contents have already been
	// moved out, so this removes an empty tree.
	defer func() { _ = os.RemoveAll(staged) }()

	written, bytes, err := opts.inflate(ctx, reader, staged)
	if err != nil {
		return ExtractResult{}, err
	}

	matchFiles, err := dataset.MatchFiles(staged)
	if err != nil {
		return ExtractResult{}, fmt.Errorf("inspect extracted files: %w", err)
	}
	if len(matchFiles) == 0 {
		return ExtractResult{}, fmt.Errorf("%w: %d entries extracted, none of them %s files",
			ErrNoMatchFilesInArchive, written, dataset.MatchFileExt)
	}

	replacedInto, err := swapIn(staged, opts.DestDir, opts.WorkDir)
	if err != nil {
		return ExtractResult{}, err
	}

	result := ExtractResult{
		ArchivePath:  opts.ArchivePath,
		DestDir:      opts.DestDir,
		Entries:      written,
		MatchFiles:   len(matchFiles),
		Bytes:        bytes,
		ReplacedInto: replacedInto,
		ExtractedAt:  time.Now().UTC().Format(time.RFC3339),
	}
	// Provenance carries across from the fetch that staged the archive, so
	// "which data produced this model?" stays answerable (A-3, P-1).
	if prev := readSidecar(opts.ArchivePath); prev.SHA256 != "" {
		result.ArchiveSHA256 = prev.SHA256
		result.SourceURL = prev.SourceURL
		result.FeedID = prev.FeedID
	}
	writeManifest(opts.DestDir, result)

	slog.Info("dataset extract complete",
		slog.String("archive", opts.ArchivePath),
		slog.String("dest", opts.DestDir),
		slog.Int("entries", written),
		slog.Int("match_files", len(matchFiles)),
		slog.Int64("bytes", bytes))
	return result, nil
}

// checkCaps refuses a bomb before a single byte is inflated. The zip central
// directory declares each entry's uncompressed size, so the total is knowable up
// front — and an entry whose declared expansion ratio is absurd is refused on that
// alone, without trusting the declaration to be honest (inflate re-checks).
func (o ExtractOptions) checkCaps(reader *zip.ReadCloser) error {
	if len(reader.File) > o.MaxEntries {
		return fmt.Errorf("%w: %d entries, cap is %d", ErrArchiveTooLarge, len(reader.File), o.MaxEntries)
	}

	// The declared sizes are uint64 and attacker-controlled, so they are compared as
	// uint64 against the cap and only narrowed to int64 once known to be under it.
	// Converting first would let a declared size above MaxInt64 wrap to a negative
	// and slip past every check below.
	cap64 := asUint64(o.MaxUncompressedBytes)
	var total uint64
	for _, f := range reader.File {
		if f.UncompressedSize64 > cap64 {
			return fmt.Errorf("%w: entry %q declares %d bytes, cap is %d",
				ErrArchiveTooLarge, f.Name, f.UncompressedSize64, o.MaxUncompressedBytes)
		}
		if f.CompressedSize64 > 0 {
			ratio := f.UncompressedSize64 / f.CompressedSize64
			if ratio > asUint64(o.MaxCompressionRatio) {
				return fmt.Errorf("%w: entry %q expands %dx, cap is %dx",
					ErrArchiveTooLarge, f.Name, ratio, o.MaxCompressionRatio)
			}
		}
		total += f.UncompressedSize64
		if total > cap64 {
			return fmt.Errorf("%w: declared total %d bytes, cap is %d",
				ErrArchiveTooLarge, total, o.MaxUncompressedBytes)
		}
	}

	// total is now known to be <= MaxUncompressedBytes, so the narrowing is safe.
	expanded := int64(total)
	if free, err := o.FreeSpace(o.WorkDir); err == nil && free < expanded {
		return fmt.Errorf("%w: %s has %d bytes free, archive expands to %d",
			ErrInsufficientSpace, o.WorkDir, free, expanded)
	}
	return nil
}

// asUint64 converts a cap for comparison against an archive's declared (unsigned)
// sizes. withDefaults has already replaced any value at or below zero, so the guard
// is unreachable — but clamping rather than wrapping means the failure mode of a
// negative cap would be "refuse everything", not "allow everything".
func asUint64(v int64) uint64 {
	if v < 0 {
		return 0
	}
	return uint64(v)
}

// inflate writes every entry into root, enforcing the caps as it goes.
func (o ExtractOptions) inflate(ctx context.Context, reader *zip.ReadCloser, root string) (int, int64, error) {
	var entries int
	var bytes int64
	started := time.Now()
	lastReport := time.Time{}

	for _, f := range reader.File {
		if err := ctx.Err(); err != nil {
			return 0, 0, err
		}

		target, isDir, err := safeEntryPath(root, f)
		if err != nil {
			return 0, 0, err
		}
		if isDir {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return 0, 0, fmt.Errorf("create directory %s: %w", f.Name, err)
			}
			continue
		}

		n, err := o.writeEntry(f, target, o.MaxUncompressedBytes-bytes)
		if err != nil {
			return 0, 0, err
		}
		entries++
		bytes += n

		if o.Progress != nil && time.Since(lastReport) >= progressInterval {
			lastReport = time.Now()
			o.Progress(extractSample(entries, len(reader.File), bytes, started))
		}
	}

	if o.Progress != nil {
		o.Progress(extractSample(entries, len(reader.File), bytes, started))
	}
	return entries, bytes, nil
}

// writeEntry inflates one file, refusing to exceed the remaining byte budget.
//
// The budget is enforced here as well as in checkCaps because the central directory's
// declared sizes are attacker-controlled. A bomb that understates itself passes the
// up-front check and is caught here, mid-inflate, with nothing outside WorkDir written.
func (o ExtractOptions) writeEntry(f *zip.File, target string, budget int64) (int64, error) {
	if budget <= 0 {
		return 0, fmt.Errorf("%w: total uncompressed size exceeded at entry %q", ErrArchiveTooLarge, f.Name)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return 0, fmt.Errorf("create directory for %s: %w", f.Name, err)
	}

	src, err := f.Open()
	if err != nil {
		return 0, fmt.Errorf("read entry %s: %w", f.Name, err)
	}
	defer func() { _ = src.Close() }()

	dst, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return 0, fmt.Errorf("create %s: %w", f.Name, err)
	}
	defer func() { _ = dst.Close() }()

	// One byte over the budget so an overrun is detectable rather than truncated to it.
	n, err := io.Copy(dst, io.LimitReader(src, budget+1))
	if err != nil {
		return 0, fmt.Errorf("extract %s: %w", f.Name, err)
	}
	if n > budget {
		return 0, fmt.Errorf("%w: entry %q inflated past the total cap", ErrArchiveTooLarge, f.Name)
	}
	if err := dst.Close(); err != nil {
		return 0, fmt.Errorf("close %s: %w", f.Name, err)
	}
	return n, nil
}

// safeEntryPath resolves an archive entry to a path inside root, or refuses it.
//
// This is the zip-slip defence, and it is written out rather than delegated because
// archive/zip does not do it for you: f.Name is whatever the archive author wrote,
// including "../../etc/cron.d/x", "/etc/passwd" and "C:\\Windows\\..." — and Go's
// stdlib will happily hand you that string. Every refusal below corresponds to a
// crafted fixture in extract_test.go.
func safeEntryPath(root string, f *zip.File) (path string, isDir bool, err error) {
	name := f.Name

	// Only regular files and directories. A symlink entry is how an archive escapes
	// a path check that only looked at names: the link passes, and the *next* entry
	// written "through" it lands wherever the link points.
	mode := f.Mode()
	switch {
	case mode.IsDir():
		isDir = true
	case mode.IsRegular():
	default:
		return "", false, fmt.Errorf("%w: %q is not a regular file or directory (mode %s)",
			ErrUnsafeEntry, name, mode)
	}

	// Backslashes are separators on the systems that write them, so normalise before
	// checking rather than after — otherwise "..\\..\\x" reads as one harmless name.
	name = strings.ReplaceAll(name, `\`, "/")
	if name == "" {
		return "", false, fmt.Errorf("%w: empty entry name", ErrUnsafeEntry)
	}
	if strings.HasPrefix(name, "/") || filepath.IsAbs(name) || volumeName(name) != "" {
		return "", false, fmt.Errorf("%w: %q is an absolute path", ErrUnsafeEntry, f.Name)
	}
	for _, segment := range strings.Split(name, "/") {
		if segment == ".." {
			return "", false, fmt.Errorf("%w: %q contains a parent-directory segment", ErrUnsafeEntry, f.Name)
		}
	}

	// Belt and braces: even with the segment check above, the resolved path must sit
	// inside root. Checked with a separator so "/data/cricsheet-evil" cannot pass as
	// a prefix match of "/data/cricsheet".
	cleanRoot := filepath.Clean(root)
	resolved := filepath.Clean(filepath.Join(cleanRoot, filepath.FromSlash(name)))
	if resolved != cleanRoot && !strings.HasPrefix(resolved, cleanRoot+string(os.PathSeparator)) {
		return "", false, fmt.Errorf("%w: %q escapes the destination directory", ErrUnsafeEntry, f.Name)
	}
	return resolved, isDir, nil
}

// volumeName reports a Windows drive or UNC prefix. filepath.IsAbs does not see one
// on a unix host, so an archive written on Windows would otherwise slip through.
func volumeName(name string) string {
	if len(name) >= 2 && name[1] == ':' {
		return name[:2]
	}
	if strings.HasPrefix(name, "//") {
		return "//"
	}
	return ""
}

// swapIn replaces the contents of dest with the contents of staged, moving whatever
// was there into a timestamped directory under workDir.
//
// It is deliberately not a single directory rename. The staging directory lives
// inside the dataset directory, so renaming the dataset directory away would take the
// archives with it. Moving entries individually is the cost of that; it is paid after
// every byte has been inflated and checked, so what remains is same-filesystem
// renames of files already known to be good.
func swapIn(staged, dest, workDir string) (replacedInto string, err error) {
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return "", fmt.Errorf("create dataset directory %s: %w", dest, err)
	}

	previous, err := displaceExisting(dest, workDir)
	if err != nil {
		return "", err
	}

	entries, err := os.ReadDir(staged)
	if err != nil {
		return "", fmt.Errorf("read extracted files: %w", err)
	}
	for _, e := range entries {
		from := filepath.Join(staged, e.Name())
		to := filepath.Join(dest, e.Name())
		if err := os.Rename(from, to); err != nil {
			return "", fmt.Errorf("move %s into the dataset directory: %w", e.Name(), err)
		}
	}
	return previous, nil
}

// displaceExisting moves the current dataset contents aside, skipping the staging
// directory itself. Returns "" when there was nothing to move.
func displaceExisting(dest, workDir string) (string, error) {
	entries, err := os.ReadDir(dest)
	if err != nil {
		return "", fmt.Errorf("read dataset directory %s: %w", dest, err)
	}

	movable := make([]os.DirEntry, 0, len(entries))
	for _, e := range entries {
		// The staging directory holds the archive being extracted right now.
		if e.IsDir() && e.Name() == dataset.StagingDirName {
			continue
		}
		movable = append(movable, e)
	}
	if len(movable) == 0 {
		return "", nil
	}

	previous := filepath.Join(workDir, "previous-"+time.Now().UTC().Format("20060102T150405Z"))
	if err := os.MkdirAll(previous, 0o755); err != nil {
		return "", fmt.Errorf("create directory for the replaced dataset: %w", err)
	}
	for _, e := range movable {
		if err := os.Rename(filepath.Join(dest, e.Name()), filepath.Join(previous, e.Name())); err != nil {
			return "", fmt.Errorf("move aside the existing %s: %w", e.Name(), err)
		}
	}
	slog.Info("dataset extract: previous contents kept",
		slog.String("path", previous), slog.Int("entries", len(movable)))
	return previous, nil
}

// extractSample builds the progress reading at a moment.
func extractSample(entries, total int, bytes int64, started time.Time) ExtractProgress {
	out := ExtractProgress{Entries: entries, EntriesTotal: total, Bytes: bytes}
	elapsed := time.Since(started).Seconds()
	if elapsed <= 0 || entries == 0 || total <= entries {
		return out
	}
	perEntry := elapsed / float64(entries)
	eta := int64(perEntry * float64(total-entries))
	out.ETASec = &eta
	return out
}

// withDefaults fills the zero values so callers only set what they care about.
func (o ExtractOptions) withDefaults() ExtractOptions {
	if o.MaxUncompressedBytes <= 0 {
		o.MaxUncompressedBytes = DefaultMaxUncompressedBytes
	}
	if o.MaxEntries <= 0 {
		o.MaxEntries = DefaultMaxEntries
	}
	if o.MaxCompressionRatio <= 0 {
		o.MaxCompressionRatio = DefaultMaxCompressionRatio
	}
	if o.FreeSpace == nil {
		o.FreeSpace = FreeSpace
	}
	return o
}

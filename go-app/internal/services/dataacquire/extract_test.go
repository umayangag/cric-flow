package dataacquire

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-flow/go-app/internal/services/dataset"
)

// zipEntry describes one crafted archive member.
type zipEntry struct {
	name string
	body string
	mode os.FileMode
	// rawSize overrides the declared uncompressed size, for the case where the
	// central directory lies about how much a member expands to.
	dir bool
}

// writeArchive builds a zip at path. It writes headers directly rather than using
// zip.Writer's Create helper, because the whole point of these tests is to produce
// entries a well-behaved writer would never emit.
func writeArchive(t *testing.T, path string, entries []zipEntry) string {
	t.Helper()
	buf := &bytes.Buffer{}
	zw := zip.NewWriter(buf)
	for _, e := range entries {
		hdr := &zip.FileHeader{Name: e.name, Method: zip.Deflate}
		if e.mode != 0 {
			hdr.SetMode(e.mode)
		}
		if e.dir && !strings.HasSuffix(hdr.Name, "/") {
			hdr.Name += "/"
		}
		w, err := zw.CreateHeader(hdr)
		require.NoError(t, err)
		if !e.dir {
			_, err = w.Write([]byte(e.body))
			require.NoError(t, err)
		}
	}
	require.NoError(t, zw.Close())
	require.NoError(t, os.WriteFile(path, buf.Bytes(), 0o600))
	return path
}

// extractFixture lays out a dataset directory with its staging subdirectory and
// returns options pointing at a crafted archive.
func extractFixture(t *testing.T, entries []zipEntry) ExtractOptions {
	t.Helper()
	destDir := t.TempDir()
	workDir := filepath.Join(destDir, dataset.StagingDirName)
	require.NoError(t, os.MkdirAll(workDir, 0o755))
	archive := writeArchive(t, filepath.Join(workDir, "all_json.zip"), entries)
	return ExtractOptions{
		ArchivePath: archive,
		DestDir:     destDir,
		WorkDir:     workDir,
		FreeSpace:   func(string) (int64, error) { return 1 << 40, nil },
	}
}

func TestExtract_WritesMatchFilesAndAManifest(t *testing.T) {
	t.Parallel()
	opts := extractFixture(t, []zipEntry{
		{name: "1234.json", body: `{"info":{}}`},
		{name: "5678.json", body: `{"info":{}}`},
		{name: "README.txt", body: "not match data"},
	})

	result, err := Extract(context.Background(), opts)
	require.NoError(t, err)
	assert.Equal(t, 3, result.Entries)
	assert.Equal(t, 2, result.MatchFiles, "only the .json files are importable")

	inv := dataset.Inspect(opts.DestDir)
	assert.Equal(t, 2, inv.MatchFiles)

	manifest, ok := ReadManifest(opts.DestDir)
	require.True(t, ok, "an extraction must leave provenance behind")
	assert.Equal(t, 2, manifest.MatchFiles)
	assert.NotEmpty(t, manifest.ExtractedAt)
}

// TestExtract_RefusesZipSlip is the highest-severity item in the ops plan. Each entry
// here is one archive/zip will hand over without complaint: the stdlib does not
// sanitise f.Name, so every refusal below has to be ours.
func TestExtract_RefusesZipSlip(t *testing.T) {
	t.Parallel()
	cases := map[string]zipEntry{
		"parent traversal":         {name: "../escaped.json", body: "{}"},
		"deep traversal":           {name: "a/b/../../../escaped.json", body: "{}"},
		"absolute path":            {name: "/etc/passwd", body: "pwned"},
		"windows separators":       {name: `..\..\escaped.json`, body: "{}"},
		"windows drive":            {name: `C:\Windows\evil.json`, body: "{}"},
		"unc path":                 {name: `//host/share/evil.json`, body: "{}"},
		"traversal inside subdir":  {name: "matches/../../escaped.json", body: "{}"},
		"sibling prefix collision": {name: "../" + filepath.Base(t.TempDir()) + "-evil/x.json", body: "{}"},
	}

	for name, bad := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// A good entry alongside the bad one: a partial extract must not happen
			// either, so nothing may reach the dataset directory.
			opts := extractFixture(t, []zipEntry{{name: "1234.json", body: "{}"}, bad})

			_, err := Extract(context.Background(), opts)
			require.ErrorIs(t, err, ErrUnsafeEntry, "%q must be refused", bad.name)

			inv := dataset.Inspect(opts.DestDir)
			assert.Zero(t, inv.MatchFiles, "a refused archive must leave the dataset directory untouched")
		})
	}
}

// TestExtract_RefusesSymlinkEntries covers the escape that a name check alone misses:
// the link entry itself has a perfectly innocent name, and it is the *next* write
// through it that lands outside the destination.
func TestExtract_RefusesSymlinkEntries(t *testing.T) {
	t.Parallel()
	outside := t.TempDir()
	opts := extractFixture(t, []zipEntry{
		{name: "1234.json", body: "{}"},
		{name: "link", body: outside, mode: os.ModeSymlink | 0o777},
		{name: "link/escaped.json", body: "{}"},
	})

	_, err := Extract(context.Background(), opts)
	require.ErrorIs(t, err, ErrUnsafeEntry)

	entries, readErr := os.ReadDir(outside)
	require.NoError(t, readErr)
	assert.Empty(t, entries, "nothing may be written through a symlink entry")
}

func TestExtract_RefusesIrregularEntries(t *testing.T) {
	t.Parallel()
	for name, mode := range map[string]os.FileMode{
		"device": os.ModeDevice | 0o666,
		"fifo":   os.ModeNamedPipe | 0o666,
		"socket": os.ModeSocket | 0o666,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			opts := extractFixture(t, []zipEntry{{name: "odd", body: "", mode: mode}})
			_, err := Extract(context.Background(), opts)
			require.ErrorIs(t, err, ErrUnsafeEntry)
		})
	}
}

// TestExtract_RefusesAZipBomb: a highly compressible member must be refused on its
// expansion ratio rather than inflated and then noticed.
func TestExtract_RefusesAZipBomb(t *testing.T) {
	t.Parallel()
	opts := extractFixture(t, []zipEntry{
		{name: "bomb.json", body: strings.Repeat("A", 4<<20)},
	})
	opts.MaxCompressionRatio = 10

	_, err := Extract(context.Background(), opts)
	require.ErrorIs(t, err, ErrArchiveTooLarge)
	assert.Zero(t, dataset.Inspect(opts.DestDir).MatchFiles)
}

func TestExtract_RefusesTooManyEntries(t *testing.T) {
	t.Parallel()
	entries := make([]zipEntry, 0, 20)
	for i := range 20 {
		entries = append(entries, zipEntry{name: string(rune('a'+i)) + ".json", body: "{}"})
	}
	opts := extractFixture(t, entries)
	opts.MaxEntries = 5

	_, err := Extract(context.Background(), opts)
	require.ErrorIs(t, err, ErrArchiveTooLarge)
}

func TestExtract_RefusesTooManyUncompressedBytes(t *testing.T) {
	t.Parallel()
	opts := extractFixture(t, []zipEntry{
		{name: "1.json", body: strings.Repeat("x", 4096)},
		{name: "2.json", body: strings.Repeat("y", 4096)},
	})
	opts.MaxUncompressedBytes = 1024
	opts.MaxCompressionRatio = 1 << 30 // isolate the byte cap from the ratio cap

	_, err := Extract(context.Background(), opts)
	require.ErrorIs(t, err, ErrArchiveTooLarge)
}

// TestExtract_RefusesAnArchiveWithNoMatchFiles closes the silent-success path: an
// archive that extracts fine but contains nothing importable would otherwise leave a
// wiped dataset directory and a green tick.
func TestExtract_RefusesAnArchiveWithNoMatchFiles(t *testing.T) {
	t.Parallel()
	opts := extractFixture(t, []zipEntry{
		{name: "README.txt", body: "no matches here"},
		{name: "checksums.sha256", body: "deadbeef"},
	})
	require.NoError(t, os.WriteFile(filepath.Join(opts.DestDir, "existing.json"), []byte("{}"), 0o600))

	_, err := Extract(context.Background(), opts)
	require.ErrorIs(t, err, ErrNoMatchFilesInArchive)
	assert.Equal(t, 1, dataset.Inspect(opts.DestDir).MatchFiles,
		"a refused archive must leave the existing dataset in place")
}

// TestExtract_ReplacesAndKeepsThePrevious: the plan calls for a swap, not a merge. A
// stale match file from the last dataset must not survive into the new one, but a
// mistaken replace must be recoverable.
func TestExtract_ReplacesAndKeepsThePrevious(t *testing.T) {
	t.Parallel()
	opts := extractFixture(t, []zipEntry{{name: "new.json", body: "{}"}})
	require.NoError(t, os.WriteFile(filepath.Join(opts.DestDir, "stale.json"), []byte("{}"), 0o600))

	result, err := Extract(context.Background(), opts)
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(opts.DestDir, "stale.json"))
	assert.True(t, os.IsNotExist(err), "the replaced dataset must not leave files behind")
	_, err = os.Stat(filepath.Join(opts.DestDir, "new.json"))
	assert.NoError(t, err)

	require.NotEmpty(t, result.ReplacedInto)
	_, err = os.Stat(filepath.Join(result.ReplacedInto, "stale.json"))
	assert.NoError(t, err, "the previous contents must be recoverable")
}

// TestExtract_KeepsTheStagingDirectory: staging lives inside the dataset directory
// and holds the archive being extracted. A replace that swept it away would delete
// the file it is reading.
func TestExtract_KeepsTheStagingDirectory(t *testing.T) {
	t.Parallel()
	opts := extractFixture(t, []zipEntry{{name: "new.json", body: "{}"}})

	_, err := Extract(context.Background(), opts)
	require.NoError(t, err)

	_, err = os.Stat(opts.ArchivePath)
	assert.NoError(t, err, "the archive must survive its own extraction")
}

func TestExtract_LeavesNoWorkspaceBehind(t *testing.T) {
	t.Parallel()
	opts := extractFixture(t, []zipEntry{{name: "new.json", body: "{}"}})

	_, err := Extract(context.Background(), opts)
	require.NoError(t, err)

	entries, err := os.ReadDir(opts.WorkDir)
	require.NoError(t, err)
	for _, e := range entries {
		assert.False(t, strings.HasPrefix(e.Name(), "extract-"),
			"the extraction workspace %s must be cleaned up", e.Name())
	}
}

func TestExtract_ReportsProgress(t *testing.T) {
	t.Parallel()
	opts := extractFixture(t, []zipEntry{
		{name: "1.json", body: "{}"},
		{name: "2.json", body: "{}"},
		{name: "3.json", body: "{}"},
	})
	var last ExtractProgress
	opts.Progress = func(p ExtractProgress) { last = p }

	_, err := Extract(context.Background(), opts)
	require.NoError(t, err)
	assert.Equal(t, 3, last.Entries)
	assert.Equal(t, 3, last.EntriesTotal)
}

func TestExtract_StopsWhenTheJobContextIsCancelled(t *testing.T) {
	t.Parallel()
	entries := make([]zipEntry, 0, 50)
	for i := range 50 {
		entries = append(entries, zipEntry{name: string(rune('a'+i%26)) + string(rune('a'+i/26)) + ".json", body: "{}"})
	}
	opts := extractFixture(t, entries)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := Extract(ctx, opts)
	require.ErrorIs(t, err, context.Canceled)
	assert.Zero(t, dataset.Inspect(opts.DestDir).MatchFiles)
}

func TestExtract_RefusesWhenTheDiskCannotHoldTheExpansion(t *testing.T) {
	t.Parallel()
	opts := extractFixture(t, []zipEntry{{name: "1.json", body: strings.Repeat("x", 4096)}})
	opts.FreeSpace = func(string) (int64, error) { return 10, nil }

	_, err := Extract(context.Background(), opts)
	require.ErrorIs(t, err, ErrInsufficientSpace)
}

func TestExtract_FailsOnAnUnreadableArchive(t *testing.T) {
	t.Parallel()
	destDir := t.TempDir()
	workDir := filepath.Join(destDir, dataset.StagingDirName)
	require.NoError(t, os.MkdirAll(workDir, 0o755))
	archive := filepath.Join(workDir, "all_json.zip")
	require.NoError(t, os.WriteFile(archive, []byte("this is not a zip"), 0o600))

	_, err := Extract(context.Background(), ExtractOptions{
		ArchivePath: archive,
		DestDir:     destDir,
		WorkDir:     workDir,
		FreeSpace:   func(string) (int64, error) { return 1 << 40, nil },
	})
	require.Error(t, err)
	assert.Zero(t, dataset.Inspect(destDir).MatchFiles)
}

// TestExtract_DigestsAHandPlacedArchive: an archive that reached staging without a
// fetch has no sidecar, so extract is the only chance to learn its digest. Without
// one the dataset registry cannot identify what is now live, which is exactly the
// case P-2 has to flag.
func TestExtract_DigestsAHandPlacedArchive(t *testing.T) {
	t.Parallel()
	opts := extractFixture(t, []zipEntry{{name: "1234.json", body: `{"info":{}}`}})

	result, err := Extract(context.Background(), opts)
	require.NoError(t, err)
	require.NotEmpty(t, result.ArchiveSHA256, "a hand-placed archive must still be identifiable")
	assert.Len(t, result.ArchiveSHA256, 64, "sha256 renders as 64 hex characters")

	raw, err := os.ReadFile(opts.ArchivePath)
	require.NoError(t, err)
	sum := sha256.Sum256(raw)
	assert.Equal(t, hex.EncodeToString(sum[:]), result.ArchiveSHA256)
}

// TestExtract_PrefersTheSidecarDigest: when the fetch already computed the digest as
// the bytes streamed past, extract must reuse it rather than re-reading the file.
func TestExtract_PrefersTheSidecarDigest(t *testing.T) {
	t.Parallel()
	opts := extractFixture(t, []zipEntry{{name: "1234.json", body: "{}"}})
	writeSidecar(opts.ArchivePath, Result{
		SHA256:    "known-from-the-fetch",
		SourceURL: "https://cricsheet.org/downloads/all_json.zip",
		FeedID:    "all",
	})

	result, err := Extract(context.Background(), opts)
	require.NoError(t, err)
	assert.Equal(t, "known-from-the-fetch", result.ArchiveSHA256)
	assert.Equal(t, "all", result.FeedID, "the fetch's provenance must carry into the manifest")
	assert.Contains(t, result.SourceURL, "cricsheet.org")
}

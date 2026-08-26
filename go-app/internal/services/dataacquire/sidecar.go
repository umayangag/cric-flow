package dataacquire

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"os"
)

// sidecarExt is appended to an archive path to hold what we know about it.
const sidecarExt = ".meta.json"

// The sidecar is what makes the conditional request possible: to send
// If-None-Match on a later fetch, the ETag from the earlier one has to survive the
// process. Writing it next to the archive keeps the two together — an archive
// without its provenance is exactly the state A-3 and P-1 exist to prevent.
func sidecarPath(archive string) string { return archive + sidecarExt }

// readSidecar returns what is known about an already-staged archive. A missing or
// unreadable sidecar is not an error: it means the next fetch is unconditional.
func readSidecar(archive string) Result {
	data, err := os.ReadFile(sidecarPath(archive))
	if err != nil {
		return Result{}
	}
	var r Result
	if err := json.Unmarshal(data, &r); err != nil {
		slog.Warn("dataset fetch: unreadable sidecar, refetching unconditionally",
			slog.String("path", sidecarPath(archive)), slog.Any("err", err))
		return Result{}
	}
	// An ETag is only usable while the archive it describes is still there.
	if _, err := os.Stat(archive); err != nil {
		return Result{}
	}
	return r
}

// writeSidecar records the fetch result beside the archive. A failure to write it
// costs a redundant download next time, so it is logged rather than returned.
func writeSidecar(archive string, r Result) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		slog.Warn("dataset fetch: encoding sidecar failed", slog.Any("err", err))
		return
	}
	if err := os.WriteFile(sidecarPath(archive), append(data, '\n'), 0o600); err != nil {
		slog.Warn("dataset fetch: writing sidecar failed",
			slog.String("path", sidecarPath(archive)), slog.Any("err", err))
	}
}

// hashFile returns the hex SHA-256 of a file on disk.
//
// Fetch computes the digest as the bytes stream past, so this is only for an archive
// that arrived some other way. It reads in chunks rather than into memory: these
// files run to hundreds of megabytes.
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

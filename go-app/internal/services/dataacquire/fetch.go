package dataacquire

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Sentinel errors callers distinguish on. Each names a postcondition this package
// verifies rather than assumes — the repo has been bitten twice by steps that
// "succeeded" without doing anything (a 401ing make target that exited 0, an import
// that read the wrong directory and committed nothing).
var (
	// ErrNotModified reports that the server answered 304: the staged archive is
	// already current. It is a success, not a failure, and callers say so.
	ErrNotModified = errors.New("archive already current")

	// ErrTooLarge is returned when the archive exceeds the configured cap, either by
	// its declared Content-Length or by outgrowing it mid-stream.
	ErrTooLarge = errors.New("archive exceeds the size cap")

	// ErrInsufficientSpace is returned when the filesystem cannot hold the archive.
	ErrInsufficientSpace = errors.New("not enough free space for the archive")

	// ErrShortRead is returned when fewer bytes arrived than the server declared.
	// Without this check a truncated download becomes a corrupt dataset that only
	// fails much later, during extraction or import.
	ErrShortRead = errors.New("download ended early")
)

// Options controls one fetch.
type Options struct {
	// StagingDir is where the archive is written. Created if absent.
	StagingDir string
	// MaxBytes is the largest archive accepted. Zero means DefaultMaxBytes.
	MaxBytes int64
	// MinFreeBytes is headroom required beyond the archive itself. Zero means
	// DefaultMinFreeBytes.
	MinFreeBytes int64
	// Client performs the request. Zero value means a client built by newClient.
	Client *http.Client
	// Timeout bounds the whole transfer. Zero means DefaultTimeout.
	Timeout time.Duration
	// Progress, when set, is called as bytes arrive. It must not block.
	Progress func(Progress)
	// FreeSpace reports free bytes on the filesystem holding a path. A field so the
	// precheck is testable without filling a disk.
	FreeSpace func(path string) (int64, error)
}

// Defaults for Options. The size cap is generous relative to Cricsheet's largest
// archive (all_json.zip is well under 1 GiB) but finite: an unbounded download to a
// path the importer reads is a disk-exhaustion primitive.
const (
	DefaultMaxBytes     int64 = 4 << 30 // 4 GiB
	DefaultMinFreeBytes int64 = 1 << 30 // 1 GiB of headroom for the extract that follows
	DefaultTimeout            = 2 * time.Hour
)

// Progress is one live sample of a download, for the ops console.
type Progress struct {
	// Downloaded is bytes received so far.
	Downloaded int64 `json:"downloaded_bytes"`
	// Total is the declared Content-Length, or 0 when the server did not send one.
	Total int64 `json:"total_bytes,omitempty"`
	// BytesPerSec is the average rate since the transfer started.
	BytesPerSec int64 `json:"bytes_per_sec,omitempty"`
	// ETASec is seconds remaining at the current rate, or nil when unknowable.
	ETASec *int64 `json:"eta_sec,omitempty"`
}

// Result describes a completed fetch. Everything here is recorded in the job's
// data_migrations metadata so "where did this dataset come from?" has an answer on
// disk rather than in someone's shell history.
type Result struct {
	FeedID       string `json:"feed_id,omitempty"`
	SourceURL    string `json:"source_url"`
	Path         string `json:"path"`
	Bytes        int64  `json:"bytes"`
	SHA256       string `json:"sha256"`
	ETag         string `json:"etag,omitempty"`
	LastModified string `json:"last_modified,omitempty"`
	FetchedAt    string `json:"fetched_at"`
	// NotModified is true when the server answered 304 and the existing staged
	// archive was kept.
	NotModified bool `json:"not_modified,omitempty"`
}

// Fetch downloads the source archive into staging and verifies what it wrote.
//
// The download goes to a temporary file and is renamed into place only after the
// byte count and digest are known, so a failed or truncated transfer can never leave
// something that looks like a usable archive.
func Fetch(ctx context.Context, src Source, opts Options) (Result, error) {
	opts = opts.withDefaults()

	if err := os.MkdirAll(opts.StagingDir, 0o755); err != nil {
		return Result{}, fmt.Errorf("create staging directory %s: %w", opts.StagingDir, err)
	}
	dest := filepath.Join(opts.StagingDir, src.Filename)

	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src.URL.String(), nil)
	if err != nil {
		return Result{}, err
	}
	// Conditional request: re-downloading a multi-hundred-megabyte archive that has
	// not changed is the common case when an operator clicks Fetch twice.
	if tag := readSidecar(dest).ETag; tag != "" {
		req.Header.Set("If-None-Match", tag)
	}
	req.Header.Set("Accept", "application/zip")

	resp, err := opts.Client.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("fetch %s: %w", src.URL.Redacted(), err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotModified {
		prev := readSidecar(dest)
		prev.NotModified = true
		return prev, nil
	}
	if resp.StatusCode != http.StatusOK {
		return Result{}, fmt.Errorf("fetch %s: server returned %s", src.URL.Redacted(), resp.Status)
	}

	declared := resp.ContentLength
	if declared > opts.MaxBytes {
		return Result{}, fmt.Errorf("%w: declared %d bytes, cap is %d", ErrTooLarge, declared, opts.MaxBytes)
	}
	if err := opts.checkSpace(opts.StagingDir, declared); err != nil {
		return Result{}, err
	}

	written, digest, err := opts.copyToTemp(ctx, dest, resp.Body, declared)
	if err != nil {
		return Result{}, err
	}

	result := Result{
		FeedID:       src.FeedID,
		SourceURL:    src.URL.Redacted(),
		Path:         dest,
		Bytes:        written,
		SHA256:       digest,
		ETag:         resp.Header.Get("ETag"),
		LastModified: resp.Header.Get("Last-Modified"),
		FetchedAt:    time.Now().UTC().Format(time.RFC3339),
	}
	writeSidecar(dest, result)
	slog.Info("dataset fetch complete",
		slog.String("url", result.SourceURL),
		slog.String("path", dest),
		slog.Int64("bytes", written),
		slog.String("sha256", digest))
	return result, nil
}

// copyToTemp streams the body to a temp file beside dest, enforcing the cap as it
// goes, then renames on success. It returns the byte count and hex digest.
func (o Options) copyToTemp(ctx context.Context, dest string, body io.Reader, declared int64) (int64, string, error) {
	tmp, err := os.CreateTemp(filepath.Dir(dest), filepath.Base(dest)+".part-*")
	if err != nil {
		return 0, "", fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	// Removing the temp file is unconditional: on the success path the rename below
	// has already moved it, and Remove on a renamed path is a harmless no-op.
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}()

	hasher := sha256.New()
	counter := &progressWriter{
		total:    declared,
		started:  time.Now(),
		report:   o.Progress,
		interval: progressInterval,
	}
	// LimitReader caps the stream one byte above the limit so an overrun is
	// detectable rather than silently truncated at the cap.
	limited := io.LimitReader(body, o.MaxBytes+1)
	written, err := io.Copy(io.MultiWriter(tmp, hasher, counter), &ctxReader{ctx: ctx, r: limited})
	if err != nil {
		return 0, "", fmt.Errorf("download: %w", err)
	}
	if written > o.MaxBytes {
		return 0, "", fmt.Errorf("%w: exceeded %d bytes mid-stream", ErrTooLarge, o.MaxBytes)
	}
	if declared > 0 && written != declared {
		return 0, "", fmt.Errorf("%w: got %d of %d bytes", ErrShortRead, written, declared)
	}
	if written == 0 {
		return 0, "", fmt.Errorf("%w: server sent an empty body", ErrShortRead)
	}
	if err := tmp.Sync(); err != nil {
		return 0, "", fmt.Errorf("flush download: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return 0, "", fmt.Errorf("close download: %w", err)
	}
	if err := os.Rename(tmpName, dest); err != nil {
		return 0, "", fmt.Errorf("move download into place: %w", err)
	}
	counter.flush()
	return written, hex.EncodeToString(hasher.Sum(nil)), nil
}

// checkSpace refuses a download the filesystem cannot hold. When the server declares
// no Content-Length there is nothing to compare against, so the headroom alone is
// required — a weaker guarantee, stated rather than pretended away.
func (o Options) checkSpace(dir string, declared int64) error {
	free, err := o.FreeSpace(dir)
	if err != nil {
		slog.Warn("dataset fetch: free-space check unavailable, proceeding", slog.Any("err", err))
		return nil
	}
	need := declared + o.MinFreeBytes
	if free < need {
		return fmt.Errorf("%w: %s has %d bytes free, needs %d", ErrInsufficientSpace, dir, free, need)
	}
	return nil
}

// withDefaults fills the zero values so callers only set what they care about.
func (o Options) withDefaults() Options {
	if o.MaxBytes <= 0 {
		o.MaxBytes = DefaultMaxBytes
	}
	if o.MinFreeBytes <= 0 {
		o.MinFreeBytes = DefaultMinFreeBytes
	}
	if o.Timeout <= 0 {
		o.Timeout = DefaultTimeout
	}
	if o.Client == nil {
		o.Client = newClient()
	}
	if o.FreeSpace == nil {
		o.FreeSpace = FreeSpace
	}
	return o
}

// newClient returns an HTTP client that re-checks the allowlist on every redirect.
//
// Validating only the URL the operator typed is not enough: a 302 is a redirect to
// wherever the responder chooses, and an allowlisted host that redirects off-list
// would hand back exactly the SSRF the allowlist is there to prevent.
func newClient() *http.Client {
	return &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			if req.URL.Scheme != "https" {
				return fmt.Errorf("redirect to non-https url %s", req.URL.Redacted())
			}
			if !hostAllowed(req.URL.Host) {
				return fmt.Errorf("%w: redirected to %s", ErrHostNotAllowed, req.URL.Host)
			}
			return nil
		},
	}
}

// ctxReader makes io.Copy cancellable: without it a stalled transfer ignores a
// cancelled job context until the whole-request timeout fires.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c *ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}

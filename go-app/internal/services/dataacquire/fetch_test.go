package dataacquire

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testSource builds a Source pointing at a test server, bypassing the allowlist.
// The allowlist is exercised by ResolveSource's own tests; wiring httptest through it
// would mean either weakening the list for tests or not testing the transfer at all.
func testSource(t *testing.T, rawURL string) Source {
	t.Helper()
	u, err := url.Parse(rawURL)
	require.NoError(t, err)
	return Source{URL: u, Filename: "all_json.zip"}
}

// testOptions points a fetch at a temp staging directory with the space check stubbed.
func testOptions(t *testing.T) Options {
	t.Helper()
	return Options{
		StagingDir: t.TempDir(),
		FreeSpace:  func(string) (int64, error) { return 1 << 40, nil },
		Timeout:    10 * time.Second,
	}
}

func serve(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

func TestFetch_WritesArchiveAndRecordsDigest(t *testing.T) {
	t.Parallel()
	payload := []byte("PK\x03\x04 pretend this is a zip")
	srv := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("ETag", `"abc123"`)
		w.Header().Set("Last-Modified", "Wed, 21 Oct 2026 07:28:00 GMT")
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		_, _ = w.Write(payload)
	})

	opts := testOptions(t)
	result, err := Fetch(context.Background(), testSource(t, srv.URL+"/all_json.zip"), opts)
	require.NoError(t, err)

	sum := sha256.Sum256(payload)
	assert.Equal(t, hex.EncodeToString(sum[:]), result.SHA256)
	assert.Equal(t, int64(len(payload)), result.Bytes)
	assert.Equal(t, `"abc123"`, result.ETag)
	assert.NotEmpty(t, result.LastModified)

	onDisk, err := os.ReadFile(filepath.Join(opts.StagingDir, "all_json.zip"))
	require.NoError(t, err)
	assert.Equal(t, payload, onDisk)
}

// TestFetch_RejectsATruncatedDownload is the --fail-less-curl bug in new clothes: a
// transfer that ends early must fail loudly rather than leave a short archive that
// only breaks later, during extraction or import.
func TestFetch_RejectsATruncatedDownload(t *testing.T) {
	t.Parallel()
	srv := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		// Declare more than we send, then hang up.
		w.Header().Set("Content-Length", "5000")
		_, _ = w.Write([]byte("only a few bytes"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		panic(http.ErrAbortHandler)
	})

	opts := testOptions(t)
	_, err := Fetch(context.Background(), testSource(t, srv.URL+"/all_json.zip"), opts)
	require.Error(t, err)

	entries, readErr := os.ReadDir(opts.StagingDir)
	require.NoError(t, readErr)
	for _, e := range entries {
		assert.NotEqual(t, "all_json.zip", e.Name(), "a failed download must not leave a usable archive")
	}
}

// TestCopyToTemp_RejectsAShortBody exercises the byte-count postcondition directly.
// Reached through Fetch it is usually pre-empted by the transport noticing the
// truncation first, which is exactly why it is worth having: it is the check that
// holds when the transport does not complain.
func TestCopyToTemp_RejectsAShortBody(t *testing.T) {
	t.Parallel()
	opts := testOptions(t).withDefaults()
	dest := filepath.Join(opts.StagingDir, "all_json.zip")

	_, _, err := opts.copyToTemp(context.Background(), dest, strings.NewReader("short"), 5000)
	require.ErrorIs(t, err, ErrShortRead)
	assert.Contains(t, err.Error(), "5 of 5000")

	_, statErr := os.Stat(dest)
	assert.True(t, os.IsNotExist(statErr), "a short body must leave nothing in place")
}

// TestCopyToTemp_LeavesNoPartFilesBehind guards the temp-and-rename discipline: a
// staging directory littered with .part-* files is how a later extract picks up a
// half-written archive.
func TestCopyToTemp_LeavesNoPartFilesBehind(t *testing.T) {
	t.Parallel()
	opts := testOptions(t).withDefaults()
	dest := filepath.Join(opts.StagingDir, "all_json.zip")

	_, _, err := opts.copyToTemp(context.Background(), dest, strings.NewReader("short"), 5000)
	require.Error(t, err)

	entries, err := os.ReadDir(opts.StagingDir)
	require.NoError(t, err)
	assert.Empty(t, entries, "the failed download's temp file must be cleaned up")
}

func TestFetch_RejectsAnEmptyBody(t *testing.T) {
	t.Parallel()
	srv := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "0")
	})
	_, err := Fetch(context.Background(), testSource(t, srv.URL+"/all_json.zip"), testOptions(t))
	require.ErrorIs(t, err, ErrShortRead)
}

func TestFetch_RefusesAnArchiveOverTheCapByDeclaredSize(t *testing.T) {
	t.Parallel()
	srv := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "1000000")
		_, _ = w.Write(make([]byte, 1000))
	})
	opts := testOptions(t)
	opts.MaxBytes = 1024

	_, err := Fetch(context.Background(), testSource(t, srv.URL+"/all_json.zip"), opts)
	require.ErrorIs(t, err, ErrTooLarge)
}

// TestFetch_RefusesAnArchiveThatOutgrowsTheCapMidStream covers the case the declared
// size cannot: a server that sends no Content-Length, or lies about it.
func TestFetch_RefusesAnArchiveThatOutgrowsTheCapMidStream(t *testing.T) {
	t.Parallel()
	srv := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		// No Content-Length: chunked, so the cap is the only limit.
		chunk := make([]byte, 512)
		for range 20 {
			if _, err := w.Write(chunk); err != nil {
				return
			}
		}
	})
	opts := testOptions(t)
	opts.MaxBytes = 1024

	_, err := Fetch(context.Background(), testSource(t, srv.URL+"/all_json.zip"), opts)
	require.ErrorIs(t, err, ErrTooLarge)
}

func TestFetch_RefusesWhenTheDiskCannotHoldIt(t *testing.T) {
	t.Parallel()
	srv := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "1000")
		_, _ = w.Write(make([]byte, 1000))
	})
	opts := testOptions(t)
	opts.FreeSpace = func(string) (int64, error) { return 100, nil }
	opts.MinFreeBytes = 1

	_, err := Fetch(context.Background(), testSource(t, srv.URL+"/all_json.zip"), opts)
	require.ErrorIs(t, err, ErrInsufficientSpace)
}

// TestFetch_ProceedsWhenFreeSpaceIsUnknowable states the deliberate choice: an
// unavailable statfs is reported and stepped over, not turned into a refusal.
func TestFetch_ProceedsWhenFreeSpaceIsUnknowable(t *testing.T) {
	t.Parallel()
	payload := []byte("zip bytes")
	srv := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		_, _ = w.Write(payload)
	})
	opts := testOptions(t)
	opts.FreeSpace = func(string) (int64, error) { return 0, errors.New("statfs unavailable") }

	_, err := Fetch(context.Background(), testSource(t, srv.URL+"/all_json.zip"), opts)
	require.NoError(t, err)
}

// TestFetch_SendsIfNoneMatchAndTreats304AsSuccess covers the re-click case: an
// operator pressing Fetch twice should not re-pull hundreds of megabytes.
func TestFetch_SendsIfNoneMatchAndTreats304AsSuccess(t *testing.T) {
	t.Parallel()
	payload := []byte("zip bytes")
	var conditional string
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if tag := r.Header.Get("If-None-Match"); tag != "" {
			conditional = tag
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		_, _ = w.Write(payload)
	})

	opts := testOptions(t)
	src := testSource(t, srv.URL+"/all_json.zip")

	first, err := Fetch(context.Background(), src, opts)
	require.NoError(t, err)
	require.False(t, first.NotModified)

	second, err := Fetch(context.Background(), src, opts)
	require.NoError(t, err, "304 is success, not failure")
	assert.True(t, second.NotModified)
	assert.Equal(t, `"v1"`, conditional, "the second request must carry the first response's ETag")
	assert.Equal(t, first.SHA256, second.SHA256, "the staged archive's digest still stands")
}

func TestFetch_ReportsProgress(t *testing.T) {
	t.Parallel()
	payload := make([]byte, 64*1024)
	srv := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		_, _ = w.Write(payload)
	})

	var last Progress
	opts := testOptions(t)
	opts.Progress = func(p Progress) { last = p }

	_, err := Fetch(context.Background(), testSource(t, srv.URL+"/all_json.zip"), opts)
	require.NoError(t, err)
	assert.Equal(t, int64(len(payload)), last.Downloaded, "the final sample must report the whole transfer")
	assert.Equal(t, int64(len(payload)), last.Total)
}

func TestFetch_FailsOnNonOKStatus(t *testing.T) {
	t.Parallel()
	srv := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "gone", http.StatusNotFound)
	})
	_, err := Fetch(context.Background(), testSource(t, srv.URL+"/all_json.zip"), testOptions(t))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

// refusingReader fails the test if it is read at all. It is how the cancellation test
// below asserts that ctxReader stops *before* the wrapped reader, rather than asserting
// an error that the wrapped reader could equally have produced.
type refusingReader struct{ t *testing.T }

func (r refusingReader) Read([]byte) (int, error) {
	r.t.Helper()
	r.t.Error("ctxReader read the body on a cancelled context")
	return 0, errors.New("must not be read")
}

// TestCtxReader_Read_CancelledContext_ReturnsContextCanceledWithoutReading drives the
// cancellation branch directly, which is the only way to assert it deterministically.
//
// Through Fetch the branch is a race: a cancelled job also fails the HTTP transport's own
// read, so the transfer ends with an error either way and "Fetch returned an error" says
// nothing about which of the two produced it. It is a race the coverage figure could see —
// this statement was covered on a fast box and not on the CI runner, moving go-app's total
// across a rounding boundary (docs/BUG_BACKLOG.md § B-9).
func TestCtxReader_Read_CancelledContext_ReturnsContextCanceledWithoutReading(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	reader := &ctxReader{ctx: ctx, r: refusingReader{t: t}}
	n, err := reader.Read(make([]byte, 8))

	require.ErrorIs(t, err, context.Canceled)
	assert.Zero(t, n, "a cancelled read reports no bytes")
}

// TestCtxReader_Read_LiveContext_ReadsTheWrappedReader is the other half: a live context
// must cost the transfer nothing, so the wrapper is a pass-through until it is not.
func TestCtxReader_Read_LiveContext_ReadsTheWrappedReader(t *testing.T) {
	t.Parallel()
	reader := &ctxReader{ctx: context.Background(), r: strings.NewReader("cricsheet")}

	buf := make([]byte, 9)
	n, err := io.ReadFull(reader, buf)

	require.NoError(t, err)
	assert.Equal(t, 9, n)
	assert.Equal(t, "cricsheet", string(buf))
}

// TestFetch_CancellingTheJobFailsTheTransfer is the end-to-end half: whichever of
// ctxReader and the HTTP transport notices the cancellation first, the download must fail
// with the cancellation rather than complete or hang. Which one notices is deliberately
// not asserted here — that is what the two ctxReader tests above cover.
func TestFetch_CancellingTheJobFailsTheTransfer(t *testing.T) {
	t.Parallel()
	srv := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		chunk := make([]byte, 4096)
		for range 10000 {
			if _, err := w.Write(chunk); err != nil {
				return
			}
			time.Sleep(time.Millisecond)
		}
	})

	ctx, cancel := context.WithCancel(context.Background())
	// Deferred as well as called from the goroutine: if Fetch returns early the
	// goroutine may never reach its cancel, and a leaked context is a leaked timer.
	// Cancelling twice is defined to be a no-op.
	defer cancel()
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := Fetch(ctx, testSource(t, srv.URL+"/all_json.zip"), testOptions(t))
	require.ErrorIs(t, err, context.Canceled, "a cancelled job must stop the transfer")
}

// TestNewClient_RejectsRedirectsOffTheAllowlist is the second half of the SSRF guard:
// validating only the typed URL is defeated by a 302.
func TestNewClient_RejectsRedirectsOffTheAllowlist(t *testing.T) {
	t.Parallel()
	client := newClient()
	require.NotNil(t, client.CheckRedirect)

	offList, err := http.NewRequest(http.MethodGet, "https://attacker.example/all_json.zip", nil)
	require.NoError(t, err)
	assert.ErrorIs(t, client.CheckRedirect(offList, nil), ErrHostNotAllowed)

	plaintext, err := http.NewRequest(http.MethodGet, "http://cricsheet.org/all_json.zip", nil)
	require.NoError(t, err)
	assert.Error(t, client.CheckRedirect(plaintext, nil), "a downgrade to http must be refused")

	onList, err := http.NewRequest(http.MethodGet, "https://cricsheet.org/downloads/all_json.zip", nil)
	require.NoError(t, err)
	assert.NoError(t, client.CheckRedirect(onList, nil))

	tooMany := make([]*http.Request, 5)
	assert.Error(t, client.CheckRedirect(onList, tooMany), "a redirect loop must terminate")
}

func TestStatus_IsAbsentUntilAFetchPublishes(t *testing.T) {
	// Not parallel: it touches the package-level live-progress slot.
	ClearProgress()
	_, _, ok := Status()
	assert.False(t, ok, "nothing downloading means no progress, not zero progress")

	PublishProgress("https://cricsheet.org/downloads/all_json.zip", Progress{Downloaded: 10, Total: 100})
	progress, source, ok := Status()
	require.True(t, ok)
	assert.Equal(t, int64(10), progress.Downloaded)
	assert.Contains(t, source, "cricsheet.org")

	ClearProgress()
	_, _, ok = Status()
	assert.False(t, ok, "a finished fetch must not leave the console showing a transfer")
}

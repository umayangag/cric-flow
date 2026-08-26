// Package datasetregistry records which Cricsheet datasets this box has acquired.
//
// It exists because data_migrations answers "what ran?" and not "what data is here?".
// A run is not a dataset: two fetches of an unchanged archive are two runs and one
// dataset, and a fetch followed by an extract is two runs and still one dataset.
// Without a table that says so, "which data produced this model?" (P-1, P-2) has to
// be reconstructed from job metadata every time, which is why the plan makes this a
// prerequisite rather than a nicety.
//
// The archive's SHA-256 is the identity. Cricsheet reuses filenames across releases —
// all_json.zip is always all_json.zip — so keying on the name would conflate every
// dataset ever fetched into one row.
package datasetregistry

import (
	"context"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// Dataset is one acquired dataset as the registry holds it.
type Dataset struct {
	ID     int64  `json:"id"`
	SHA256 string `json:"sha256"`

	// Feed and SourceURL are empty for an archive placed in staging by hand. That is
	// reported as unknown rather than guessed at.
	Feed      string `json:"feed,omitempty"`
	SourceURL string `json:"source_url,omitempty"`
	Filename  string `json:"filename"`
	Bytes     int64  `json:"bytes"`

	ETag         string `json:"etag,omitempty"`
	LastModified string `json:"last_modified,omitempty"`

	// FetchedAt is empty for a hand-placed archive; ExtractedAt is empty for one that
	// has been downloaded but not yet extracted.
	FetchedAt   string `json:"fetched_at,omitempty"`
	ExtractedAt string `json:"extracted_at,omitempty"`

	EntryCount     int    `json:"entry_count,omitempty"`
	MatchFiles     int    `json:"match_files,omitempty"`
	ExtractedBytes int64  `json:"extracted_bytes,omitempty"`
	DestDir        string `json:"dest_dir,omitempty"`

	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`

	// Live is true for the dataset currently in the data directory. It is derived
	// from the on-disk manifest rather than stored, so an operator who changes the
	// directory by other means cannot leave the registry asserting something false.
	Live bool `json:"live"`
}

// FetchRecord is what a completed download knows about a dataset.
type FetchRecord struct {
	SHA256       string
	Feed         string
	SourceURL    string
	Filename     string
	Bytes        int64
	ETag         string
	LastModified string
	FetchedAt    time.Time
}

// ExtractRecord is what a completed extraction knows about a dataset.
type ExtractRecord struct {
	SHA256         string
	Filename       string
	Bytes          int64
	EntryCount     int
	MatchFiles     int
	ExtractedBytes int64
	DestDir        string
	ExtractedAt    time.Time
	// Feed and SourceURL carry across from the archive's sidecar when it has one, so
	// extracting a fetched archive does not lose where it came from.
	Feed      string
	SourceURL string
}

// RecordFetch inserts or updates the row for a downloaded archive.
//
// Conflicting on sha256 rather than inserting blindly is what makes a re-fetch
// idempotent: identical bytes are the same dataset however many times they arrive.
// The extract columns are deliberately untouched — re-downloading an archive that is
// already extracted must not erase the record of that extraction.
func RecordFetch(ctx context.Context, r FetchRecord) error {
	if !db.Available() || r.SHA256 == "" {
		return nil
	}
	return db.Exec(ctx, `
		INSERT INTO datasets (sha256, feed, source_url, filename, bytes, etag, last_modified, fetched_at)
		VALUES ($1, NULLIF($2, ''), NULLIF($3, ''), $4, $5, NULLIF($6, ''), NULLIF($7, ''), $8)
		ON CONFLICT (sha256) DO UPDATE SET
			feed          = COALESCE(EXCLUDED.feed, datasets.feed),
			source_url    = COALESCE(EXCLUDED.source_url, datasets.source_url),
			filename      = EXCLUDED.filename,
			bytes         = EXCLUDED.bytes,
			etag          = COALESCE(EXCLUDED.etag, datasets.etag),
			last_modified = COALESCE(EXCLUDED.last_modified, datasets.last_modified),
			fetched_at    = EXCLUDED.fetched_at,
			updated_at    = now()
	`, r.SHA256, r.Feed, r.SourceURL, r.Filename, r.Bytes, r.ETag, r.LastModified, r.FetchedAt)
}

// RecordExtract inserts or updates the row for an extracted archive.
//
// It inserts rather than only updating because an archive can be extracted without
// ever having been fetched through this service — an operator may place one in
// staging. Recording it on the way through is the only chance to know it exists.
func RecordExtract(ctx context.Context, r ExtractRecord) error {
	if !db.Available() || r.SHA256 == "" {
		return nil
	}
	return db.Exec(ctx, `
		INSERT INTO datasets (
			sha256, feed, source_url, filename, bytes,
			entry_count, match_files, extracted_bytes, dest_dir, extracted_at
		)
		VALUES ($1, NULLIF($2, ''), NULLIF($3, ''), $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (sha256) DO UPDATE SET
			feed            = COALESCE(datasets.feed, EXCLUDED.feed),
			source_url      = COALESCE(datasets.source_url, EXCLUDED.source_url),
			filename        = EXCLUDED.filename,
			entry_count     = EXCLUDED.entry_count,
			match_files     = EXCLUDED.match_files,
			extracted_bytes = EXCLUDED.extracted_bytes,
			dest_dir        = EXCLUDED.dest_dir,
			extracted_at    = EXCLUDED.extracted_at,
			updated_at      = now()
	`, r.SHA256, r.Feed, r.SourceURL, r.Filename, r.Bytes,
		r.EntryCount, r.MatchFiles, r.ExtractedBytes, r.DestDir, r.ExtractedAt)
}

// List returns the registry, newest first, with liveSHA256's row marked Live.
//
// The caller passes the live digest rather than this package reading the filesystem:
// which directory is live is a question about configuration and disk, and mixing that
// into a repository would make it untestable without one.
func List(ctx context.Context, limit int, liveSHA256 string) ([]Dataset, error) {
	if !db.Available() {
		return nil, nil
	}
	if limit <= 0 {
		limit = 50
	}

	rows, err := db.Query(ctx, `
		SELECT id, sha256, feed, source_url, filename, bytes, etag, last_modified,
		       fetched_at, extracted_at, entry_count, match_files, extracted_bytes,
		       dest_dir, created_at, updated_at
		FROM datasets
		ORDER BY created_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Dataset, 0, limit)
	for rows.Next() {
		d, err := scanDataset(rows)
		if err != nil {
			return nil, err
		}
		d.Live = liveSHA256 != "" && d.SHA256 == liveSHA256
		out = append(out, d)
	}
	return out, rows.Err()
}

package datasetregistry

import (
	"database/sql"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// scanDataset reads one registry row.
//
// Nearly every column is nullable, and deliberately so: a hand-placed archive has no
// feed or URL, and an archive that has been fetched but not extracted has no entry
// count. Scanning through sql.Null* and rendering absence as an empty field is what
// lets the UI distinguish "not known" from "zero" — the difference between "we do not
// know where this came from" and "it came from nowhere".
func scanDataset(rows db.Rows) (Dataset, error) {
	var (
		d              Dataset
		feed           sql.NullString
		sourceURL      sql.NullString
		etag           sql.NullString
		lastModified   sql.NullString
		fetchedAt      sql.NullTime
		extractedAt    sql.NullTime
		entryCount     sql.NullInt64
		matchFiles     sql.NullInt64
		extractedBytes sql.NullInt64
		destDir        sql.NullString
		createdAt      time.Time
		updatedAt      time.Time
	)

	if err := rows.Scan(
		&d.ID, &d.SHA256, &feed, &sourceURL, &d.Filename, &d.Bytes, &etag, &lastModified,
		&fetchedAt, &extractedAt, &entryCount, &matchFiles, &extractedBytes,
		&destDir, &createdAt, &updatedAt,
	); err != nil {
		return Dataset{}, err
	}

	d.Feed = feed.String
	d.SourceURL = sourceURL.String
	d.ETag = etag.String
	d.LastModified = lastModified.String
	d.DestDir = destDir.String
	d.EntryCount = int(entryCount.Int64)
	d.MatchFiles = int(matchFiles.Int64)
	d.ExtractedBytes = extractedBytes.Int64
	d.FetchedAt = formatTime(fetchedAt)
	d.ExtractedAt = formatTime(extractedAt)
	d.CreatedAt = createdAt.UTC().Format(time.RFC3339)
	d.UpdatedAt = updatedAt.UTC().Format(time.RFC3339)
	return d, nil
}

// formatTime renders a nullable timestamp, or "" when it is null.
func formatTime(t sql.NullTime) string {
	if !t.Valid {
		return ""
	}
	return t.Time.UTC().Format(time.RFC3339)
}

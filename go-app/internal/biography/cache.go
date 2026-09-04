package biography

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Cache is the local record of what Wikidata has already been asked.
//
// It exists so the backfill is resumable: a run interrupted after twenty batches asks for
// the remaining ten and nothing else. It is append-only JSON Lines rather than a
// serialised map because an append survives being killed mid-write — the worst a torn
// last line costs is one batch re-asked — where a rewritten map file would be truncated
// to nothing.
//
// A miss is cached too. "Wikidata has no item carrying this id" is an answer, and a cache
// that only remembered hits would re-ask every miss on every run, which for this dataset
// is most of the requests.
type Cache struct {
	path    string
	entries map[string]Lookup
}

// OpenCache reads an existing cache file, creating nothing. A missing file is an empty
// cache, not an error: the first run has none by definition.
func OpenCache(path string) (*Cache, error) {
	cache := &Cache{path: path, entries: make(map[string]Lookup)}
	file, err := os.Open(path) // #nosec G304 -- the path is an operator's own flag
	if errors.Is(err, os.ErrNotExist) {
		return cache, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var lookup Lookup
		if err := json.Unmarshal([]byte(line), &lookup); err != nil {
			// A torn final line is what an interrupted append leaves behind. Dropping it
			// costs one batch on the next run; refusing to open the cache would cost all
			// of them, which is the opposite of resumable.
			continue
		}
		if lookup.CricinfoID != "" {
			cache.entries[lookup.CricinfoID] = lookup
		}
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return cache, nil
}

// Get returns what is known about an id and whether it has been asked at all.
func (c *Cache) Get(cricinfoID string) (Lookup, bool) {
	lookup, ok := c.entries[cricinfoID]
	return lookup, ok
}

// Len is how many ids the cache has answers for.
func (c *Cache) Len() int { return len(c.entries) }

// Put appends answers for a batch, including the misses, and flushes before returning.
//
// Flushing per batch rather than per run is the whole point: the file on disk is always a
// truthful record of what has been asked, so a run that is killed loses at most the batch
// in flight.
func (c *Cache) Put(lookups []Lookup) error {
	if len(lookups) == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o750); err != nil {
		return err
	}
	file, err := os.OpenFile(c.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) // #nosec G304
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	writer := bufio.NewWriter(file)
	for _, lookup := range lookups {
		if lookup.CricinfoID == "" {
			continue
		}
		encoded, err := json.Marshal(lookup)
		if err != nil {
			return fmt.Errorf("encoding a cache entry for %s: %w", lookup.CricinfoID, err)
		}
		if _, err := writer.Write(append(encoded, '\n')); err != nil {
			return err
		}
		c.entries[lookup.CricinfoID] = lookup
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	return file.Sync()
}

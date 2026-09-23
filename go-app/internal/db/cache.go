package db

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/umayangag/cric-flow/go-app/internal/venues"
)

// EntityCache provides a thread-safe, process-global cache for database entities
// to minimize roundtrips during high-concurrency imports.
//
// A miss goes to the repository function behind it and returns whatever that returns,
// error included. Each lookup used to answer (0, nil) when there was no pool -- a
// convenience for tests that swallowed the "db pool not initialized" the repository
// function already reported -- so a caller that asked for an id got zero and no error,
// and the importer wrote that zero into format_id, venue_id, season_id, player_id and
// opposition_id for every row of every file (IMPORT-13). A lookup that cannot reach its
// table has not found a zero; it has failed.
type EntityCache struct {
	players     sync.Map // player key (see playerKey) -> int64
	venues      sync.Map // venue key (see GetVenueID) -> int64
	seasons     sync.Map // string -> int64
	formats     sync.Map // string -> int64
	oppositions sync.Map // opposition key (see oppositionKey) -> int64
}

// playerKey is the cache key for a player: the Cricsheet person identifier when the
// source has one, and the name only when it does not. The two are namespaced apart so a
// registry identifier can never collide with a name and silently merge two people --
// which is the whole class of bug this identity work removes.
func playerKey(externalID, name string) string {
	if externalID != "" {
		return "id:" + externalID
	}
	return "name:" + name
}

// oppositionKey is the cache key for a team: name and gender, matching the table's
// unique constraint. NUL separates them so "India" + "" cannot be confused with "Indi"
// + "a".
func oppositionKey(name, gender string) string {
	return name + "\x00" + gender
}

var globalCache = &EntityCache{}

// GetGlobalCache returns the singleton cache instance.
func GetGlobalCache() *EntityCache {
	return globalCache
}

// memoiseID remembers an id under a key, unless the id is zero.
//
// Every dimension table's primary key is a serial starting at 1, so zero is not a row:
// it is a lookup that produced nothing while reporting success. Remembering it would
// hand that nothing to every later caller of the same key, and the caller would write it
// into a foreign key -- which is what a fake pool in a test does to every real lookup
// that follows it in the same process.
func memoiseID(store *sync.Map, key string, id int64) {
	if id == 0 {
		return
	}
	store.Store(key, id)
}

// GetPlayerID returns the ID for a player, keyed by the Cricsheet person identifier.
//
// externalID empty takes the name-keyed fallback, which is the pre-identity behaviour and
// the only path that can still merge two people. Callers log when they take it.
// nameAsOf is the match date the name came from; it is stored on a new row so an import
// can later settle which spelling to display.
func (c *EntityCache) GetPlayerID(ctx context.Context, externalID, name, nameAsOf string) (int64, error) {
	key := playerKey(externalID, name)
	if id, ok := c.players.Load(key); ok {
		return id.(int64), nil
	}
	id, _, err := GetOrCreatePlayer(ctx, externalID, name, nameAsOf)
	if err != nil {
		return 0, err
	}
	memoiseID(&c.players, key, id)
	return id, nil
}

// GetVenueID returns the ID for a venue named by a match file, using the cache if
// available. `city` is the city that file named beside the ground, and may be empty.
//
// The cache key is the folded identity key plus the city, not the spelling: two spellings
// of one ground share an entry, so the second never reaches the database, while a ground
// first seen without a city still gets one when a later file names it. Both halves matter
// -- keying on the spelling alone would let a rival row through, and keying on the ground
// alone would freeze `venue.city` empty for whichever ground the archive first mentions
// without one (IMPORT-08).
func (c *EntityCache) GetVenueID(ctx context.Context, name, city string) (int64, error) {
	key := venues.NormalizeName(name) + "\x00" + city
	if id, ok := c.venues.Load(key); ok {
		return id.(int64), nil
	}
	id, err := GetOrCreateVenue(ctx, name, city)
	if err != nil {
		return 0, err
	}
	memoiseID(&c.venues, key, id)
	return id, nil
}

// GetSeasonID returns the ID for a season name, using the cache if available.
func (c *EntityCache) GetSeasonID(ctx context.Context, name string) (int64, error) {
	if id, ok := c.seasons.Load(name); ok {
		return id.(int64), nil
	}
	id, err := GetOrCreateSeason(ctx, name)
	if err != nil {
		return 0, err
	}
	memoiseID(&c.seasons, name, id)
	return id, nil
}

// GetFormatID returns the ID for a format code, using the cache if available.
func (c *EntityCache) GetFormatID(ctx context.Context, code string) (int64, error) {
	if id, ok := c.formats.Load(code); ok {
		return id.(int64), nil
	}
	id, err := GetMatchFormatIDByCode(ctx, code)
	if err != nil {
		return 0, err
	}
	memoiseID(&c.formats, code, id)
	return id, nil
}

// FormatCodesForTrainingBucket is the format codes one training bucket covers, spelled
// the way the format dimension spells them. T20 and T20I are one bucket: both codes come
// back so matches stored under either are included (Cricsheet/ingest may store T20I as
// T20). It is a naming rule and nothing else, which is why it takes no database.
func FormatCodesForTrainingBucket(format string) []string {
	code := strings.ToUpper(strings.TrimSpace(format))
	switch code {
	case "T20", "T20I":
		return []string{"T20", "T20I"}
	default:
		return []string{code}
	}
}

// GetFormatIDsForTrainingBucket returns the format IDs one format bucket covers.
func (c *EntityCache) GetFormatIDsForTrainingBucket(ctx context.Context, format string) ([]int64, error) {
	codes := FormatCodesForTrainingBucket(format)
	ids := make([]int64, 0, len(codes))
	for _, code := range codes {
		id, err := c.GetFormatID(ctx, code)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// GetOppositionID returns the ID for a team, using the cache if available. A team is
// (name, gender): see GetOrCreateOpposition.
func (c *EntityCache) GetOppositionID(ctx context.Context, name, gender string) (int64, error) {
	key := oppositionKey(name, gender)
	if id, ok := c.oppositions.Load(key); ok {
		return id.(int64), nil
	}
	id, err := GetOrCreateOpposition(ctx, name, gender)
	if err != nil {
		return 0, err
	}
	memoiseID(&c.oppositions, key, id)
	return id, nil
}

// WarmupPlayers pre-loads all players into the cache. Useful on startup.
func (c *EntityCache) WarmupPlayers(ctx context.Context) error {
	if Pool == nil {
		return fmt.Errorf("db pool not initialized")
	}
	rows, err := Pool.Query(ctx, "SELECT id, external_id, player_name FROM player")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var externalID *string
		var name string
		if err := rows.Scan(&id, &externalID, &name); err != nil {
			return err
		}
		c.players.Store(playerKey(derefOrEmpty(externalID), name), id)
	}
	return rows.Err()
}

func derefOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

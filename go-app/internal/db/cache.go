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
	if Pool == nil {
		return 0, nil // Fallback for tests or uninitialized DB
	}
	id, _, err := GetOrCreatePlayer(ctx, externalID, name, nameAsOf)
	if err != nil {
		return 0, err
	}
	c.players.Store(key, id)
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
	if Pool == nil {
		return 0, nil
	}
	id, err := GetOrCreateVenue(ctx, name, city)
	if err != nil {
		return 0, err
	}
	c.venues.Store(key, id)
	return id, nil
}

// GetSeasonID returns the ID for a season name, using the cache if available.
func (c *EntityCache) GetSeasonID(ctx context.Context, name string) (int64, error) {
	if id, ok := c.seasons.Load(name); ok {
		return id.(int64), nil
	}
	if Pool == nil {
		return 0, nil
	}
	id, err := GetOrCreateSeason(ctx, name)
	if err != nil {
		return 0, err
	}
	c.seasons.Store(name, id)
	return id, nil
}

// GetFormatID returns the ID for a format code, using the cache if available.
func (c *EntityCache) GetFormatID(ctx context.Context, code string) (int64, error) {
	if id, ok := c.formats.Load(code); ok {
		return id.(int64), nil
	}
	if Pool == nil {
		return 0, nil
	}
	id, err := GetMatchFormatIDByCode(ctx, code)
	if err != nil {
		return 0, err
	}
	c.formats.Store(code, id)
	return id, nil
}

// GetFormatIDsForTrainingBucket returns the format IDs one format bucket covers.
// T20 and T20I are treated as one bucket: both IDs are returned so matches stored
// under either format_id are included (Cricsheet/ingest may store T20I as T20).
func (c *EntityCache) GetFormatIDsForTrainingBucket(ctx context.Context, format string) ([]int64, error) {
	code := strings.ToUpper(strings.TrimSpace(format))
	switch code {
	case "T20", "T20I":
		idT20, err := c.GetFormatID(ctx, "T20")
		if err != nil {
			return nil, err
		}
		idT20I, err := c.GetFormatID(ctx, "T20I")
		if err != nil {
			return nil, err
		}
		return []int64{idT20, idT20I}, nil
	default:
		id, err := c.GetFormatID(ctx, code)
		if err != nil {
			return nil, err
		}
		return []int64{id}, nil
	}
}

// GetOppositionID returns the ID for a team, using the cache if available. A team is
// (name, gender): see GetOrCreateOpposition.
func (c *EntityCache) GetOppositionID(ctx context.Context, name, gender string) (int64, error) {
	key := oppositionKey(name, gender)
	if id, ok := c.oppositions.Load(key); ok {
		return id.(int64), nil
	}
	if Pool == nil {
		return 0, nil
	}
	id, err := GetOrCreateOpposition(ctx, name, gender)
	if err != nil {
		return 0, err
	}
	c.oppositions.Store(key, id)
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

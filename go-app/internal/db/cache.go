package db

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// EntityCache provides a thread-safe, process-global cache for database entities
// to minimize roundtrips during high-concurrency imports.
type EntityCache struct {
	players     sync.Map // string -> int64
	venues      sync.Map // string -> int64
	seasons     sync.Map // string -> int64
	formats     sync.Map // string -> int64
	oppositions sync.Map // string -> int64
}

var globalCache = &EntityCache{}

// GetGlobalCache returns the singleton cache instance.
func GetGlobalCache() *EntityCache {
	return globalCache
}

// GetPlayerID returns the ID for a player name, using the cache if available.
// If not cached, it calls GetOrCreateByName and updates the cache.
func (c *EntityCache) GetPlayerID(ctx context.Context, name string) (int64, error) {
	if id, ok := c.players.Load(name); ok {
		return id.(int64), nil
	}
	if Pool == nil {
		return 0, nil // Fallback for tests or uninitialized DB
	}
	id, err := GetOrCreateByName(ctx, name)
	if err != nil {
		return 0, err
	}
	c.players.Store(name, id)
	return id, nil
}

// GetVenueID returns the ID for a venue name, using the cache if available.
func (c *EntityCache) GetVenueID(ctx context.Context, name string) (int64, error) {
	if id, ok := c.venues.Load(name); ok {
		return id.(int64), nil
	}
	if Pool == nil {
		return 0, nil
	}
	id, err := GetOrCreateVenue(ctx, name)
	if err != nil {
		return 0, err
	}
	c.venues.Store(name, id)
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

// GetFormatIDsForTrainingBucket returns format IDs to use for training-data queries.
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

// GetOppositionID returns the ID for an opposition name, using the cache if available.
func (c *EntityCache) GetOppositionID(ctx context.Context, name string) (int64, error) {
	if id, ok := c.oppositions.Load(name); ok {
		return id.(int64), nil
	}
	if Pool == nil {
		return 0, nil
	}
	id, err := GetOrCreateOpposition(ctx, name)
	if err != nil {
		return 0, err
	}
	c.oppositions.Store(name, id)
	return id, nil
}

// WarmupPlayers pre-loads all players into the cache. Useful on startup.
func (c *EntityCache) WarmupPlayers(ctx context.Context) error {
	if Pool == nil {
		return fmt.Errorf("db pool not initialized")
	}
	rows, err := Pool.Query(ctx, "SELECT id, player_name FROM player")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return err
		}
		c.players.Store(name, id)
	}
	return nil
}

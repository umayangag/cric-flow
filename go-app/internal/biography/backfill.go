package biography

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"time"
)

// Player is one row of the registry the backfill walks: who to look up, and how much he
// matters. Appearances is carried so the progress log and the coverage report can order
// by it without a second query.
type Player struct {
	ID          int64
	ExternalID  string
	Name        string
	Appearances int64
}

// Store is the persistence the backfill needs. It is an interface so the orchestration
// can be tested without Postgres, and so this package holds no SQL.
type Store interface {
	// ListPlayers returns every player, with his Cricsheet registry identifier and his
	// appearance count.
	ListPlayers(ctx context.Context) ([]Player, error)
	// UpsertBiographies writes the rows, replacing what is there for those players.
	UpsertBiographies(ctx context.Context, records []Record) error
	// Coverage measures what is now stored.
	Coverage(ctx context.Context) (Coverage, error)
}

// Lookuper is the Wikidata side. SPARQLClient implements it; a test supplies a map.
type Lookuper interface {
	Query(ctx context.Context, cricinfoIDs []string) (map[string]Lookup, error)
}

// Options configure one backfill run.
type Options struct {
	// Register is Cricsheet's people register, already parsed.
	Register map[string]RegisterEntry
	// Overrides is the curated file, already parsed. May be nil.
	Overrides map[string]Override
	// Cache is the resumable record of what has been asked. Required.
	Cache *Cache
	// BatchSize is how many ESPNcricinfo ids go into one SPARQL query.
	BatchSize int
	// Now supplies the fetch timestamp, injected so a test can assert on it.
	Now func() time.Time
	// Offline answers every player from the cache alone and asks Wikidata nothing.
	//
	// It is how a purged database is rebuilt from the committed snapshot: the cost of
	// re-acquiring these biographies is a rate-limited pass over every player, so the
	// restore has to be provably incapable of starting one. An id the snapshot has no
	// answer for is left unanswered and reported, not guessed at and not cached as a
	// miss — a fabricated miss would make the next online run skip the one id it should
	// ask about.
	Offline bool
}

// DefaultBatchSize is how many ids one query carries. Five hundred keeps the query well
// inside the service's limits and turns 13,000 players into roughly thirty requests.
const DefaultBatchSize = 500

// Result is what one run did, reported so the operator sees the work rather than
// inferring it from the coverage figure.
type Result struct {
	Players        int
	WithCricinfoID int
	AskedNow       int
	FromCache      int
	Matched        int
	Overridden     int
	// Unanswered is how many ids an offline run had no cached answer for and therefore
	// left alone. It is always zero for an online run, which asks about them instead.
	Unanswered int
}

// Run acquires biographies for every player and stores them.
//
// The shape is: resolve each player's ESPNcricinfo id from the register, ask Wikidata for
// the ids the cache has no answer for, then write one row per player — including the ones
// nothing was found for, because a measured miss is the thing that makes the coverage
// figure a measurement.
//
// It is resumable at batch granularity and idempotent: running it twice against an
// unchanged register and an unchanged cache asks Wikidata nothing and writes the same
// rows.
//
// With Options.Offline the cache is the only source and lookuper is never consulted, so a
// restore from the committed snapshot cannot reach the network even if the snapshot is
// short of an id.
func Run(ctx context.Context, store Store, lookuper Lookuper, options Options) (Result, error) {
	if options.Cache == nil {
		return Result{}, fmt.Errorf("a cache is required; it is what makes the run resumable")
	}
	if !options.Offline && lookuper == nil {
		return Result{}, fmt.Errorf(
			"an online run needs a lookuper; pass one, or set Offline to answer from the cache alone")
	}
	batchSize := options.BatchSize
	if batchSize <= 0 {
		batchSize = DefaultBatchSize
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}

	players, err := store.ListPlayers(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("listing players: %w", err)
	}

	result := Result{Players: len(players)}
	wanted := idsToAsk(players, options.Register, options.Cache, &result)
	slog.Info("player biographies: starting the Wikidata pass",
		slog.Int("players", len(players)),
		slog.Int("with_cricinfo_id", result.WithCricinfoID),
		slog.Int("already_cached", result.FromCache),
		slog.Int("to_ask", len(wanted)),
		slog.Bool("offline", options.Offline))

	if options.Offline {
		result.Unanswered = len(wanted)
		if result.Unanswered > 0 {
			slog.Warn("player biographies: ids the snapshot has no answer for, left unanswered",
				slog.Int("unanswered", result.Unanswered),
				slog.String("fix", "run the online backfill to acquire them"))
		}
	} else if err := askInBatches(ctx, lookuper, options.Cache, wanted, batchSize, &result); err != nil {
		return result, err
	}

	records := buildRecords(players, options, now())
	for i := range records {
		if records[i].Matched() {
			result.Matched++
		}
		if records[i].Source == SourceOverride {
			result.Overridden++
		}
	}
	if err := store.UpsertBiographies(ctx, records); err != nil {
		return result, fmt.Errorf("storing biographies: %w", err)
	}
	slog.Info("player biographies: stored",
		slog.Int("rows", len(records)),
		slog.Int("matched", result.Matched),
		slog.Int("overridden", result.Overridden))
	return result, nil
}

// idsToAsk collects the ESPNcricinfo ids not already answered, deduplicated and ordered so
// two runs batch the same ids together and a resumed run's cache lines up.
func idsToAsk(
	players []Player,
	register map[string]RegisterEntry,
	cache *Cache,
	result *Result,
) []string {
	seen := make(map[string]bool)
	var wanted []string
	for _, player := range players {
		ids := register[player.ExternalID].CricinfoIDs
		if len(ids) > 0 {
			result.WithCricinfoID++
		}
		for _, id := range ids {
			if seen[id] {
				continue
			}
			seen[id] = true
			if _, cached := cache.Get(id); cached {
				result.FromCache++
				continue
			}
			wanted = append(wanted, id)
		}
	}
	sort.Strings(wanted)
	return wanted
}

// askInBatches queries Wikidata for the outstanding ids, caching each batch — including
// its misses — before the next one starts.
func askInBatches(
	ctx context.Context,
	lookuper Lookuper,
	cache *Cache,
	wanted []string,
	batchSize int,
	result *Result,
) error {
	for start := 0; start < len(wanted); start += batchSize {
		end := min(start+batchSize, len(wanted))
		batch := wanted[start:end]

		found, err := lookuper.Query(ctx, batch)
		if err != nil {
			// The cache holds every batch before this one, so the operator's fix is to
			// run the command again rather than to start over.
			return fmt.Errorf("querying Wikidata for ids %d-%d of %d (re-run to resume): %w",
				start, end, len(wanted), err)
		}
		answers := make([]Lookup, 0, len(batch))
		for _, id := range batch {
			if lookup, ok := found[id]; ok {
				lookup.CricinfoID = id
				answers = append(answers, lookup)
				continue
			}
			// A miss is an answer and is cached as one, so the next run does not re-ask.
			answers = append(answers, Lookup{CricinfoID: id})
		}
		if err := cache.Put(answers); err != nil {
			return fmt.Errorf("caching a batch: %w", err)
		}
		result.AskedNow += len(batch)
		slog.Info("player biographies: batch done",
			slog.Int("asked", end), slog.Int("of", len(wanted)), slog.Int("found", len(found)))
	}
	return nil
}

// buildRecords turns cached lookups and curated overrides into one row per player.
func buildRecords(players []Player, options Options, fetchedAt time.Time) []Record {
	records := make([]Record, 0, len(players))
	for _, player := range players {
		record := Record{
			PlayerID:  player.ID,
			Source:    SourceWikidata,
			License:   SourceLicense,
			FetchedAt: fetchedAt,
		}
		for _, id := range options.Register[player.ExternalID].CricinfoIDs {
			lookup, cached := options.Cache.Get(id)
			if record.CricinfoID == "" {
				// The first id is the one the row reports having tried, even when the
				// lookup missed: it is what a curator re-checks by hand.
				record.CricinfoID = id
			}
			if cached && lookup.Found() {
				record.CricinfoID = id
				applyLookup(&record, lookup)
				break
			}
		}
		if override, ok := options.Overrides[player.ExternalID]; ok {
			record = Apply(record, override)
		}
		records = append(records, record)
	}
	return records
}

// applyLookup copies an acquired lookup onto a record, mapping the free-text labels into
// the controlled vocabulary as it goes.
func applyLookup(record *Record, lookup Lookup) {
	record.WikidataQID = lookup.QID
	record.BirthDate = lookup.BirthDate
	record.DeathDate = lookup.DeathDate
	record.CareerEndDate = lookup.CareerEndDate
	record.BattingHand = MapBattingHand(lookup.BattingHandRaw)
	record.BowlingStyle = MapBowlingStyle(lookup.BowlingStyleRaw)
	record.BowlingStyleRaw = lookup.BowlingStyleRaw
}

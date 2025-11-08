// Command backfill-fielding rebuilds fielding_event aggregates into fielding_data.
// Phase 2 scope: iterate matches and recompute aggregates; a future update may
// re-derive events from raw sources and support --force to purge/re-emit events.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

func main() {
	var (
		all       bool
		matchID   int64
		force     bool
		batchSize int
	)
	flag.BoolVar(&all, "all", false, "process all matches found in match_details")
	flag.Int64Var(&matchID, "match", 0, "specific match_id to process (overrides --all if >0)")
	flag.BoolVar(&force, "force", false, "force rebuild (reserved; future: purge/re-derive events)")
	flag.IntVar(&batchSize, "batch-size", 500, "number of matches to process per batch")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if _, err := db.Connect(ctx); err != nil {
		log.Fatalf("db connect failed: %v", err)
	}

	if matchID > 0 {
		if err := backfillOne(ctx, matchID, force); err != nil {
			log.Fatalf("backfill match %d: %v", matchID, err)
		}
		fmt.Printf("ok backfilled match_id=%d\n", matchID)
		return
	}
	if !all {
		fmt.Fprintln(os.Stderr, "either --match <id> or --all must be provided")
		os.Exit(2)
	}

	processed := 0
	offset := 0
	for {
		ids, err := listMatchIDs(ctx, batchSize, offset)
		if err != nil {
			log.Fatalf("list matches: %v", err)
		}
		if len(ids) == 0 {
			break
		}
		for _, id := range ids {
			if err := backfillOne(ctx, id, force); err != nil {
				log.Printf("warn: backfill match %d failed: %v", id, err)
				continue
			}
			processed++
			if processed%50 == 0 {
				log.Printf("progress: %d matches processed", processed)
			}
		}
		offset += len(ids)
	}
	log.Printf("done: %d matches processed", processed)
}

func listMatchIDs(ctx context.Context, limit, offset int) ([]int64, error) {
	rows, err := db.Pool.Query(
		ctx,
		`SELECT match_id FROM match_details WHERE match_id IS NOT NULL ORDER BY match_id LIMIT $1 OFFSET $2`,
		limit,
		offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func backfillOne(ctx context.Context, matchID int64, _ bool) error {
	// Phase 2: We recompute aggregates from existing fielding_event rows.
	// Future: if force is true, we may purge and re-derive fielding_event from sources.
	if err := db.RecomputeFieldingAggregates(ctx, matchID); err != nil {
		return err
	}
	return nil
}

// Command backfill-fielding rebuilds fielding_event aggregates into fielding_data.
// Phase 2 scope: iterate matches and recompute aggregates; a future update may
// re-derive events from raw sources and support --force to purge/re-emit events.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/logger"
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

	logger.SetupFromEnv()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if _, err := db.Connect(ctx); err != nil {
		slog.Error("db connect failed", slog.Any("err", err))
		os.Exit(1)
	}

	if matchID > 0 {
  if err := backfillOne(ctx, matchID, force); err != nil {
		slog.Error("backfill one failed", slog.Int64("match_id", matchID), slog.Any("err", err))
		os.Exit(1)
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
			slog.Error("list matches failed", slog.Any("err", err), slog.Int("offset", offset), slog.Int("batch_size", batchSize))
			os.Exit(1)
		}
		if len(ids) == 0 {
			break
		}
		for _, id := range ids {
   if err := backfillOne(ctx, id, force); err != nil {
				slog.Warn("backfill match failed", slog.Int64("match_id", id), slog.Any("err", err))
				continue
			}
			processed++
			if processed%50 == 0 {
				slog.Info("progress", slog.Int("processed", processed))
			}
		}
		offset += len(ids)
	}
	slog.Info("done", slog.Int("processed", processed))
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

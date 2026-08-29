package precomputefeatures

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// replayEmit hands one work item to the pool. It reports false once the pool is
// shutting down, which is a producer's signal to stop enumerating.
type replayEmit func(item *replayWorkItem) bool

// replayProducer enumerates work items for one format.
type replayProducer func(ctx context.Context, emit replayEmit) error

// replayConsumer processes one work item.
type replayConsumer func(ctx context.Context, item *replayWorkItem) error

// runReplayPool runs the producers and a fixed number of workers over one channel and
// returns the first error either side reported.
//
// Producers and workers share a single errgroup deliberately. Giving the producers a
// group of their own and deriving the workers from *its* context looked like the same
// thing and was not: errgroup.Wait cancels the group's context on success as well as
// on failure, so the instant the last producer finished enumerating, every worker
// still holding a buffered item was cancelled mid-write. The run then failed with
// "context canceled" on the final match of the largest format — deterministically,
// after several minutes of correct work, at what looked like an unrelated player.
//
// One group means cancellation still travels both ways (a failed producer stops the
// workers, a failed worker stops the producers) but nothing is cancelled until every
// side has finished.
func runReplayPool(
	ctx context.Context,
	bufferSize, workers int,
	producers []replayProducer,
	consume replayConsumer,
) error {
	if workers < 1 {
		workers = 1
	}
	if bufferSize < 1 {
		bufferSize = 1
	}

	group, poolCtx := errgroup.WithContext(ctx)
	workCh := make(chan *replayWorkItem, bufferSize)

	emit := func(item *replayWorkItem) bool {
		select {
		case workCh <- item:
			return true
		case <-poolCtx.Done():
			return false
		}
	}

	// Closing the channel belongs to the producers collectively, so it is tracked
	// separately from the group: it has to happen when a producer fails too, or the
	// workers would block on a channel nobody will ever close again.
	var producing sync.WaitGroup
	producing.Add(len(producers))
	for i := range producers {
		produce := producers[i]
		group.Go(func() error {
			defer producing.Done()
			return produce(poolCtx, emit)
		})
	}
	go func() {
		producing.Wait()
		close(workCh)
	}()

	for i := 0; i < workers; i++ {
		group.Go(func() error {
			for item := range workCh {
				// Stop on cancellation rather than pushing every remaining item
				// through a doomed consumer: the group already holds whichever
				// error caused it, and the log would otherwise fill with victims.
				if err := poolCtx.Err(); err != nil {
					return err
				}
				if err := consume(poolCtx, item); err != nil {
					return err
				}
			}
			return nil
		})
	}

	if err := group.Wait(); err != nil {
		return err
	}
	// A cancelled parent means the run did not finish, even when every goroutine
	// happened to return cleanly: producers stop quietly on cancellation by design,
	// and reporting that as success would let a stopped plan march on to its next step.
	return ctx.Err()
}

// matchPlayerProducers builds one producer per format. Each walks its format's matches
// by keyset page and emits one work item per player in each match, so that a format
// that runs out of matches leaves the workers free for the others rather than idle.
func matchPlayerProducers(jobs []FormatJob, pageSize int) []replayProducer {
	producers := make([]replayProducer, 0, len(jobs))
	for i := range jobs {
		job := jobs[i]
		producers = append(producers, func(ctx context.Context, emit replayEmit) error {
			return produceMatchPlayers(ctx, job, pageSize, emit)
		})
	}
	return producers
}

// produceMatchPlayers enumerates one format's (match, player) pairs into the pool.
func produceMatchPlayers(ctx context.Context, job FormatJob, pageSize int, emit replayEmit) error {
	var after *db.MatchLite
	for {
		matches, err := db.ListMatchesByFormatDatePage(ctx, job.FormatID, nil, nil, pageSize, after)
		if err != nil {
			slog.Error("precompute-features(replay-global-pool): list matches failed",
				slog.String("format", job.Code), slog.Int64("format_id", job.FormatID), slog.Any("err", err))
			return fmt.Errorf("list matches %s: %w", job.Code, err)
		}
		if len(matches) == 0 {
			return nil
		}
		for _, match := range matches {
			players, err := db.ListPlayersInMatch(ctx, match.MatchID)
			if err != nil {
				slog.Error("precompute-features(replay-global-pool): list players failed",
					slog.Int64("match_id", match.MatchID), slog.String("format", job.Code), slog.Any("err", err))
				return fmt.Errorf("list players match %d: %w", match.MatchID, err)
			}
			for _, playerID := range players {
				item := &replayWorkItem{
					FormatCode: job.Code,
					FormatID:   job.FormatID,
					Match:      match,
					PlayerID:   playerID,
				}
				if !emit(item) {
					return nil
				}
			}
		}
		after = &matches[len(matches)-1]
	}
}

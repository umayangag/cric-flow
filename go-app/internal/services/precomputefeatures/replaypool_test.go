package precomputefeatures

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// collector records what the consumer saw, so a test can assert on both the items and
// the state of the context they were handed.
type collector struct {
	mu       sync.Mutex
	items    []*replayWorkItem
	ctxErrs  []error
	consumed atomic.Int64
}

func (c *collector) consume(ctx context.Context, item *replayWorkItem) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = append(c.items, item)
	c.ctxErrs = append(c.ctxErrs, ctx.Err())
	c.consumed.Add(1)
	return nil
}

func (c *collector) snapshot() ([]*replayWorkItem, []error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]*replayWorkItem(nil), c.items...), append([]error(nil), c.ctxErrs...)
}

// emitN returns a producer that emits n items and then returns, signalling `done`.
func emitN(t *testing.T, n int, done chan<- struct{}) replayProducer {
	t.Helper()
	return func(_ context.Context, emit replayEmit) error {
		for i := 0; i < n; i++ {
			if !emit(&replayWorkItem{FormatCode: "T20", PlayerID: int64(i)}) {
				break
			}
		}
		close(done)
		return nil
	}
}

// TestRunReplayPool_ProducersFinishFirst_AllItemsProcessed is the regression test for
// the premature cancellation that killed a four-minute precompute run on its last
// match: the workers used to hang off the producers' errgroup context, which
// errgroup.Wait cancels on success, so every buffered item still in flight died the
// moment enumeration finished.
func TestRunReplayPool_ProducersFinishFirst_AllItemsProcessed(t *testing.T) {
	t.Parallel()

	const items = 4
	// Long enough for a group that cancels on success to have done so; the fixed pool
	// simply waits it out, because nothing cancels this context at all.
	const settleForCancellation = 50 * time.Millisecond

	producersDone := make(chan struct{})
	var collected collector
	// No item is touched until every producer has finished, so the run only passes if
	// a finished producer leaves the workers alone. The buffer holds the whole batch,
	// so the producer never needs a worker to drain it.
	consume := func(ctx context.Context, item *replayWorkItem) error {
		<-producersDone
		select {
		case <-ctx.Done():
		case <-time.After(settleForCancellation):
		}
		return collected.consume(ctx, item)
	}

	err := runReplayPool(
		context.Background(),
		items,
		2,
		[]replayProducer{emitN(t, items, producersDone)},
		consume,
	)

	require.NoError(t, err)
	got, ctxErrs := collected.snapshot()
	require.Len(t, got, items, "every produced item must reach a worker")
	for i := range ctxErrs {
		require.NoError(t, ctxErrs[i], "workers must not be handed a cancelled context")
	}
}

func TestRunReplayPool_ConsumerError_ReturnedAndStopsProducers(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("upsert failed")
	var produced atomic.Int64
	// An endless producer: the test passes only if the failing consumer stops it.
	producer := func(_ context.Context, emit replayEmit) error {
		for {
			if !emit(&replayWorkItem{FormatCode: "ODI", PlayerID: produced.Add(1)}) {
				return nil
			}
		}
	}

	err := runReplayPool(context.Background(), 2, 2, []replayProducer{producer},
		func(context.Context, *replayWorkItem) error { return wantErr })

	require.ErrorIs(t, err, wantErr)
}

func TestRunReplayPool_ProducerError_ReturnedNotMaskedByCancellation(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("list matches T20: boom")
	producerFailed := make(chan struct{})
	producer := func(_ context.Context, emit replayEmit) error {
		emit(&replayWorkItem{FormatCode: "T20", PlayerID: 1})
		close(producerFailed)
		return wantErr
	}
	// Holding the consumer until the producer has failed makes the ordering explicit:
	// the error the caller gets must be the producer's, not the "context canceled"
	// the workers see as a consequence of it.
	consume := func(ctx context.Context, _ *replayWorkItem) error {
		<-producerFailed
		return ctx.Err()
	}

	err := runReplayPool(context.Background(), 2, 2, []replayProducer{producer}, consume)

	require.ErrorIs(t, err, wantErr)
}

func TestRunReplayPool_ParentCancelled_ReturnsContextError(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	producer := func(pCtx context.Context, emit replayEmit) error {
		cancel()
		<-pCtx.Done()
		emit(&replayWorkItem{FormatCode: "TEST", PlayerID: 1})
		return nil
	}
	var consumed atomic.Int64

	err := runReplayPool(ctx, 2, 2, []replayProducer{producer},
		func(context.Context, *replayWorkItem) error {
			consumed.Add(1)
			return nil
		})

	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, consumed.Load(), "no work should start once the parent is cancelled")
}

func TestRunReplayPool_MultipleProducers_AllItemsProcessedOnce(t *testing.T) {
	t.Parallel()

	const perProducer = 25
	formats := []string{"TEST", "ODI", "T20", "T20I"}
	producers := make([]replayProducer, 0, len(formats))
	for i := range formats {
		code := formats[i]
		producers = append(producers, func(_ context.Context, emit replayEmit) error {
			for n := 0; n < perProducer; n++ {
				if !emit(&replayWorkItem{FormatCode: code, PlayerID: int64(n)}) {
					return nil
				}
			}
			return nil
		})
	}
	var collected collector

	err := runReplayPool(context.Background(), 6, 3, producers, collected.consume)

	require.NoError(t, err)
	got, _ := collected.snapshot()
	require.Len(t, got, len(formats)*perProducer)
	perFormat := map[string]int{}
	for i := range got {
		perFormat[got[i].FormatCode]++
	}
	require.Equal(t, map[string]int{"TEST": perProducer, "ODI": perProducer, "T20": perProducer, "T20I": perProducer}, perFormat)
}

func TestRunReplayPool_NonPositiveSizes_StillRun(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		bufferSize int
		workers    int
	}{
		{"zero buffer and workers", 0, 0},
		{"negative buffer and workers", -1, -1},
		{"single worker", 1, 1},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var collected collector
			producer := func(_ context.Context, emit replayEmit) error {
				emit(&replayWorkItem{FormatCode: "T20", PlayerID: 7})
				return nil
			}

			err := runReplayPool(context.Background(), tc.bufferSize, tc.workers,
				[]replayProducer{producer}, collected.consume)

			require.NoError(t, err)
			require.Equal(t, int64(1), collected.consumed.Load())
		})
	}
}

func TestRunReplayPool_NoProducers_ReturnsNil(t *testing.T) {
	t.Parallel()
	var collected collector

	err := runReplayPool(context.Background(), 2, 2, nil, collected.consume)

	require.NoError(t, err)
	require.Zero(t, collected.consumed.Load())
}

func TestMatchPlayerProducers_OnePerJob(t *testing.T) {
	t.Parallel()

	jobs := []FormatJob{{Code: "T20", FormatID: 1}, {Code: "ODI", FormatID: 2}}

	got := matchPlayerProducers(jobs, 500)

	require.Len(t, got, len(jobs))
	require.Empty(t, matchPlayerProducers(nil, 500))
}

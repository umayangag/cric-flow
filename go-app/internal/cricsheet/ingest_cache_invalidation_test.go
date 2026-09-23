package cricsheet_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/cricsheet"
	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// fixedPlayerIDPool answers every player lookup with the same (id, name), whatever SQL it
// is asked -- enough to drive db.EntityCache.GetPlayerID through GetOrCreatePlayer's
// external-id path without a database.
type fixedPlayerIDPool struct {
	id   int64
	name string
}

func (p fixedPlayerIDPool) Exec(_ context.Context, _ string, _ ...any) error { return nil }

func (p fixedPlayerIDPool) Query(_ context.Context, _ string, _ ...any) (db.Rows, error) {
	return nopRows{}, nil
}

func (p fixedPlayerIDPool) QueryRow(_ context.Context, _ string, _ ...any) db.Row {
	return fixedPlayerIDRow(p)
}

func (p fixedPlayerIDPool) Begin(_ context.Context) (db.CopyFromTx, error) {
	return nopTx{}, nil
}

type fixedPlayerIDRow struct {
	id   int64
	name string
}

func (r fixedPlayerIDRow) Scan(dest ...any) error {
	if len(dest) > 0 {
		if p, ok := dest[0].(*int64); ok {
			*p = r.id
		}
	}
	if len(dest) > 1 {
		if p, ok := dest[1].(*string); ok {
			*p = r.name
		}
	}
	return nil
}

// TestImportDir_ClearsTheGlobalCacheBeforeDoingAnythingElse pins IMPORT-16 at the call
// site that matters: go-app's own API server reaches ImportDir more than once across its
// own lifetime (the pipeline's import step, internal/server/handlers.go and
// step_work.go), so the process-global db.EntityCache can hold an id memoised before a
// dev-destroy or a re-migrate ran between two such calls. That id must not survive into
// the next run.
//
// An empty directory still fails ImportDir with ErrNoMatchFiles, but only after the
// clear -- proof enough that no file needs to actually name the player for this to show
// the fix runs unconditionally, first.
func TestImportDir_ClearsTheGlobalCacheBeforeDoingAnythingElse(t *testing.T) {
	// Not parallel: uses package-level singletons (db.PoolAPI, db.GetGlobalCache()).
	ctx := context.Background()
	cache := db.GetGlobalCache()
	prevPool := db.PoolAPI
	t.Cleanup(func() {
		db.SetPoolAPI(prevPool)
		cache.Clear()
	})

	// Memoise an id the way a run before a dev-destroy / re-migrate would have.
	db.SetPoolAPI(fixedPlayerIDPool{id: 999, name: "Someone"})
	staleID, err := cache.GetPlayerID(ctx, "abc12345", "Someone", "2020-01-01")
	require.NoError(t, err)
	require.Equal(t, int64(999), staleID)

	// The schema behind it changed; the same identifier now resolves to a different row.
	db.SetPoolAPI(fixedPlayerIDPool{id: 5, name: "Someone"})

	_, err = cricsheet.ImportDir(ctx, t.TempDir(), &cricsheet.Options{}, 1)
	require.ErrorIs(t, err, cricsheet.ErrNoMatchFiles)

	freshID, err := cache.GetPlayerID(ctx, "abc12345", "Someone", "2020-01-01")
	require.NoError(t, err)
	require.Equal(t, int64(5), freshID,
		"ImportDir must clear the cache before it does anything else, so an id it "+
			"resolves is asked fresh rather than served from before the schema changed")
}

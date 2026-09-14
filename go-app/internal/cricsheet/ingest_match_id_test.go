package cricsheet_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/cricsheet"
	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// importFileNamed runs one match through the importer under a chosen file name, and
// returns the match ids the squad insert carried. The name is the point: every case here
// imports the same match and varies only what the file is called, because that is now
// where the match id comes from.
func importFileNamed(t *testing.T, name string) []int64 {
	t.Helper()
	prevPool := db.PoolAPI
	db.SetPoolAPI(nopPool{})
	t.Cleanup(func() { db.SetPoolAPI(prevPool) })

	spy := &squadSpyTx{}
	cricsheet.SetRunInTxFn(func(ctx context.Context, inner func(context.Context, db.CopyFromTx) error) error {
		return inner(ctx, spy)
	})
	t.Cleanup(func() { cricsheet.SetRunInTxFn(nil) })

	file := writeTempJSON(t, t.TempDir(), name, squadJSON)
	require.NoError(t, cricsheet.ImportMatchFile(context.Background(), file, &cricsheet.Options{}))

	// match_player rows are (match_id, player_id, opposition_id, is_replacement) quads.
	ids := make([]int64, 0, len(spy.insertArgs)/matchPlayerInsertArgs)
	for i := 0; i < len(spy.insertArgs); i += matchPlayerInsertArgs {
		ids = append(ids, spy.insertArgs[i].(int64))
	}
	return ids
}

func TestImportMatchFile_TakesTheMatchIDFromTheFileName(t *testing.T) {
	// Not parallel: uses package-level singletons (db.PoolAPI, SetRunInTxFn).
	// Arrange + Act
	ids := importFileNamed(t, "1130677.json")

	// Assert
	require.NotEmpty(t, ids)
	for _, id := range ids {
		assert.Equal(t, int64(1130677), id, "Cricsheet's own match id is the identity")
	}
}

func TestImportMatchFile_TwoFilesWithOneDateAndOnePairOfSidesAreTwoMatches(t *testing.T) {
	// Not parallel: uses package-level singletons (db.PoolAPI, SetRunInTxFn).
	//
	// The regression this guards. 309 of the 22,734 files in the dataset share a
	// (date, team, team) with another file; keyed on that content they shared a match id,
	// so 309 matches had no row of their own and 6,223 deliveries were filed under a match
	// they did not belong to.
	// Arrange + Act
	first := importFileNamed(t, "1452624.json")
	second := importFileNamed(t, "1452625.json")

	// Assert
	require.NotEmpty(t, first)
	require.NotEmpty(t, second)
	assert.NotEqual(t, first[0], second[0], "same date, same sides, different match")
}

func TestImportMatchFile_AFileNameThatIsNotACricsheetIDStillImports(t *testing.T) {
	// Not parallel: uses package-level singletons (db.PoolAPI, SetRunInTxFn).
	//
	// 25 files are named "wi_211824" and cannot be a bigint. They fall back to a derived
	// id rather than failing: a name this importer did not expect is not a reason to lose
	// a match's ball-by-ball record.
	// Arrange + Act
	ids := importFileNamed(t, "wi_211824.json")

	// Assert
	require.NotEmpty(t, ids)
	assert.GreaterOrEqual(t, ids[0], int64(100000000000), "derived ids sit above every Cricsheet id")
}

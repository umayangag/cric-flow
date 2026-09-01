package cricsheet_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-flow/go-app/internal/cricsheet"
	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// fieldingJSON has two caught dismissals: one taken by a named fielder, one by a
// substitute Cricsheet could not name -- 469 such entries in the archive.
const fieldingJSON = `{
  "info": {
    "balls_per_over": 6,
    "dates": ["2024-01-02"],
    "match_type": "T20",
    "teams": ["Alpha", "Beta"],
    "season": "2024",
    "players": {"Alpha": ["A1", "A2", "A3"], "Beta": ["B1", "B2", "B3"]}
  },
  "innings": [
    {"team":"Alpha","overs":[
      {"over":1,"deliveries":[
        {"batter":"A1","bowler":"B1","non_striker":"A2","runs":{"batter":0,"extras":0,"total":0},
         "wickets":[{"player_out":"A1","kind":"caught","fielders":[{"name":"B2"}]}]},
        {"batter":"A3","bowler":"B1","non_striker":"A2","runs":{"batter":0,"extras":0,"total":0},
         "wickets":[{"player_out":"A3","kind":"caught","fielders":[{"substitute":true}]}]}
      ]}
    ]}
  ]
}`

// fieldingSpyTx records what the importer writes to fielding_event.
type fieldingSpyTx struct {
	deletes int
	kinds   []string
}

func (t *fieldingSpyTx) Exec(_ context.Context, sql string, _ ...any) error {
	if strings.Contains(sql, "DELETE FROM fielding_event") {
		t.deletes++
	}
	return nil
}

func (t *fieldingSpyTx) Query(_ context.Context, _ string, _ ...any) (db.Rows, error) {
	return nopRows{}, nil
}
func (t *fieldingSpyTx) QueryRow(_ context.Context, _ string, _ ...any) db.Row { return nopRow{} }

func (t *fieldingSpyTx) CopyFrom(
	_ context.Context,
	table pgx.Identifier,
	columns []string,
	src pgx.CopyFromSource,
) (int64, error) {
	n := int64(0)
	for src.Next() {
		n++
		if table.Sanitize() != `"fielding_event_tmp"` {
			continue
		}
		values, err := src.Values()
		if err != nil {
			return n, err
		}
		for i, column := range columns {
			if column == "kind" {
				t.kinds = append(t.kinds, values[i].(string))
			}
		}
	}
	return n, src.Err()
}

func (t *fieldingSpyTx) Commit(_ context.Context) error   { return nil }
func (t *fieldingSpyTx) Rollback(_ context.Context) error { return nil }

func TestImportMatchFile_UnnamedSubstituteCatch_IsCreditedToNobody(t *testing.T) {
	// Not parallel: uses package-level singletons (db.PoolAPI, SetRunInTxFn).
	prevPool := db.PoolAPI
	db.SetPoolAPI(nopPool{})
	t.Cleanup(func() { db.SetPoolAPI(prevPool) })
	spy := &fieldingSpyTx{}
	cricsheet.SetRunInTxFn(func(ctx context.Context, inner func(context.Context, db.CopyFromTx) error) error {
		return inner(ctx, spy)
	})
	t.Cleanup(func() { cricsheet.SetRunInTxFn(nil) })
	file := writeTempJSON(t, t.TempDir(), "m.json", fieldingJSON)

	err := cricsheet.ImportMatchFile(context.Background(), file, &cricsheet.Options{})

	require.NoError(t, err)
	assert.Equal(t, 1, spy.deletes, "the match's previous fielding events are replaced, not accumulated")
	// One catch by B2; the substitute's catch is nobody's -- it used to be the bowler's.
	assert.Equal(t, []string{"caught"}, spy.kinds)
}

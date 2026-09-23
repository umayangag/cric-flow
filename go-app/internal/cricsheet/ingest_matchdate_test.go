package cricsheet_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/cricsheet"
)

// undatedMatchJSON names no date at all: no info.dates, no info.match_date. The old
// default placed a file like this at 1970-01-01 -- before every real match in the
// archive, and so the as-of clock every rating and serving decision reads a player's
// history against.
const undatedMatchJSON = `{
  "info": {
    "balls_per_over": 6,
    "match_type": "T20",
    "team_type": "club",
    "gender": "male",
    "teams": ["Alpha", "Beta"]
  },
  "innings": [
    {"team":"Alpha","overs":[
      {"over":0,"deliveries":[
        {"batter":"A1","bowler":"B1","non_striker":"A2","runs":{"batter":1,"extras":0,"total":1}}
      ]}
    ]}
  ]
}`

// TestImportMatchFile_NoDate_IsRefused pins IMPORT-17: a file with no date fails the
// import rather than being silently placed at 1970-01-01.
func TestImportMatchFile_NoDate_IsRefused(t *testing.T) {
	// Not parallel: uses package-level singletons (db.PoolAPI, SetRunInTxFn).
	ctx := context.Background()
	spy := arrangeMatchInningSpy(t)
	file := writeJSON(t, t.TempDir(), "9000400.json", undatedMatchJSON)

	err := cricsheet.ImportMatchFile(ctx, file, &cricsheet.Options{})

	require.Error(t, err, "a file naming no date must fail the import, not default to 1970-01-01")
	require.Empty(t, spy.innings, "nothing should have reached match_inning for a file that never resolved a date")
}

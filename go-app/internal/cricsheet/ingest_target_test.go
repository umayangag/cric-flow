package cricsheet_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/cricsheet"
	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// matchFileForTarget is a two-innings match of one over a side, parameterised on the match
// type and on the `target` object Cricsheet writes on the second innings. The first innings
// is worth 11 runs (4 + 1 wide + 6), so a chase with no stated target needs 12.
//
// targetJSON is spliced into the second innings verbatim; "" leaves the key out, which is
// how 4,641 of the archive's 22,905 files stand.
func matchFileForTarget(matchType, targetJSON string) string {
	target := ""
	if targetJSON != "" {
		target = `"target":` + targetJSON + `,`
	}
	return `{
      "info": {
        "balls_per_over": 6,
        "dates": ["2024-06-04"],
        "match_type": "` + matchType + `",
        "team_type": "club",
        "gender": "male",
        "teams": ["Alpha", "Beta"],
        "venue": "The Oval",
        "city": "Metropolis",
        "season": "2024"
      },
      "innings": [
        {"team":"Alpha","overs":[
          {"over":0,"deliveries":[
            {"batter":"A1","bowler":"B1","non_striker":"A2","runs":{"batter":4,"extras":0,"total":4}},
            {"batter":"A1","bowler":"B1","non_striker":"A2","runs":{"batter":0,"extras":1,"total":1},"extras":{"wides":1}},
            {"batter":"A2","bowler":"B1","non_striker":"A1","runs":{"batter":6,"extras":0,"total":6}}
          ]}
        ]},
        {"team":"Beta",` + target + `"overs":[
          {"over":0,"deliveries":[
            {"batter":"B1","bowler":"A1","non_striker":"B2","runs":{"batter":1,"extras":0,"total":1}}
          ]}
        ]}
      ]
    }`
}

// TestImportMatchFile_SecondInningsTargetIsWhatTheChaseNeeded pins IMPORT-11.
//
// Three failures in one line of the importer: the target was the first innings' runs rather
// than the runs that win (all 19,432 limited-overs second innings in the archive were stored
// one short), Cricsheet's own `innings[].target` was never read (so every one of the 983
// rain-revised targets was replaced by the unrevised total, and the over limit was thrown
// away), and a target was written onto the 3,102 second innings of Tests and first-class
// matches, which are not chases.
func TestImportMatchFile_SecondInningsTargetIsWhatTheChaseNeeded(t *testing.T) {
	// Not parallel: uses package-level singletons (db.PoolAPI, SetRunInTxFn).
	testCases := []struct {
		name       string
		matchType  string
		targetJSON string
		wantTarget string
	}{
		{
			name:       "a chase whose file states no target needs one more than the innings defended",
			matchType:  "T20",
			targetJSON: "",
			wantTarget: "12 runs, no stated over limit",
		},
		{
			name:       "an unrevised target is taken from the file, not derived",
			matchType:  "ODI",
			targetJSON: `{"runs":12,"overs":50}`,
			wantTarget: "12 runs in 50 overs",
		},
		{
			name:       "a rain-revised target keeps the revised runs and the shortened over limit",
			matchType:  "ODI",
			targetJSON: `{"runs":235,"overs":42}`,
			wantTarget: "235 runs in 42 overs",
		},
		{
			name:       "a revised over limit in the scorer's O.B notation survives as a fraction",
			matchType:  "T20",
			targetJSON: `{"runs":97,"overs":12.4}`,
			wantTarget: "97 runs in 12.4 overs",
		},
		{
			name:       "a Test second innings is not a chase and carries no target",
			matchType:  "Test",
			targetJSON: "",
			wantTarget: "no target",
		},
		{
			name:       "a first-class second innings is not a chase either",
			matchType:  "MDM",
			targetJSON: "",
			wantTarget: "no target",
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			ctx := context.Background()
			spy := arrangeMatchInningSpy(t)
			file := writeJSON(
				t,
				t.TempDir(),
				"900011"+strconv.Itoa(i)+".json",
				matchFileForTarget(tc.matchType, tc.targetJSON),
			)

			// Act
			err := cricsheet.ImportMatchFile(ctx, file, &cricsheet.Options{})

			// Assert
			require.NoError(t, err)
			require.Len(t, spy.innings, 2, "both innings reach the match_inning upsert")
			assert.Equal(t, tc.wantTarget, describeTarget(spy.innings[1]))
		})
	}
}

// TestImportMatchFile_FirstInningsNeverCarriesATarget: nobody is chasing while the first
// innings is being batted, whatever the format.
func TestImportMatchFile_FirstInningsNeverCarriesATarget(t *testing.T) {
	// Not parallel: uses package-level singletons (db.PoolAPI, SetRunInTxFn).
	testCases := []struct {
		name      string
		matchType string
	}{
		{name: "limited overs", matchType: "T20"},
		{name: "multi-day", matchType: "Test"},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			ctx := context.Background()
			spy := arrangeMatchInningSpy(t)
			file := writeJSON(
				t,
				t.TempDir(),
				"900012"+strconv.Itoa(i)+".json",
				matchFileForTarget(tc.matchType, `{"runs":12,"overs":20}`),
			)

			// Act
			err := cricsheet.ImportMatchFile(ctx, file, &cricsheet.Options{})

			// Assert
			require.NoError(t, err)
			require.Len(t, spy.innings, 2, "both innings reach the match_inning upsert")
			assert.Equal(t, "no target", describeTarget(spy.innings[0]))
		})
	}
}

// describeTarget renders an innings' stored target as one readable line, so a failure says
// which of the runs and the over limit is wrong rather than printing two pointer addresses.
func describeTarget(mi db.MatchInningInsert) string {
	if mi.TargetRuns == nil {
		return "no target"
	}
	if mi.TargetOvers == nil {
		return strconv.Itoa(*mi.TargetRuns) + " runs, no stated over limit"
	}
	overs := strconv.FormatFloat(float64(*mi.TargetOvers), 'g', -1, 32)
	return strconv.Itoa(*mi.TargetRuns) + " runs in " + overs + " overs"
}

// arrangeMatchInningSpy routes the import's transaction through a spy that captures every
// match_inning upsert, with no database behind it.
func arrangeMatchInningSpy(t *testing.T) *offlineSpyTx {
	t.Helper()
	prevPool := db.PoolAPI
	db.SetPoolAPI(nopPool{})
	t.Cleanup(func() { db.SetPoolAPI(prevPool) })
	spy := &offlineSpyTx{}
	cricsheet.SetRunInTxFn(
		func(ctx context.Context, inner func(context.Context, db.CopyFromTx) error) error {
			return inner(ctx, spy)
		},
	)
	t.Cleanup(func() { cricsheet.SetRunInTxFn(nil) })
	return spy
}

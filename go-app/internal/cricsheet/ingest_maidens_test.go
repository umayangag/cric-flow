package cricsheet_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/cricsheet"
)

// maidensFixtureJSON is one bowler ("B1") bowling two overs in the first innings, one over
// per IMPORT-14 sub-defect:
//
//   - Over 0 is six legal dot balls plus a seventh delivery that is a wide conceding one run.
//     The over is complete (six legal balls), but a wide is not one of the bowler's six and
//     was dropped from the maiden accumulation entirely -- so a wide-conceded run never broke
//     a maiden.
//   - Over 1 is three legal dot balls and then the innings ends: a partial over, not a
//     complete one, which the old rule still counted as a maiden because it only looked at
//     whether the over's tracked runs were zero, never at how many balls were bowled.
//
// A second, one-ball innings closes out the file; it is not part of what this test asserts.
const maidensFixtureJSON = `{
  "info": {
    "balls_per_over": 6,
    "dates": ["2024-03-01"],
    "match_type": "T20",
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
        {"batter":"A1","bowler":"B1","non_striker":"A2","runs":{"batter":0,"extras":0,"total":0}},
        {"batter":"A1","bowler":"B1","non_striker":"A2","runs":{"batter":0,"extras":1,"total":1},"extras":{"wides":1}},
        {"batter":"A1","bowler":"B1","non_striker":"A2","runs":{"batter":0,"extras":0,"total":0}},
        {"batter":"A2","bowler":"B1","non_striker":"A1","runs":{"batter":0,"extras":0,"total":0}},
        {"batter":"A2","bowler":"B1","non_striker":"A1","runs":{"batter":0,"extras":0,"total":0}},
        {"batter":"A1","bowler":"B1","non_striker":"A2","runs":{"batter":0,"extras":0,"total":0}},
        {"batter":"A1","bowler":"B1","non_striker":"A2","runs":{"batter":0,"extras":0,"total":0}}
      ]},
      {"over":1,"deliveries":[
        {"batter":"A2","bowler":"B1","non_striker":"A1","runs":{"batter":0,"extras":0,"total":0}},
        {"batter":"A2","bowler":"B1","non_striker":"A1","runs":{"batter":0,"extras":0,"total":0}},
        {"batter":"A2","bowler":"B1","non_striker":"A1","runs":{"batter":0,"extras":0,"total":0}}
      ]}
    ]},
    {"team":"Beta","overs":[
      {"over":0,"deliveries":[
        {"batter":"B1","bowler":"A1","non_striker":"B2","runs":{"batter":1,"extras":0,"total":1}}
      ]}
    ]}
  ]
}`

// TestImportMatchFile_MaidenOvers_WidesAndPartialOvers pins IMPORT-14: wides did not break
// a maiden and a partial over counted as a complete one. Both fixtures wrongly earn bowler
// "B1" a maiden on main (the wide-conceded over because the wide run never reached the
// accumulation, the three-ball over because completeness was never checked), so the file
// reads two maidens where there are none; after the fix it reads zero.
func TestImportMatchFile_MaidenOvers_WidesAndPartialOvers(t *testing.T) {
	// Not parallel: uses package-level singletons (db.PoolAPI, SetRunInTxFn).
	ctx := context.Background()
	spy := arrangeMatchInningSpy(t)
	file := writeJSON(t, t.TempDir(), "9000200.json", maidensFixtureJSON)

	err := cricsheet.ImportMatchFile(ctx, file, &cricsheet.Options{})

	require.NoError(t, err)
	require.NotEmpty(t, spy.bowlingMaidens, "expected a bowling_data row to have been copied")
	require.Equal(t, 0, *spy.bowlingMaidens[0],
		"bowler B1 should have no maidens: the wide-conceded over conceded a run and "+
			"the three-ball over was never completed")
}

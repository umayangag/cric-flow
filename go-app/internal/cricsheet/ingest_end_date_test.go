package cricsheet_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/cricsheet"
)

// FEAT-09: a match played over several days is written with the day it ended beside the
// day it started, so the rating pass can fold it in at the close of its last day rather
// than its first. Before this the upsert carried only dates[0].
func TestImportMatchFile_WritesTheDayTheMatchEnded(t *testing.T) {
	// Not parallel: uses package-level singletons (db.PoolAPI, SetRunInTxFn).
	testCases := []struct {
		name        string
		dates       string
		wantStart   string
		wantEndDate string
	}{
		{
			name:        "a match on one day ends on it",
			dates:       `["2024-06-04"]`,
			wantStart:   "2024-06-04",
			wantEndDate: "2024-06-04",
		},
		{
			name:        "a Test ends on its last listed day",
			dates:       `["2024-06-04", "2024-06-05", "2024-06-06", "2024-06-07", "2024-06-08"]`,
			wantStart:   "2024-06-04",
			wantEndDate: "2024-06-08",
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			ctx := context.Background()
			spy := arrangeMatchUpsertSpy(t)
			content := strings.Replace(
				matchFileForCompetition("MDM", "club", "Victoria", "Tasmania", 0),
				`"dates": ["2024-06-04"]`,
				`"dates": `+tc.dates,
				1,
			)
			file := writeJSON(t, t.TempDir(), "9000031.json", content)

			// Act
			err := cricsheet.ImportMatchFile(ctx, file, &cricsheet.Options{})

			// Assert
			require.NoError(t, err)
			require.Len(t, spy.matchArgs, matchArgCount, "the match upsert carries the match's last day")
			assert.Equal(t, tc.wantStart, spy.matchArgs[2], "match_date stays the first day")
			assert.Equal(t, tc.wantEndDate, spy.matchArgs[matchArgMatchEndDate])
		})
	}
}

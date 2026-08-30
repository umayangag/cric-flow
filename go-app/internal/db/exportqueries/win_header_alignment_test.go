package exportqueries

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// zeroScanner satisfies the scanner interface without writing anything, so the row
// builder runs on zero values. Width is what this file is about, not content.
type zeroScanner struct{}

func (zeroScanner) Scan(...any) error { return nil }

// TestWinExportRowMatchesItsHeader is the guard the win export never had.
//
// The scanner emitted eight values the header did not name: a second, Unix-formatted
// match_date and seven weather zeros left behind when the weather features were removed.
// A CSV whose rows are wider than its header is not rejected by pandas — it consumes the
// surplus leading columns as an index — so the win training set mapped its columns eight
// places out of position, and team1_wins, the target, was read from a weather zero.
//
// Nothing failed. The export succeeded, training succeeded, and the model learned that
// team1 never wins.
func TestWinExportRowMatchesItsHeader(t *testing.T) {
	t.Parallel()

	// Arrange / Act
	headers := winEnhancedHeaders()
	row, err := scanWinEnhancedRow(zeroScanner{})

	// Assert
	require.NoError(t, err)
	assert.Equal(t, len(headers), len(row),
		"the win export writes %d values under %d column names", len(row), len(headers))
}

// TestWinExportHeaderShape pins the two halves of the header so a feature group added
// to one is not silently absent from the other.
func TestWinExportHeaderShape(t *testing.T) {
	t.Parallel()

	// Arrange / Act
	headers := winEnhancedHeaders()

	// Assert
	require.Len(t, headers, len(winBaseHeaders)+len(winFeatureGroupNames)*len(winDistStatSuffixes))
	assert.Equal(t, winBaseHeaders, headers[:len(winBaseHeaders)])
	assert.Contains(t, headers, "team1_wins", "the training target must be a named column")
	assert.NotContains(t, headers, "toss_winner_opposition_id",
		"the toss is not known when a side is selected, so it is not a win feature")
}

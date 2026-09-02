package server

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/umayangag/cric-flow/go-app/internal/tracking"
)

func TestGenerateSuggestions(t *testing.T) {
	now := time.Now()
	hourAgo := now.Add(-1 * time.Hour)
	twoHoursAgo := now.Add(-2 * time.Hour)
	threeHoursAgo := now.Add(-3 * time.Hour)

	testCases := []struct {
		name           string
		migrations     []tracking.Migration
		seqPopulated   bool
		expectedTitles []string
	}{
		{
			name:         "No migrations",
			migrations:   []tracking.Migration{},
			seqPopulated: false, // Ignored because no import
			expectedTitles: []string{
				"Initialize Data",
			},
		},
		{
			name: "Import done, no precompute, no seq data",
			migrations: []tracking.Migration{
				{Command: "cricsheet-import", Status: tracking.StatusCompleted, StartedAt: now},
			},
			seqPopulated: false,
			expectedTitles: []string{
				"Fix Missing Features",
			},
		},
		{
			name: "Import done, no precompute, seq data somehow exists (unlikely but possible)",
			migrations: []tracking.Migration{
				{Command: "cricsheet-import", Status: tracking.StatusCompleted, StartedAt: now},
			},
			seqPopulated: true,
			expectedTitles: []string{
				"Run Full Precompute",
			},
		},
		{
			name: "Import done, precompute old, seq data ok",
			migrations: []tracking.Migration{
				{Command: "cricsheet-import", Status: tracking.StatusCompleted, StartedAt: now},
				{Command: "precompute-features", Status: tracking.StatusCompleted, StartedAt: hourAgo},
			},
			seqPopulated: true,
			expectedTitles: []string{
				"Run Full Precompute",
			},
		},
		{
			name: "Precompute done, export old, seq data missing",
			migrations: []tracking.Migration{
				{Command: "precompute-features", Status: tracking.StatusCompleted, StartedAt: now},
				{Command: "cricsheet-import", Status: tracking.StatusCompleted, StartedAt: hourAgo},
				{Command: "export-dataset", Status: tracking.StatusCompleted, StartedAt: hourAgo},
			},
			seqPopulated: false,
			expectedTitles: []string{
				"Fix Missing Features",
			},
		},
		{
			name: "Precompute done, export old, seq data ok",
			migrations: []tracking.Migration{
				{Command: "precompute-features", Status: tracking.StatusCompleted, StartedAt: now},
				{Command: "cricsheet-import", Status: tracking.StatusCompleted, StartedAt: hourAgo},
				{Command: "export-dataset", Status: tracking.StatusCompleted, StartedAt: hourAgo},
			},
			seqPopulated: true,
			expectedTitles: []string{
				"Export Dataset",
			},
		},
		{
			name: "Export done, train old",
			migrations: []tracking.Migration{
				{Command: "export-dataset", Status: tracking.StatusCompleted, StartedAt: now},
				{Command: "precompute-features", Status: tracking.StatusCompleted, StartedAt: hourAgo},
				{Command: "cricsheet-import", Status: tracking.StatusCompleted, StartedAt: twoHoursAgo},
				{Command: "train-win", Status: tracking.StatusCompleted, StartedAt: twoHoursAgo},
			},
			seqPopulated: true,
			expectedTitles: []string{
				"Train Win Model",
			},
		},
		{
			name: "Conflict: Import new, Precompute old, Export older",
			migrations: []tracking.Migration{
				{Command: "cricsheet-import", Status: tracking.StatusCompleted, StartedAt: now},
				{Command: "precompute-features", Status: tracking.StatusCompleted, StartedAt: hourAgo},
				{Command: "export-dataset", Status: tracking.StatusCompleted, StartedAt: twoHoursAgo},
			},
			seqPopulated: true,
			// Current logic would suggest Precompute AND Export.
			// Desired: Only Precompute.
			expectedTitles: []string{
				"Run Full Precompute",
			},
		},
		{
			name: "Conflict: Precompute new, Export old, Train older",
			migrations: []tracking.Migration{
				{Command: "precompute-features", Status: tracking.StatusCompleted, StartedAt: now},
				{Command: "cricsheet-import", Status: tracking.StatusCompleted, StartedAt: hourAgo},
				{Command: "export-dataset", Status: tracking.StatusCompleted, StartedAt: hourAgo},
				{Command: "train-win", Status: tracking.StatusCompleted, StartedAt: twoHoursAgo},
			},
			seqPopulated: true,
			// Current logic would suggest Export AND Train.
			// Desired: Only Export.
			expectedTitles: []string{
				"Export Dataset",
			},
		},
		{
			name: "Everything up to date",
			migrations: []tracking.Migration{
				{Command: "ml-auto-tune", Status: tracking.StatusCompleted, StartedAt: now},
				{Command: "train-win", Status: tracking.StatusCompleted, StartedAt: now},
				{Command: "export-dataset", Status: tracking.StatusCompleted, StartedAt: hourAgo},
				{Command: "precompute-features", Status: tracking.StatusCompleted, StartedAt: twoHoursAgo},
				{Command: "cricsheet-import", Status: tracking.StatusCompleted, StartedAt: threeHoursAgo},
			},
			seqPopulated:   true,
			expectedTitles: []string{},
		},
		{
			name: "Failed Import (should be ignored)",
			migrations: []tracking.Migration{
				{
					Command:   "cricsheet-import",
					Status:    tracking.StatusFailed,
					StartedAt: hourAgo,
				},
			},
			seqPopulated: false,
			// Behaves like no migrations
			expectedTitles: []string{
				"Initialize Data",
			},
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			suggestions := GenerateSuggestions(tc.migrations, tc.seqPopulated)
			titles := make([]string, 0, len(suggestions))
			for _, s := range suggestions {
				titles = append(titles, s.Title)
			}
			assert.ElementsMatch(t, tc.expectedTitles, titles)
		})
	}
}

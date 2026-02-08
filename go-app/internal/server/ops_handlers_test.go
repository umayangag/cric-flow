package server

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/tracking"
)

func TestGenerateSuggestions(t *testing.T) {
	now := time.Now()
	hourAgo := now.Add(-1 * time.Hour)
	twoHoursAgo := now.Add(-2 * time.Hour)
	threeHoursAgo := now.Add(-3 * time.Hour)

	tests := []struct {
		name           string
		migrations     []tracking.Migration
		expectedTitles []string
	}{
		{
			name:       "No migrations",
			migrations: []tracking.Migration{},
			expectedTitles: []string{
				"Initialize Data",
			},
		},
		{
			name: "Import done, no precompute",
			migrations: []tracking.Migration{
				{Command: "cricsheet-import", Status: tracking.StatusCompleted, StartedAt: now},
			},
			expectedTitles: []string{
				"Run Precompute",
			},
		},
		{
			name: "Import done, precompute old",
			migrations: []tracking.Migration{
				{Command: "cricsheet-import", Status: tracking.StatusCompleted, StartedAt: now},
				{Command: "precompute-features", Status: tracking.StatusCompleted, StartedAt: hourAgo},
			},
			expectedTitles: []string{
				"Run Precompute",
			},
		},
		{
			name: "Precompute done, export old",
			migrations: []tracking.Migration{
				{Command: "precompute-features", Status: tracking.StatusCompleted, StartedAt: now},
				{Command: "cricsheet-import", Status: tracking.StatusCompleted, StartedAt: hourAgo},
				{Command: "export-dataset", Status: tracking.StatusCompleted, StartedAt: hourAgo},
			},
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
				{Command: "train-batting", Status: tracking.StatusCompleted, StartedAt: hourAgo},
				{Command: "train-bowling", Status: tracking.StatusCompleted, StartedAt: hourAgo},
			},
			expectedTitles: []string{
				"Train Batting Model",
				"Train Bowling Model",
			},
		},
		{
			name: "Conflict: Import new, Precompute old, Export older",
			migrations: []tracking.Migration{
				{Command: "cricsheet-import", Status: tracking.StatusCompleted, StartedAt: now},
				{Command: "precompute-features", Status: tracking.StatusCompleted, StartedAt: hourAgo},
				{Command: "export-dataset", Status: tracking.StatusCompleted, StartedAt: twoHoursAgo},
			},
			// Current logic would suggest Precompute AND Export.
			// Desired: Only Precompute.
			expectedTitles: []string{
				"Run Precompute",
			},
		},
		{
			name: "Conflict: Precompute new, Export old, Train older",
			migrations: []tracking.Migration{
				{Command: "precompute-features", Status: tracking.StatusCompleted, StartedAt: now},
				{Command: "cricsheet-import", Status: tracking.StatusCompleted, StartedAt: hourAgo},
				{Command: "export-dataset", Status: tracking.StatusCompleted, StartedAt: hourAgo},
				{Command: "train-batting", Status: tracking.StatusCompleted, StartedAt: twoHoursAgo},
			},
			// Current logic would suggest Export AND Train.
			// Desired: Only Export.
			expectedTitles: []string{
				"Export Dataset",
			},
		},
		{
			name: "Everything up to date",
			migrations: []tracking.Migration{
				{Command: "train-bowling", Status: tracking.StatusCompleted, StartedAt: now},
				{Command: "train-batting", Status: tracking.StatusCompleted, StartedAt: now},
				{Command: "export-dataset", Status: tracking.StatusCompleted, StartedAt: hourAgo},
				{Command: "precompute-features", Status: tracking.StatusCompleted, StartedAt: twoHoursAgo},
				{Command: "cricsheet-import", Status: tracking.StatusCompleted, StartedAt: threeHoursAgo},
			},
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
			// Behaves like no migrations
			expectedTitles: []string{
				"Initialize Data",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suggestions := GenerateSuggestions(tt.migrations)
			titles := make([]string, 0, len(suggestions))
			for _, s := range suggestions {
				titles = append(titles, s.Title)
			}
			assert.ElementsMatch(t, tt.expectedTitles, titles)
		})
	}
}

package server

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/umayangag/cric-flow/go-app/internal/tracking"
)

// TestGenerateSuggestions walks the three-step chain: import, retrain, reload. Each
// rule fires when a step has never completed or last ran before the step it depends
// on, and only the earliest unmet one is offered -- suggesting every stale step at
// once is how the console used to tell an operator to do three things in an order it
// did not name.
func TestGenerateSuggestions(t *testing.T) {
	t.Parallel()

	now := time.Now()
	hourAgo := now.Add(-1 * time.Hour)
	twoHoursAgo := now.Add(-2 * time.Hour)

	completed := func(command string, at time.Time) tracking.Migration {
		return tracking.Migration{Command: command, Status: tracking.StatusCompleted, StartedAt: at}
	}

	testCases := []struct {
		name           string
		migrations     []tracking.Migration
		expectedTitles []string
	}{
		{
			name:           "no migrations",
			migrations:     []tracking.Migration{},
			expectedTitles: []string{"Initialize Data"},
		},
		{
			name:           "imported but never retrained",
			migrations:     []tracking.Migration{completed("cricsheet-import", now)},
			expectedTitles: []string{"Retrain"},
		},
		{
			name: "retrain predates the import",
			migrations: []tracking.Migration{
				completed("cricsheet-import", now),
				completed("xi-retrain", hourAgo),
				completed("xi-reload", hourAgo),
			},
			expectedTitles: []string{"Retrain"},
		},
		{
			name: "a run trained but nothing is serving it",
			migrations: []tracking.Migration{
				completed("xi-retrain", now),
				completed("cricsheet-import", hourAgo),
			},
			expectedTitles: []string{"Reload"},
		},
		{
			name: "reload predates the retrain",
			migrations: []tracking.Migration{
				completed("xi-retrain", now),
				completed("xi-reload", hourAgo),
				completed("cricsheet-import", twoHoursAgo),
			},
			expectedTitles: []string{"Reload"},
		},
		{
			name: "everything up to date",
			migrations: []tracking.Migration{
				completed("xi-reload", now),
				completed("xi-retrain", hourAgo),
				completed("cricsheet-import", twoHoursAgo),
			},
			expectedTitles: []string{},
		},
		{
			name: "a failed import is not an import",
			migrations: []tracking.Migration{
				{Command: "cricsheet-import", Status: tracking.StatusFailed, StartedAt: hourAgo},
			},
			expectedTitles: []string{"Initialize Data"},
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			suggestions := GenerateSuggestions(tc.migrations)
			titles := make([]string, 0, len(suggestions))
			for _, s := range suggestions {
				titles = append(titles, s.Title)
			}
			assert.ElementsMatch(t, tc.expectedTitles, titles)
		})
	}
}

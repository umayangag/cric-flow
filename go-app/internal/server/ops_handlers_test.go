package server

import (
	"testing"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/tracking"
)

func TestGenerateSuggestions(t *testing.T) {
	now := time.Now()
	oneHourAgo := now.Add(-1 * time.Hour)
	twoHoursAgo := now.Add(-2 * time.Hour)
	threeHoursAgo := now.Add(-3 * time.Hour)

	tests := []struct {
		name       string
		migrations []tracking.Migration
		wantTitle  string
		wantCount  int
	}{
		{
			name:       "No migrations",
			migrations: []tracking.Migration{},
			wantTitle:  "Initialize Data",
			wantCount:  1,
		},
		{
			name: "Import only",
			migrations: []tracking.Migration{
				{
					Command:   "cricsheet-import",
					Status:    tracking.StatusCompleted,
					StartedAt: oneHourAgo,
				},
			},
			wantTitle: "Run Precompute",
			wantCount: 1,
		},
		{
			name: "Import and Precompute (Precompute older)",
			migrations: []tracking.Migration{
				{
					Command:   "cricsheet-import",
					Status:    tracking.StatusCompleted,
					StartedAt: oneHourAgo,
				},
				{
					Command:   "precompute-features",
					Status:    tracking.StatusCompleted,
					StartedAt: twoHoursAgo,
				},
			},
			wantTitle: "Run Precompute",
			wantCount: 2,
		},
		{
			name: "Import and Precompute (Precompute newer)",
			migrations: []tracking.Migration{
				{
					Command:   "precompute-features",
					Status:    tracking.StatusCompleted,
					StartedAt: oneHourAgo,
				},
				{
					Command:   "cricsheet-import",
					Status:    tracking.StatusCompleted,
					StartedAt: twoHoursAgo,
				},
			},
			wantTitle: "Export Dataset",
			wantCount: 1,
		},
		{
			name: "Full Chain",
			migrations: []tracking.Migration{
				{Command: "train-bowling", Status: tracking.StatusCompleted, StartedAt: now},
				{Command: "train-batting", Status: tracking.StatusCompleted, StartedAt: now},
				{Command: "export-dataset", Status: tracking.StatusCompleted, StartedAt: oneHourAgo},
				{Command: "precompute-features", Status: tracking.StatusCompleted, StartedAt: twoHoursAgo},
				{Command: "cricsheet-import", Status: tracking.StatusCompleted, StartedAt: threeHoursAgo},
			},
			wantTitle: "", // No suggestions expected
			wantCount: 0,
		},
		{
			name: "Failed Import (should be ignored)",
			migrations: []tracking.Migration{
				{
					Command:   "cricsheet-import",
					Status:    tracking.StatusFailed,
					StartedAt: oneHourAgo,
				},
			},
			wantTitle: "",
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suggestions := GenerateSuggestions(tt.migrations)
			if len(suggestions) != tt.wantCount {
				t.Errorf("GenerateSuggestions() count = %v, want %v", len(suggestions), tt.wantCount)
				return
			}
			if tt.wantCount > 0 {
				found := false
				for _, s := range suggestions {
					if s.Title == tt.wantTitle {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("GenerateSuggestions() expected title %v not found", tt.wantTitle)
				}
			}
		})
	}
}

package weather

import (
	"context"
	"testing"
)

func TestNormalizeVenue(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"trim spaces", "  Lord's  ", "lord's"},
		{"lowercase", "Melbourne Cricket Ground", "melbourne cricket ground"},
		{"collapse whitespace", "  Lords   Stadium  ", "lords stadium"},
		{"single word", "SCG", "scg"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeVenue(tt.in)
			if got != tt.want {
				t.Errorf("NormalizeVenue(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestEnqueueJob_EmptyVenueReturnsNil(t *testing.T) {
	t.Parallel()
	// When both venue and city are empty, EnqueueJob returns nil immediately
	// without touching the DB. This path is testable without a database.
	ctx := context.Background()
	err := EnqueueJob(ctx, 1, "", "", 2)
	if err != nil {
		t.Errorf("EnqueueJob with empty venue/city should return nil, got %v", err)
	}
}

func TestBuildSessions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		inningsCount int
		wantLabels   []string
	}{
		{"zero defaults to 2", 0, []string{"inning1", "inning2"}},
		{"negative defaults to 2", -1, []string{"inning1", "inning2"}},
		{"one inning", 1, []string{"inning1"}},
		{"two innings", 2, []string{"inning1", "inning2"}},
		{"four innings", 4, []string{"inning1", "inning2", "inning3", "inning4"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildSessions(tt.inningsCount)
			if len(got) != len(tt.wantLabels) {
				t.Fatalf("buildSessions(%d) len = %d, want %d", tt.inningsCount, len(got), len(tt.wantLabels))
			}
			for i, want := range tt.wantLabels {
				if got[i].Label != want {
					t.Errorf("buildSessions(%d)[%d].Label = %q, want %q", tt.inningsCount, i, got[i].Label, want)
				}
			}
		})
	}
}

func TestFirstNonEmpty(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   []string
		want string
	}{
		{"empty slice", []string{}, ""},
		{"single empty", []string{""}, ""},
		{"first non-empty", []string{"a", "b", "c"}, "a"},
		{"skip leading empty", []string{"", "", "c"}, "c"},
		{"all empty", []string{"", "  ", ""}, ""},
		{"whitespace-only ignored", []string{"  ", "\t", "x"}, "x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := firstNonEmpty(tt.in...)
			if got != tt.want {
				t.Errorf("firstNonEmpty(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

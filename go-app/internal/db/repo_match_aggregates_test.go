package db

import (
	"testing"
)

// TestBuildMatchAggregates_SimpleMapping ensures the helper constructs the
// struct with expected field assignments. This locks the mapping semantics
// used by GetMatchAggregates (score -> Runs, wickets -> Wickets, extras -> Extras).
func TestBuildMatchAggregates_SimpleMapping(t *testing.T) {
	got := buildMatchAggregates(150, 7, 10, "IND")
	if got.Runs != 150 {
		t.Fatalf("Runs = %v, want 150", got.Runs)
	}
	if got.Wickets != 7 {
		t.Fatalf("Wickets = %v, want 7", got.Wickets)
	}
	if got.Extras != 10 {
		t.Fatalf("Extras = %v, want 10", got.Extras)
	}
	if got.WinnerTeamCode != "IND" {
		t.Fatalf("WinnerTeamCode = %q, want IND", got.WinnerTeamCode)
	}
}

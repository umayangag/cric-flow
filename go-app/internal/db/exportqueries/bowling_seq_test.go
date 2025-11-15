package exportqueries

import (
	"context"
	"strings"
	"testing"
)

func TestBuildBowlingSeqFragments_Gating(t *testing.T) {
	// OFF
	fields, joins := BuildBowlingSeqFragments(context.Background())
	if len(fields) != 0 || joins != "" {
		t.Fatalf("OFF: expected empty fragments, got fields=%v joins=%q", fields, joins)
	}
	// ON
	ctx := WithSeqEnabled(context.Background(), true)
	fields, joins = BuildBowlingSeqFragments(ctx)
	if len(fields) == 0 {
		t.Fatalf("ON: expected non-empty fields")
	}
	// Check a few key aliases to ensure the right fragments are present
	wantAliases := []string{"pw_death", "ed_pp", "er", "bs", "obw1", "obw6"}
	for _, a := range wantAliases {
		if !strings.Contains(joins, a) {
			t.Fatalf("ON: expected joins to contain alias %q; joins=%q", a, joins)
		}
	}
	// Also ensure concrete table names appear in the ON fragments
	wantTables := []string{
		"player_window_features",
		"extras_discipline_features",
		"event_reaction_features",
		"bowling_spell_features",
		"over_boundary_wicket_features",
	}
	for _, tbl := range wantTables {
		if !strings.Contains(joins, tbl) {
			t.Fatalf("ON: expected joins to reference table %q; joins=%q", tbl, joins)
		}
	}
}

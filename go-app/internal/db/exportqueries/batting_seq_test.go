package exportqueries

import (
	"context"
	"strings"
	"testing"
)

func TestBuildBattingSeqFragments_Gating(t *testing.T) {
	// OFF
	fields, joins := BuildBattingSeqFragments(context.Background())
	if len(fields) != 0 || joins != "" {
		t.Fatalf("OFF: expected empty fragments, got fields=%v joins=%q", fields, joins)
	}
	// ON
	ctx := WithSeqEnabled(context.Background(), true)
	fields, joins = BuildBattingSeqFragments(ctx)
	if len(fields) == 0 {
		t.Fatalf("ON: expected non-empty fields")
	}
	// Check a few key aliases to ensure the right fragments are present
	wantAliases := []string{"pw_pp", "ent", "set", "er_bat"}
	for _, a := range wantAliases {
		if !strings.Contains(joins, a) {
			t.Fatalf("ON: expected joins to contain alias %q; joins=%q", a, joins)
		}
	}
	// Ensure concrete table names exist in ON fragments where applicable
	wantTables := []string{
		"player_window_features",
		"event_reaction_features",
	}
	for _, tbl := range wantTables {
		if !strings.Contains(joins, tbl) {
			t.Fatalf("ON: expected joins to reference table %q; joins=%q", tbl, joins)
		}
	}
	// ORDER BY latest-as-of marker should appear in at least one fragment when wired later.
	// For placeholders this may be absent; allow either presence or absence, but keep a marker check when present.
	if strings.Contains(joins, "ORDER BY") && !strings.Contains(joins, "ORDER BY as_of_date DESC LIMIT 1") {
		t.Fatalf("expected ORDER BY latest-as-of pattern when ORDER BY present; joins=%q", joins)
	}
}

package exportqueries_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-flow/go-app/internal/db/exportqueries"
)

func TestBuildBattingSeqFragments_Gating(t *testing.T) {
	// OFF
	fields, joins := exportqueries.BuildBattingSeqFragments(context.Background())
	require.Empty(t, fields)
	require.Empty(t, joins)

	// ON
	ctx := exportqueries.WithSeqEnabled(context.Background(), true)
	fields, joins = exportqueries.BuildBattingSeqFragments(ctx)
	require.NotEmpty(t, fields)

	// Check a few key aliases to ensure the right fragments are present
	for _, a := range []string{"pw_pp", "ent", "set", "er_bat"} {
		require.True(t, strings.Contains(joins, a), "expected joins to contain alias %q; joins=%q", a, joins)
	}

	// Ensure concrete table names exist in ON fragments where applicable
	for _, tbl := range []string{"player_window_features", "event_reaction_features"} {
		require.True(t, strings.Contains(joins, tbl), "expected joins to reference table %q; joins=%q", tbl, joins)
	}

	// ORDER BY latest-as-of marker should appear in at least one fragment when wired later.
	if strings.Contains(joins, "ORDER BY") {
		require.Contains(t, joins, "ORDER BY as_of_date DESC LIMIT 1")
	}
}

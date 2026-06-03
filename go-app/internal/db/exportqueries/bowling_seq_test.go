package exportqueries_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-flow/go-app/internal/db/exportqueries"
)

func TestBuildBowlingSeqFragments_Gating(t *testing.T) {
	// OFF
	fields, joins := exportqueries.BuildBowlingSeqFragments(context.Background())
	require.Empty(t, fields)
	require.Empty(t, joins)

	// ON
	ctx := exportqueries.WithSeqEnabled(context.Background(), true)
	fields, joins = exportqueries.BuildBowlingSeqFragments(ctx)
	require.NotEmpty(t, fields)

	// Check a few key aliases to ensure the right fragments are present
	for _, a := range []string{"pw_death", "ed_pp", "er", "bs", "obw1", "obw6"} {
		require.True(t, strings.Contains(joins, a), "expected joins to contain alias %q; joins=%q", a, joins)
	}

	// Also ensure concrete table names appear in the ON fragments
	for _, tbl := range []string{
		"player_window_features",
		"extras_discipline_features",
		"event_reaction_features",
		"bowling_spell_features",
		"over_boundary_wicket_features",
	} {
		require.True(t, strings.Contains(joins, tbl), "expected joins to reference table %q; joins=%q", tbl, joins)
	}
}

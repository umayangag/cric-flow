package features

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultContract_NonEmptyAndNoDuplicates(t *testing.T) {
	c := &defaultContract
	require.NotEmpty(t, c.Batting, "batting features required")
	require.NotEmpty(t, c.Bowling, "bowling features required")
	require.NotEmpty(t, c.Fielding, "fielding features required")
	require.Equal(t, "2", c.Version, "default contract version")

	for _, name := range []string{"Batting", "Bowling", "Fielding"} {
		var list []string
		switch name {
		case "Batting":
			list = c.Batting
		case "Bowling":
			list = c.Bowling
		case "Fielding":
			list = c.Fielding
		}
		seen := make(map[string]bool)
		for _, f := range list {
			require.False(t, seen[f], "duplicate feature %q in %s", f, name)
			seen[f] = true
		}
	}
}

func TestBattingFeatureNames_MatchesDefaultContract(t *testing.T) {
	got := BattingFeatureNames()
	require.Equal(t, defaultContract.Batting, got)
}

func TestBowlingFeatureNames_MatchesDefaultContract(t *testing.T) {
	got := BowlingFeatureNames()
	require.Equal(t, defaultContract.Bowling, got)
}

func TestFieldingFeatureNames_MatchesDefaultContract(t *testing.T) {
	got := FieldingFeatureNames()
	require.Equal(t, defaultContract.Fielding, got)
}

func TestLoadContract_WithVersionInJSON(t *testing.T) {
	// Use repo configs/feature_vectors.json (go-app is under repo root, so ../../../configs from internal/features)
	rel := filepath.Join("..", "..", "..", "configs", "feature_vectors.json")
	abs, err := filepath.Abs(rel)
	if err != nil {
		t.Skipf("resolve path: %v", err)
	}
	if _, err := os.Stat(abs); os.IsNotExist(err) {
		t.Skipf("config not found at %s", abs)
	}
	c, err := loadContract(abs)
	require.NoError(t, err)
	require.NotNil(t, c)
	require.Equal(t, "2", c.Version)
	require.Equal(t, len(defaultContract.Batting), len(c.Batting))
	require.Equal(t, len(defaultContract.Bowling), len(c.Bowling))
	require.Equal(t, len(defaultContract.Fielding), len(c.Fielding))
}

func TestContractVersion_ReturnsVersionFromContract(t *testing.T) {
	// Default or cached contract should have version "2" (v2 adds raw windowed stat features)
	v := ContractVersion()
	require.NotEmpty(t, v)
}

// TestRawStatsFeatureNames_MatchesContractSubset ensures RawStatsFeatureNames() stays in sync with
// the batting/bowling contract: the first 18 names must appear in BattingFeatureNames() in the same
// order, and the last 18 must appear in BowlingFeatureNames() in the same order.
func TestRawStatsFeatureNames_MatchesContractSubset(t *testing.T) {
	raw := RawStatsFeatureNames()
	require.Len(t, raw, 36, "RawStatsFeatureNames must have 18 batting + 18 bowling")
	batRaw := raw[:18]
	bowlRaw := raw[18:]

	batAll := BattingFeatureNames()
	bowlAll := BowlingFeatureNames()

	// Subsequence: batRaw must appear in batAll in order (filter batAll to batRaw set, must equal batRaw).
	batFiltered := filterToSubset(batAll, batRaw)
	require.Equal(t, batRaw, batFiltered, "BattingFeatureNames() must contain raw stat names in same order as RawStatsFeatureNames()")

	bowlFiltered := filterToSubset(bowlAll, bowlRaw)
	require.Equal(t, bowlRaw, bowlFiltered, "BowlingFeatureNames() must contain raw stat names in same order as RawStatsFeatureNames()")
}

func filterToSubset(all, subset []string) []string {
	set := make(map[string]struct{})
	for _, s := range subset {
		set[s] = struct{}{}
	}
	var out []string
	for _, a := range all {
		if _, ok := set[a]; ok {
			out = append(out, a)
		}
	}
	return out
}

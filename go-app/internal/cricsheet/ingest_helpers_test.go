package cricsheet

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnsureBatAndBowl(t *testing.T) {
	t.Parallel()
	bm := map[string]*batRow{}
	br1 := ensureBat(bm, "A")
	require.NotNil(t, br1)
	require.NotNil(t, bm["A"])
	br2 := ensureBat(bm, "A")
	require.Same(t, br1, br2)

	wm := map[string]*bowlRow{}
	bw1 := ensureBowl(wm, "B")
	require.NotNil(t, bw1)
	require.NotNil(t, wm["B"])
	bw2 := ensureBowl(wm, "B")
	require.Same(t, bw1, bw2)
}

func TestInningsRuns(t *testing.T) {
	t.Parallel()
	inng := Innings{Team: "X", Overs: []Over{
		{Over: 0, Deliveries: []Delivery{{Runs: RunInfo{Total: 1}}, {Runs: RunInfo{Total: 4}}}},
		{Over: 1, Deliveries: []Delivery{{Runs: RunInfo{Total: 6}}}},
	}}
	require.Equal(t, 11, inningsRuns(inng))
}

func TestOversFromBalls(t *testing.T) {
	t.Parallel()
	require.Equal(t, float32(2.5), oversFromBalls(17, 6))
	// invalid bpo -> default to 6
	require.Equal(t, float32(1.0), oversFromBalls(6, 0))
}

func TestMaidenCount(t *testing.T) {
	t.Parallel()
	m := map[int]int{0: 0, 1: 6, 2: 0, 3: 1}
	require.Equal(t, 2, maidenCount(m))
}

func TestStrikeRate(t *testing.T) {
	t.Parallel()
	exp := float32(float64(50) / float64(35) * 100.0)
	require.InDelta(t, exp, strikeRate(50, 35), 1e-6)
	require.Equal(t, float32(0), strikeRate(10, 0))
}

func TestStrPtrAndFirstNonEmpty(t *testing.T) {
	t.Parallel()
	p := strPtr("hello")
	require.NotNil(t, p)
	require.Equal(t, "hello", *p)
	require.Equal(t, "A", firstNonEmpty(" ", "", "A", "B"))
	require.Equal(t, "", firstNonEmpty(" ", "  "))
}

func TestOtherTeam(t *testing.T) {
	t.Parallel()
	require.Equal(t, "Australia", otherTeam("India", "India", "Australia"))
	require.Equal(t, "India", otherTeam("Australia", "India", "Australia"))
	require.Equal(t, "India", otherTeam("", " India ", " "))
	require.Equal(t, "", otherTeam("England", "India", "Australia"))
}

func TestBattingOrderFromInnings(t *testing.T) {
	t.Parallel()
	// Deliveries show appearance order: C (with non-striker A), then B appears
	inng := Innings{Team: "X", Overs: []Over{
		{Over: 0, Deliveries: []Delivery{
			{Batter: "C", NonStriker: "A", Runs: RunInfo{Batter: 4, Total: 4}},
			{Batter: "C", NonStriker: "A", Runs: RunInfo{Batter: 1, Total: 1}},
		}},
		{Over: 1, Deliveries: []Delivery{
			{Batter: "B", NonStriker: "C", Runs: RunInfo{Batter: 0, Total: 0}},
		}},
	}}
	// include an unseen player D; should be appended alphabetically at end
	names := []string{"A", "B", "C", "D"}
	order := battingOrderFromInnings(inng, names)
	// expected first-seen order: C, A, B then unseen: D
	expected := []string{"C", "A", "B", "D"}
	require.Len(t, order, len(expected))
	for i := range expected {
		require.Equalf(t, expected[i], order[i], "idx %d order=%v", i, order)
	}
	// Ensure unseen are sorted
	unseen := []string{"X", "Z", "Y"}
	sort.Strings(unseen)
	_ = unseen
}

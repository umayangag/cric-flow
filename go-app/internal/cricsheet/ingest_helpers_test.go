package cricsheet

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestEnsureBatAndBowl uses table-driven subtests to validate ensureBat/ensureBowl behavior.
func TestEnsureBatAndBowl(t *testing.T) {
	t.Parallel()

	t.Run("ensureBat returns same pointer on repeated calls", func(t *testing.T) {
		// Arrange
		bm := map[string]*batRow{}
		// Act
		br1 := ensureBat(bm, "A")
		br2 := ensureBat(bm, "A")
		// Assert
		require.NotNil(t, br1)
		require.NotNil(t, bm["A"]) // inserted
		require.Same(t, br1, br2)
	})

	t.Run("ensureBowl returns same pointer on repeated calls", func(t *testing.T) {
		// Arrange
		wm := map[string]*bowlRow{}
		// Act
		bw1 := ensureBowl(wm, "B")
		bw2 := ensureBowl(wm, "B")
		// Assert
		require.NotNil(t, bw1)
		require.NotNil(t, wm["B"]) // inserted
		require.Same(t, bw1, bw2)
	})
}

func TestInningsRuns(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		inng Innings
		want int
	}{
		{
			name: "simple two overs",
			inng: Innings{Team: "X", Overs: []Over{
				{Over: 0, Deliveries: []Delivery{{Runs: RunInfo{Total: 1}}, {Runs: RunInfo{Total: 4}}}},
				{Over: 1, Deliveries: []Delivery{{Runs: RunInfo{Total: 6}}}},
			}},
			want: 11,
		},
		{
			name: "no overs",
			inng: Innings{Team: "X", Overs: nil},
			want: 0,
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			got := inningsRuns(tc.inng)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestOversFromBalls(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		balls int
		bpo   int
		want  float32
	}{
		{name: "17 balls @6 bpo -> 2.5", balls: 17, bpo: 6, want: 2.5},
		{name: "exact over boundary", balls: 12, bpo: 6, want: 2.0},
		{name: "invalid bpo -> default to 6", balls: 6, bpo: 0, want: 1.0},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			got := oversFromBalls(tc.balls, tc.bpo)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestMaidenCount(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		overs        map[int]int
		legalBalls   map[int]int
		ballsPerOver int
		want         int
	}{
		{
			name:         "two complete scoreless overs are maidens",
			overs:        map[int]int{0: 0, 1: 6, 2: 0, 3: 1},
			legalBalls:   map[int]int{0: 6, 1: 6, 2: 6, 3: 6},
			ballsPerOver: 6,
			want:         2,
		},
		{
			name:         "none scoreless",
			overs:        map[int]int{0: 1, 1: 2},
			legalBalls:   map[int]int{0: 6, 1: 6},
			ballsPerOver: 6,
			want:         0,
		},
		{
			name: "a wide-conceded run breaks the maiden even though it is not the bowler's" +
				" own figures (IMPORT-14)",
			overs:        map[int]int{0: 1},
			legalBalls:   map[int]int{0: 6},
			ballsPerOver: 6,
			want:         0,
		},
		{
			name: "a scoreless over cut short by the innings ending is not a maiden" +
				" (IMPORT-14)",
			overs:        map[int]int{0: 0},
			legalBalls:   map[int]int{0: 3},
			ballsPerOver: 6,
			want:         0,
		},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, maidenCount(tc.overs, tc.legalBalls, tc.ballsPerOver))
		})
	}
}

func TestStrikeRate(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		runs  int
		balls int
		want  float32
	}{
		{name: "50 off 35", runs: 50, balls: 35, want: float32(float64(50) / float64(35) * 100.0)},
		{name: "zero balls -> zero sr", runs: 10, balls: 0, want: 0},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			got := strikeRate(tc.runs, tc.balls)
			if tc.balls == 0 {
				require.Equal(t, tc.want, got)
				return
			}
			require.InDelta(t, tc.want, got, 1e-6)
		})
	}
}

func TestStrPtrAndFirstNonEmpty(t *testing.T) {
	t.Parallel()

	t.Run("strPtr returns pointer with value", func(t *testing.T) {
		p := strPtr("hello")
		require.NotNil(t, p)
		require.Equal(t, "hello", *p)
	})

	testCases := []struct {
		name string
		in   []string
		want string
	}{
		{name: "skips blanks returns A", in: []string{" ", "", "A", "B"}, want: "A"},
		{name: "all blank -> empty", in: []string{" ", "  "}, want: ""},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			got := firstNonEmpty(tc.in...)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestOtherTeam(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		team string
		a    string
		b    string
		want string
	}{
		{name: "matches first -> return second", team: "India", a: "India", b: "Australia", want: "Australia"},
		{name: "matches second -> return first", team: "Australia", a: "India", b: "Australia", want: "India"},
		{name: "trimmed spaces", team: "", a: " India ", b: " ", want: "India"},
		{name: "no match -> empty", team: "England", a: "India", b: "Australia", want: ""},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, otherTeam(tc.team, tc.a, tc.b))
		})
	}
}

func TestBattingOrderFromInnings(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		inng     Innings
		names    []string
		expected []string
	}{
		{
			name: "appearance order respected; unseen appended sorted",
			inng: Innings{Team: "X", Overs: []Over{
				{Over: 0, Deliveries: []Delivery{
					{Batter: "C", NonStriker: "A", Runs: RunInfo{Batter: 4, Total: 4}},
					{Batter: "C", NonStriker: "A", Runs: RunInfo{Batter: 1, Total: 1}},
				}},
				{Over: 1, Deliveries: []Delivery{
					{Batter: "B", NonStriker: "C", Runs: RunInfo{Batter: 0, Total: 0}},
				}},
			}},
			names:    []string{"A", "B", "C", "D"},
			expected: []string{"C", "A", "B", "D"},
		},
		{
			name: "multiple unseen appended alphabetically",
			inng: Innings{Team: "X", Overs: []Over{
				{Over: 0, Deliveries: []Delivery{
					{Batter: "C", NonStriker: "A", Runs: RunInfo{Batter: 4, Total: 4}},
					{Batter: "C", NonStriker: "A", Runs: RunInfo{Batter: 1, Total: 1}},
				}},
				{Over: 1, Deliveries: []Delivery{
					{Batter: "B", NonStriker: "C", Runs: RunInfo{Batter: 0, Total: 0}},
				}},
			}},
			names:    []string{"A", "B", "C", "E", "D"},
			expected: []string{"C", "A", "B", "D", "E"},
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			order := battingOrderFromInnings(tc.inng, tc.names)
			require.Len(t, order, len(tc.expected))
			for i := range tc.expected {
				require.Equalf(t, tc.expected[i], order[i], "idx %d order=%v", i, order)
			}
		})
	}
}

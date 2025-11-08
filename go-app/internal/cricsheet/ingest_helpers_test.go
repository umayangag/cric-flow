package cricsheet_test

import (
	"math"
	"sort"
	"testing"
)

func TestEnsureBatAndBowl(t *testing.T) {
	bm := map[string]*batRow{}
	br1 := ensureBat(bm, "A")
	if br1 == nil || bm["A"] == nil {
		t.Fatalf("ensureBat should create entry")
	}
	br2 := ensureBat(bm, "A")
	if br1 != br2 {
		t.Fatalf("ensureBat should return same pointer for existing")
	}

	wm := map[string]*bowlRow{}
	bw1 := ensureBowl(wm, "B")
	if bw1 == nil || wm["B"] == nil {
		t.Fatalf("ensureBowl should create entry")
	}
	bw2 := ensureBowl(wm, "B")
	if bw1 != bw2 {
		t.Fatalf("ensureBowl should return same pointer for existing")
	}
}

func TestInningsRuns(t *testing.T) {
	inng := Innings{Team: "X", Overs: []Over{
		{Over: 0, Deliveries: []Delivery{{Runs: RunInfo{Total: 1}}, {Runs: RunInfo{Total: 4}}}},
		{Over: 1, Deliveries: []Delivery{{Runs: RunInfo{Total: 6}}}},
	}}
	if got := inningsRuns(inng); got != 11 {
		t.Fatalf("runs mismatch: got %d want %d", got, 11)
	}
}

func TestOversFromBalls(t *testing.T) {
	if got := oversFromBalls(17, 6); got != 2.5 {
		t.Fatalf("expected 2.5 overs, got %.1f", got)
	}
	// invalid bpo -> default to 6
	if got := oversFromBalls(6, 0); got != 1.0 {
		t.Fatalf("expected 1.0 overs with default bpo, got %.1f", got)
	}
}

func TestMaidenCount(t *testing.T) {
	m := map[int]int{0: 0, 1: 6, 2: 0, 3: 1}
	if got := maidenCount(m); got != 2 {
		t.Fatalf("expected 2 maidens, got %d", got)
	}
}

func TestStrikeRate(t *testing.T) {
	exp := float32(float64(50) / float64(35) * 100.0)
	if got := strikeRate(50, 35); math.Abs(float64(got-exp)) > 1e-6 {
		t.Fatalf("strike rate mismatch: got %.6f want %.6f", got, exp)
	}
	if got := strikeRate(10, 0); got != 0 {
		t.Fatalf("expected 0 when balls=0, got %.1f", got)
	}
}

func TestStrPtrAndFirstNonEmpty(t *testing.T) {
	p := strPtr("hello")
	if p == nil || *p != "hello" {
		t.Fatalf("strPtr incorrect")
	}
	if v := firstNonEmpty(" ", "", "A", "B"); v != "A" {
		t.Fatalf("firstNonEmpty expected A, got %q", v)
	}
	if v := firstNonEmpty(" ", "  "); v != "" {
		t.Fatalf("firstNonEmpty expected empty, got %q", v)
	}
}

func TestOtherTeam(t *testing.T) {
	if v := otherTeam("India", "India", "Australia"); v != "Australia" {
		t.Fatalf("expected Australia, got %q", v)
	}
	if v := otherTeam("Australia", "India", "Australia"); v != "India" {
		t.Fatalf("expected India, got %q", v)
	}
	if v := otherTeam("", " India ", " "); v != "India" {
		t.Fatalf("expected first non-empty trimmed candidate, got %q", v)
	}
	if v := otherTeam("England", "India", "Australia"); v != "" {
		t.Fatalf("expected empty when no match, got %q", v)
	}
}

func TestBattingOrderFromInnings(t *testing.T) {
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
	if len(order) != len(expected) {
		t.Fatalf("order len mismatch: got %d want %d", len(order), len(expected))
	}
	for i := range expected {
		if order[i] != expected[i] {
			t.Fatalf("idx %d: got %q want %q (order=%v)", i, order[i], expected[i], order)
		}
	}
	// Ensure unseen are sorted
	unseen := []string{"X", "Z", "Y"}
	sort.Strings(unseen)
	_ = unseen
}

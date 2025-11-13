package mathrand

import (
	mrand "math/rand"
	"testing"
)

func TestRNG_DeterministicWithSeed(t *testing.T) {
	//nolint:gosec // deterministic math/rand is intentional for testing repeatability
	r1 := New(mrand.New(mrand.NewSource(1)))
	//nolint:gosec // deterministic math/rand is intentional for testing repeatability
	r2 := New(mrand.New(mrand.NewSource(1)))

	vals1 := []int{r1.Intn(100), r1.Intn(100), r1.Intn(100)}
	vals2 := []int{r2.Intn(100), r2.Intn(100), r2.Intn(100)}

	for i := range vals1 {
		if vals1[i] != vals2[i] {
			t.Fatalf("determinism failed at %d: %v vs %v", i, vals1, vals2)
		}
	}
}

func TestRNG_SeedAffectsSequence(t *testing.T) {
	//nolint:gosec // deterministic math/rand is intentional for testing repeatability
	r := New(mrand.New(mrand.NewSource(0)))
	_ = []int{r.Intn(100), r.Intn(100)} // burn a couple
	r.Seed(42)
	a := []int{r.Intn(100), r.Intn(100), r.Intn(100)}
	r.Seed(42)
	b := []int{r.Intn(100), r.Intn(100), r.Intn(100)}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("seed should reset sequence: %v vs %v", a, b)
		}
	}
}

package precomputefeatures

import (
	"testing"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/db"
)

func TestToFeatureInnings(t *testing.T) {
	t.Parallel()
	d1 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	d2 := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
	in := []db.InnVal{
		{MatchDate: d1, Value: 25.5},
		{MatchDate: d2, Value: 30.0},
	}
	out := toFeatureInnings(in)
	if len(out) != 2 {
		t.Fatalf("expected 2 innings, got %d", len(out))
	}
	if out[0].Date != d1 || out[0].Value != 25.5 {
		t.Errorf("out[0] = %+v, want Date=%v Value=25.5", out[0], d1)
	}
	if out[1].Date != d2 || out[1].Value != 30.0 {
		t.Errorf("out[1] = %+v, want Date=%v Value=30.0", out[1], d2)
	}
}

func TestToFeatureInnings_Empty(t *testing.T) {
	t.Parallel()
	out := toFeatureInnings(nil)
	if len(out) != 0 {
		t.Errorf("toFeatureInnings(nil) len = %d, want 0", len(out))
	}
}

func TestResolveConcurrencyLimit_PositiveValue(t *testing.T) {
	t.Parallel()
	// When requestedLimit > 0, it should be returned as-is (capped by resources ceiling elsewhere).
	got := resolveConcurrencyLimit(5)
	if got != 5 {
		t.Errorf("resolveConcurrencyLimit(5) = %d, want 5", got)
	}
}

func TestResolveConcurrencyLimit_UsesResourcesWhenZero(t *testing.T) {
	t.Parallel()
	// When requestedLimit is 0, resolveConcurrencyLimit delegates to resources.GetLimit.
	// GetLimit returns at least 1, so we verify we get a positive number.
	got := resolveConcurrencyLimit(0)
	if got < 1 {
		t.Errorf("resolveConcurrencyLimit(0) = %d, want >= 1", got)
	}
}

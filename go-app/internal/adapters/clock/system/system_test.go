package system

import (
	"testing"
	"time"
)

func TestClock_Now(t *testing.T) {
	c := Clock{}
	t0 := time.Now()
	t1 := c.Now()
	// Allow a small window; simply ensure t1 is not zero and is after t0 - 1s
	if t1.IsZero() {
		t.Fatalf("Now() returned zero time")
	}
	if t1.Before(t0.Add(-1 * time.Second)) {
		t.Fatalf("Now() returned unexpected time: %v vs %v", t1, t0)
	}
}

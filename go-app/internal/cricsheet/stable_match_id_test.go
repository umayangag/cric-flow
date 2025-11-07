package cricsheet

import "testing"

func TestStableMatchID_DeterministicAndSensitive(t *testing.T) {
	id1 := StableMatchID("2025-11-07", "India", "Australia")
	id2 := StableMatchID("2025-11-07", "India", "Australia")
	if id1 != id2 {
		t.Fatalf("expected deterministic id, got %d and %d", id1, id2)
	}
	// Changing input should change id
	id3 := StableMatchID("2025-11-07", "Australia", "India") // swapped order
	if id3 == id1 {
		t.Fatalf("expected different id when team order changes: got %d == %d", id3, id1)
	}
	id4 := StableMatchID("2025-11-08", "India", "Australia") // changed date
	if id4 == id1 {
		t.Fatalf("expected different id when date changes: got %d == %d", id4, id1)
	}
}

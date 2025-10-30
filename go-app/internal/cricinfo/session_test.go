package cricinfo

import "testing"

func TestDeriveSessions_MissingKey(t *testing.T) {
	b, bw := DeriveSessions(1, map[string]string{"Other": "value"})
	if b != "" || bw != "" {
		t.Fatalf("expected empty sessions, got batting=%q bowling=%q", b, bw)
	}
}

func TestDeriveSessions_NormalString_Innings1(t *testing.T) {
	text := "10:00 start, Session 1 : Morning, Lunch Interval, Session 2 : Afternoon, Tea Interval, Session 3 : Evening"
	hours := map[string]string{"Hours of play (local time)": text}
	b, bw := DeriveSessions(1, hours)
	if b != "Morning" || bw != "Afternoon" {
		t.Fatalf("innings=1 unexpected sessions: batting=%q bowling=%q", b, bw)
	}
}

func TestDeriveSessions_NormalString_Innings2Plus(t *testing.T) {
	text := "10:00 start, Session 1 : Morning, Lunch Interval, Session 2 : Afternoon, Tea Interval, Session 3 : Evening"
	hours := map[string]string{"Hours of play (local time)": text}
	b, bw := DeriveSessions(2, hours)
	if b != "Afternoon" || bw != "Morning" {
		t.Fatalf("innings=2 unexpected sessions: batting=%q bowling=%q", b, bw)
	}
}

func TestDeriveSessions_FixMissingCommaBeforeInterval(t *testing.T) {
	// Only three segments -> we inject a comma before first " Interval"
	text := "10:00 start, Session 1 : Morning Interval, Session 2 : Afternoon"
	hours := map[string]string{"Hours of play (local time)": text}
	b, bw := DeriveSessions(1, hours)
	// After fix, positions 1 and 3 should map to Morning and Afternoon respectively
	if b != "Morning" || bw != "Afternoon" {
		t.Fatalf("missing-comma fix failed: batting=%q bowling=%q", b, bw)
	}
}

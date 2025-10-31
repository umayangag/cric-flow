package cricinfo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseMatchPage_MockFixture(t *testing.T) {
	f := filepath.Join("testdata", "scorecard_sample.html")
	fh, err := os.Open(f)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer fh.Close()

	mi, batting, bowling, fielding, weather, err := ParseMatchPage(fh, 123456)
	if err != nil {
		t.Fatalf("ParseMatchPage error: %v", err)
	}

	if mi.MatchID != 123456 {
		t.Errorf("MatchID = %d, want 123456", mi.MatchID)
	}
	if mi.Venue == "" || mi.Toss == "" {
		t.Errorf("expected venue and toss populated, got venue=%q toss=%q", mi.Venue, mi.Toss)
	}
	if mi.BattingSession != "Morning" || mi.BowlingSession != "Afternoon" {
		t.Errorf("unexpected sessions: batting=%q bowling=%q", mi.BattingSession, mi.BowlingSession)
	}

	if len(weather) != 2 {
		t.Fatalf("weather len=%d, want 2", len(weather))
	}
	if weather[0].Session != "Morning" || weather[1].Session != "Afternoon" {
		t.Errorf("unexpected weather labels: %+v", weather)
	}

	if len(batting) != 2 {
		t.Fatalf("batting len=%d, want 2", len(batting))
	}
	if batting[0].PlayerName == "" || batting[0].BattingPosition != 1 {
		t.Errorf("unexpected batting[0]: %+v", batting[0])
	}

	if len(bowling) != 1 || bowling[0].PlayerName == "" {
		t.Fatalf("unexpected bowling: %+v", bowling)
	}

	if len(fielding) != 1 || fielding[0].PlayerName == "" {
		t.Fatalf("unexpected fielding: %+v", fielding)
	}
}

func TestParseMatchPage_RealishFixture(t *testing.T) {
	f := filepath.Join("testdata", "scorecard_realish.html")
	fh, err := os.Open(f)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer fh.Close()

	mi, batting, bowling, fielding, weather, err := ParseMatchPage(fh, 654321)
	if err != nil {
		t.Fatalf("ParseMatchPage error: %v", err)
	}
	_ = fielding // not present in realish fixture
	_ = weather  // not present in realish fixture

	if mi.MatchID != 654321 {
		t.Errorf("MatchID = %d, want 654321", mi.MatchID)
	}
	if mi.Venue == "" || mi.Toss == "" {
		t.Errorf("expected venue and toss populated, got venue=%q toss=%q", mi.Venue, mi.Toss)
	}
	if mi.BattingSession == "" || mi.BowlingSession == "" {
		t.Errorf(
			"expected derived sessions from hours-of-play, got batting=%q bowling=%q",
			mi.BattingSession,
			mi.BowlingSession,
		)
	}

	if len(batting) < 2 {
		t.Fatalf("batting len=%d, want >=2", len(batting))
	}
	if batting[0].PlayerName == "" || batting[0].BattingPosition != 1 {
		t.Errorf("unexpected batting[0]: %+v", batting[0])
	}

	if len(bowling) < 1 || bowling[0].PlayerName == "" {
		t.Fatalf("unexpected bowling: %+v", bowling)
	}
}

func TestParseMatchPage_DNBFixture(t *testing.T) {
	f := filepath.Join("testdata", "scorecard_dnb.html")
	fh, err := os.Open(f)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer fh.Close()

	mi, batting, bowling, fielding, weather, err := ParseMatchPage(fh, 777777)
	if err != nil {
		t.Fatalf("ParseMatchPage error: %v", err)
	}
	_ = fielding
	_ = weather

	if mi.Venue == "" || mi.Toss == "" {
		t.Errorf("expected venue and toss populated, got venue=%q toss=%q", mi.Venue, mi.Toss)
	}
	if len(batting) < 1 {
		t.Fatalf("expected at least 1 batting row (DNB), got %d", len(batting))
	}
	if batting[0].PlayerName == "" {
		t.Errorf("expected player name in DNB row, got empty: %+v", batting[0])
	}
	if len(bowling) < 1 {
		t.Fatalf("expected at least 1 bowling row, got %d", len(bowling))
	}
}

func TestParseMatchPage_RainFixture(t *testing.T) {
	f := filepath.Join("testdata", "scorecard_rain.html")
	fh, err := os.Open(f)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer fh.Close()

	mi, batting, bowling, _, _, err := ParseMatchPage(fh, 888888)
	if err != nil {
		t.Fatalf("ParseMatchPage error: %v", err)
	}
	if mi.BattingSession == "" || mi.BowlingSession == "" {
		t.Errorf(
			"expected sessions derived for rain fixture, got batting=%q bowling=%q",
			mi.BattingSession,
			mi.BowlingSession,
		)
	}
	if len(batting) < 1 || len(bowling) < 1 {
		t.Fatalf("expected some batting and bowling rows, got batting=%d bowling=%d", len(batting), len(bowling))
	}
}

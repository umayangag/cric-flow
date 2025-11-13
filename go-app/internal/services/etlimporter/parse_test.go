package etlimporter_test

import (
	"strings"
	"testing"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/services/etlimporter"
)

type assertParseBatFn func(t *testing.T, rowsLen int, err error)

type assertParseBowlFn func(t *testing.T, rowsLen int, err error)

func assertNoErrorRowsBat(want int) assertParseBatFn {
	return func(t *testing.T, rowsLen int, err error) {
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if rowsLen != want {
			t.Fatalf("want rows=%d got %d", want, rowsLen)
		}
	}
}

func assertErrContainsBat(sub string) assertParseBatFn {
	return func(t *testing.T, _ int, err error) {
		s := ""
		if err != nil {
			s = err.Error()
		}
		if err == nil || indexOf(s, sub) < 0 {
			t.Fatalf("want err containing %q, got %v", sub, err)
		}
	}
}

func assertNoErrorRowsBowl(want int) assertParseBowlFn {
	return func(t *testing.T, rowsLen int, err error) {
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if rowsLen != want {
			t.Fatalf("want rows=%d got %d", want, rowsLen)
		}
	}
}

func assertErrContainsBowl(sub string) assertParseBowlFn {
	return func(t *testing.T, _ int, err error) {
		s := ""
		if err != nil {
			s = err.Error()
		}
		if err == nil || indexOf(s, sub) < 0 {
			t.Fatalf("want err containing %q, got %v", sub, err)
		}
	}
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		ok := true
		for j := 0; j < len(sub); j++ {
			if s[i+j] != sub[j] {
				ok = false
				break
			}
		}
		if ok {
			return i
		}
	}
	return -1
}

func TestParseBattingCSV(t *testing.T) {
	t.Parallel()
	good := "player_name,season,format,runs,balls,fours,sixes,position\nA,2019,T20,10,8,1,0,3\nB,2019,odi,20,15,2,1,4\n"
	badHeader := "foo,bar\n1,2\n"
	badRow := "player_name,season,format,runs,balls,fours,sixes,position\nA,2019,T20,abc,8,1,0,3\n"
	cases := []struct {
		name   string
		csv    string
		assert assertParseBatFn
	}{
		{"ok two rows", good, assertNoErrorRowsBat(2)},
		{"bad header", badHeader, assertErrContainsBat("unexpected batting header")},
		{"bad value", badRow, assertErrContainsBat("runs")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rows, err := etlimporter.ParseBattingCSV(strings.NewReader(tc.csv))
			l := len(rows)
			_ = rows
			// use len only to avoid unused var; detailed field checks are unnecessary here
			if l < 0 {
				// never executed
				t.Fatalf("unreachable")
			}
			tc.assert(t, l, err)
		})
	}
}

func TestParseBowlingCSV(t *testing.T) {
	t.Parallel()
	good := "player_name,season,format,overs,balls,maidens,runs,wickets,economy\nA,2019,T20,4,24,0,20,2,5.0\n"
	badHeader := "x,y\n1,2\n"
	badRow := "player_name,season,format,overs,balls,maidens,runs,wickets,economy\nA,2019,T20,xx,24,0,20,2,5.0\n"
	cases := []struct {
		name   string
		csv    string
		assert assertParseBowlFn
	}{
		{"ok one row", good, assertNoErrorRowsBowl(1)},
		{"bad header", badHeader, assertErrContainsBowl("unexpected bowling header")},
		{"bad value", badRow, assertErrContainsBowl("overs")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rows, err := etlimporter.ParseBowlingCSV(strings.NewReader(tc.csv))
			l := len(rows)
			_ = rows
			if l < 0 {
				t.Fatalf("unreachable")
			}
			tc.assert(t, l, err)
		})
	}
}

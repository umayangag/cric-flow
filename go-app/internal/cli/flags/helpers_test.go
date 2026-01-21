package flags

import (
	"reflect"
	"testing"
	"time"
)

func TestParseDateISO(t *testing.T) {
	cases := []struct {
		in string
		ok bool
		y  int
		m  time.Month
		d  int
	}{
		{"2024-01-02", true, 2024, time.January, 2},
		{" 2024-12-31 ", true, 2024, time.December, 31},
		{"2024/01/02", false, 0, 0, 0},
		{"", false, 0, 0, 0},
	}
	for _, c := range cases {
		got, err := ParseDateISO(c.in)
		if c.ok && err != nil {
			t.Fatalf("expected ok for %q, got error: %v", c.in, err)
		}
		if !c.ok && err == nil {
			t.Fatalf("expected error for %q, got none (time=%v)", c.in, got)
		}
		if c.ok {
			if got.Year() != c.y || got.Month() != c.m || got.Day() != c.d {
				t.Fatalf("unexpected date for %q: %v", c.in, got)
			}
		}
	}
}

func TestParseRFC3339(t *testing.T) {
	ts := "2024-01-02T15:04:05Z"
	got, err := ParseRFC3339(ts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Format(time.RFC3339) != ts {
		t.Fatalf("round trip mismatch: %v", got)
	}
	if _, err := ParseRFC3339("bad"); err == nil {
		t.Fatal("expected error for invalid RFC3339")
	}
}

func TestParseCSVList(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"A,B,C", []string{"A", "B", "C"}},
		{" A , B ,, C ", []string{"A", "B", "C"}},
		{"", []string{}},
	}
	for _, c := range cases {
		got := ParseCSVList(c.in)
		if !reflect.DeepEqual(got, c.want) {
			t.Fatalf("ParseCSVList(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestRequireNonEmpty(t *testing.T) {
	if err := RequireNonEmpty("format", "T20"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := RequireNonEmpty("format", "  "); err == nil {
		t.Fatal("expected error for empty value")
	}
}

func TestParseDurationFlag(t *testing.T) {
	d, err := ParseDurationFlag(" 5s ")
	if err != nil || d != 5*time.Second {
		t.Fatalf("unexpected: d=%v err=%v", d, err)
	}
	if _, err := ParseDurationFlag("nonsense"); err == nil {
		t.Fatal("expected error for invalid duration")
	}
}

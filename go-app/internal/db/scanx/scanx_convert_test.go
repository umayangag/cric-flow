package scanx

import (
	"errors"
	"testing"
)

type fakeScanner struct {
	vals []any
	err  error
}

func (f *fakeScanner) Scan(dest ...any) error {
	if f.err != nil {
		return f.err
	}
	if len(dest) != len(f.vals) {
		return errors.New("dest len mismatch")
	}
	for i := range dest {
		p, ok := dest[i].(*any)
		if !ok {
			return errors.New("dest must be *any")
		}
		*p = f.vals[i]
	}
	return nil
}

func TestAnyToString_Primitives(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{nil, ""},
		{"abc", "abc"},
		{[]byte("xyz"), "xyz"},
		{int64(42), "42"},
		{int32(7), "7"},
		{3, "3"},
		{float32(1.5), "1.5"},
		{float64(2.0), "2"},
		{true, "1"},
		{false, "0"},
	}
	for i, c := range cases {
		got := AnyToString(c.in)
		if got != c.want {
			t.Fatalf("case %d: want %q, got %q", i, c.want, got)
		}
	}
}

func TestTrimFloat(t *testing.T) {
	cases := map[string]string{
		"1.000000": "1",
		"3.140000": "3.14",
		"0.500000": "0.5",
		"10":       "10",
	}
	for in, want := range cases {
		if got := TrimFloat(in); got != want {
			t.Fatalf("TrimFloat(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestScanToStrings(t *testing.T) {
	fs := &fakeScanner{vals: []any{int64(1), float64(2.0), []byte("ok"), true, nil}}
	got, err := ScanToStrings(fs, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"1", "2", "ok", "1", ""}
	if len(got) != len(want) {
		t.Fatalf("len mismatch: got %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("idx %d: want %q, got %q", i, want[i], got[i])
		}
	}
}

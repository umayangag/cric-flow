package cricsheet

import (
	"bytes"
	"testing"
)

func TestSeason_UnmarshalJSON_VariousTypes(t *testing.T) {
	cases := []struct {
		name string
		in   string
		out  string
	}{
		{"string year", `"2019"`, "2019"},
		{"string range", `"2007/08"`, "2007/08"},
		{"int year", `2012`, "2012"},
		{"float intish", `2012.0`, "2012"},
		{"float non-int", `2012.5`, "2012.5"},
		{"null", `null`, ""},
	}
	for _, tc := range cases {
		var s Season
		if err := s.UnmarshalJSON([]byte(tc.in)); err != nil {
			t.Fatalf("%s: unexpected error: %v", tc.name, err)
		}
		if string(s) != tc.out {
			t.Fatalf("%s: got %q want %q", tc.name, string(s), tc.out)
		}
	}
}

func TestCollection_UnmarshalJSON_Forms(t *testing.T) {
	cases := []struct {
		name string
		in   string
		out  []string
	}{
		{"array strings", `["A","B"]`, []string{"A", "B"}},
		{"array objects", `[{"name":"A"},{"name":"B"}]`, []string{"A", "B"}},
		{"single object", `{"name":"A"}`, []string{"A"}},
		{"single string", `"A"`, []string{"A"}},
		{"mixed array", `[{"name":"A"},"B",{"name":""},123]`, []string{"A", "B"}},
		{"null", `null`, nil},
	}
	for _, tc := range cases {
		var c Collection
		if err := c.UnmarshalJSON([]byte(tc.in)); err != nil {
			t.Fatalf("%s: unexpected error: %v", tc.name, err)
		}
		if len(c) != len(tc.out) {
			t.Fatalf("%s: len=%d want=%d", tc.name, len(c), len(tc.out))
		}
		for i := range tc.out {
			if c[i] != tc.out[i] {
				t.Fatalf("%s: idx %d got %q want %q", tc.name, i, c[i], tc.out[i])
			}
		}
	}
}

func TestCollection_UnmarshalJSON_GarbageFallback(t *testing.T) {
	var c Collection
	// An object without a name should fallback to empty slice (not error)
	if err := c.UnmarshalJSON([]byte(`{"foo":"bar"}`)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c == nil || len(c) != 0 {
		t.Fatalf("expected empty collection, got %#v", []string(c))
	}
}

func TestParse_DoesNotPanicOnMinimalJSON(t *testing.T) {
	// Minimal valid shape
	data := []byte(`{"info":{"teams":["A","B"],"match_type":"T20","season":"2019"},"innings":[]}`)
	m, err := Parse(bytes.NewReader(data))
	if err != nil || m == nil {
		t.Fatalf("parse failed: %v", err)
	}
}

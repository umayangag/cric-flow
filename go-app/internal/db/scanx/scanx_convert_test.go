package scanx_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-flow/go-app/internal/db/scanx"
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
	t.Parallel()

	testCases := []struct {
		name     string
		input    any
		expected string
	}{
		{name: "nil returns empty", input: nil, expected: ""},
		{name: "string passthrough", input: "abc", expected: "abc"},
		{name: "byte slice", input: []byte("xyz"), expected: "xyz"},
		{name: "int64", input: int64(42), expected: "42"},
		{name: "int32", input: int32(7), expected: "7"},
		{name: "int", input: 3, expected: "3"},
		{name: "float32", input: float32(1.5), expected: "1.5"},
		{name: "float64", input: float64(2.0), expected: "2"},
		{name: "bool true", input: true, expected: "1"},
		{name: "bool false", input: false, expected: "0"},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := scanx.AnyToString(tc.input)
			assert.Equal(t, tc.expected, got)
		})
	}
}

func TestTrimFloat(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "trailing zeros removed", input: "1.000000", expected: "1"},
		{name: "partial trailing zeros", input: "3.140000", expected: "3.14"},
		{name: "single trailing zero", input: "0.500000", expected: "0.5"},
		{name: "no decimal unchanged", input: "10", expected: "10"},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := scanx.TrimFloat(tc.input)
			assert.Equal(t, tc.expected, got)
		})
	}
}

func TestScanToStrings(t *testing.T) {
	t.Parallel()

	fs := &fakeScanner{vals: []any{int64(1), float64(2.0), []byte("ok"), true, nil}}
	got, err := scanx.ScanToStrings(fs, 5)
	require.NoError(t, err)

	expected := []string{"1", "2", "ok", "1", ""}
	assert.Equal(t, expected, got)
}

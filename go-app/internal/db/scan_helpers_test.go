package db

import (
	"database/sql"
	"testing"
)

func TestToNullString(t *testing.T) {
	testCases := []struct {
		name string
		in   string
		want sql.NullString
	}{
		{name: "empty => invalid", in: "", want: sql.NullString{Valid: false}},
		{name: "non-empty => valid", in: "IND", want: sql.NullString{String: "IND", Valid: true}},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			got := toNullString(tc.in)
			if got.Valid != tc.want.Valid || got.String != tc.want.String {
				t.Fatalf("toNullString(%q)=%+v want %+v", tc.in, got, tc.want)
			}
		})
	}
}

func TestToNullFloat64(t *testing.T) {
	testCases := []struct {
		name string
		in   float64
	}{
		{name: "zero => valid zero", in: 0},
		{name: "positive", in: 123.45},
		{name: "negative", in: -7.2},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			got := toNullFloat64(tc.in)
			if !got.Valid || got.Float64 != tc.in {
				t.Fatalf("toNullFloat64(%v)=%+v want valid with same value", tc.in, got)
			}
		})
	}
}

func TestFromNullString(t *testing.T) {
	testCases := []struct {
		name string
		in   sql.NullString
		want string
	}{
		{name: "invalid => empty", in: sql.NullString{Valid: false}, want: ""},
		{name: "valid returns value", in: sql.NullString{String: "AUS", Valid: true}, want: "AUS"},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			got := fromNullString(tc.in)
			if got != tc.want {
				t.Fatalf("fromNullString(%+v)=%q want %q", tc.in, got, tc.want)
			}
		})
	}
}

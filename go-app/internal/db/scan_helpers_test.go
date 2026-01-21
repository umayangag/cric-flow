package db

import (
	"database/sql"
	"testing"
)

func TestToNullString(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want sql.NullString
	}{
		{name: "empty => invalid", in: "", want: sql.NullString{Valid: false}},
		{name: "non-empty => valid", in: "IND", want: sql.NullString{String: "IND", Valid: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toNullString(tt.in)
			if got.Valid != tt.want.Valid || got.String != tt.want.String {
				t.Fatalf("toNullString(%q)=%+v want %+v", tt.in, got, tt.want)
			}
		})
	}
}

func TestToNullFloat64(t *testing.T) {
	tests := []struct {
		name string
		in   float64
	}{
		{name: "zero => valid zero", in: 0},
		{name: "positive", in: 123.45},
		{name: "negative", in: -7.2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toNullFloat64(tt.in)
			if !got.Valid || got.Float64 != tt.in {
				t.Fatalf("toNullFloat64(%v)=%+v want valid with same value", tt.in, got)
			}
		})
	}
}

func TestFromNullString(t *testing.T) {
	tests := []struct {
		name string
		in   sql.NullString
		want string
	}{
		{name: "invalid => empty", in: sql.NullString{Valid: false}, want: ""},
		{name: "valid returns value", in: sql.NullString{String: "AUS", Valid: true}, want: "AUS"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fromNullString(tt.in)
			if got != tt.want {
				t.Fatalf("fromNullString(%+v)=%q want %q", tt.in, got, tt.want)
			}
		})
	}
}

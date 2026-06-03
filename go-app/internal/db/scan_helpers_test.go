package db

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"
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
			require.Equal(t, tc.want.Valid, got.Valid)
			require.Equal(t, tc.want.String, got.String)
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
			require.True(t, got.Valid)
			require.Equal(t, tc.in, got.Float64)
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
			require.Equal(t, tc.want, got)
		})
	}
}

package server

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSqlNullString(t *testing.T) {
	testCases := []struct {
		name string
		in   string
		want sql.NullString
	}{
		{"empty returns invalid", "", sql.NullString{Valid: false}},
		{"non_empty returns valid", "abc", sql.NullString{String: "abc", Valid: true}},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			got := sqlNullString(tc.in)
			require.Equal(t, tc.want.Valid, got.Valid)
			require.Equal(t, tc.want.String, got.String)
		})
	}
}

func TestSqlNullFloat64(t *testing.T) {
	got := sqlNullFloat64(3.14)
	require.True(t, got.Valid)
	require.Equal(t, 3.14, got.Float64)
}

func TestNullString(t *testing.T) {
	testCases := []struct {
		name string
		in   sql.NullString
		want string
	}{
		{"invalid returns empty", sql.NullString{Valid: false}, ""},
		{"valid returns string", sql.NullString{String: "xyz", Valid: true}, "xyz"},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			got := nullString(tc.in)
			require.Equal(t, tc.want, got)
		})
	}
}

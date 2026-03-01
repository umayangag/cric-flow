package server

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseMatchDate(t *testing.T) {
	tests := []struct {
		name    string
		s       string
		wantErr bool
	}{
		{"empty", "", true},
		{"invalid", "not-a-date", true},
		{"RFC3339", "2024-03-15T12:00:00Z", false},
		{"RFC3339 with offset", "2024-03-15T12:00:00+05:30", false},
		{"date only", "2024-03-15", false},
		{"whitespace trimmed", "  2024-03-15  ", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseMatchDate(tt.s)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.False(t, got.IsZero())
		})
	}
}

func TestParseMatchDate_ValidFormats(t *testing.T) {
	// RFC3339
	t1, err := parseMatchDate("2024-03-15T12:00:00Z")
	require.NoError(t, err)
	require.Equal(t, 2024, t1.Year())
	require.Equal(t, time.March, t1.Month())
	require.Equal(t, 15, t1.Day())

	// Date only
	t2, err := parseMatchDate("2024-03-15")
	require.NoError(t, err)
	require.Equal(t, 2024, t2.Year())
	require.Equal(t, time.March, t2.Month())
	require.Equal(t, 15, t2.Day())
}

package server

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseMatchDate(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		input   string
		want    time.Time
		wantErr bool
	}{
		{
			name:  "rfc3339_full",
			input: "2025-06-15T10:30:00Z",
			want:  time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC),
		},
		{
			name:  "rfc3339_with_offset",
			input: "2025-06-15T10:30:00+05:30",
			want:  time.Date(2025, 6, 15, 10, 30, 0, 0, time.FixedZone("", 5*3600+30*60)),
		},
		{
			name:  "date_only_yyyy_mm_dd",
			input: "2025-06-15",
			want:  time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			name:  "whitespace_trimmed",
			input: "  2025-06-15  ",
			want:  time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			name:    "empty_string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "invalid_format",
			input:   "15/06/2025",
			wantErr: true,
		},
		{
			name:    "partial_date",
			input:   "2025-06",
			wantErr: true,
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseMatchDate(tc.input)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.True(t, tc.want.Equal(got), "expected %v, got %v", tc.want, got)
		})
	}
}

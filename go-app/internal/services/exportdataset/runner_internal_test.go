package exportdataset

import (
	"errors"
	"io/fs"
	"testing"
)

func TestIsPermissionDenied(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"fs.ErrPermission", fs.ErrPermission, true},
		{"wrapped permission", errors.New("permission denied"), true},
		{"uppercase Permission Denied", errors.New("Permission Denied"), true},
		{"other error", errors.New("something else"), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isPermissionDenied(tc.err)
			if got != tc.want {
				t.Errorf("isPermissionDenied(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestSafeFormatForFilename(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		s    string
		want bool
	}{
		{"empty", "", false},
		{"valid T20", "T20", true},
		{"valid with underscore", "T20I", true},
		{"valid lowercase", "t20", true},
		{"valid mixed", "T20_ODI", true},
		{"contains slash", "T20/ODI", false},
		{"contains dot", "T20.ODI", false},
		{"contains hyphen", "T20-ODI", false},
		{"space", "T20 ODI", false},
		{"too long", "A23456789012345678901234567890123", false},
		{"length 32 ok", "A2345678901234567890123456789012", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := safeFormatForFilename(tt.s)
			if got != tt.want {
				t.Errorf("safeFormatForFilename(%q) = %v, want %v", tt.s, got, tt.want)
			}
		})
	}
}

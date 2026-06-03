package exportdataset

import (
	"errors"
	"io/fs"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsPermissionDenied(t *testing.T) {
	t.Parallel()

	testCases := []struct {
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

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			got := isPermissionDenied(tc.err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestSafeFormatForFilename(t *testing.T) {
	t.Parallel()
	testCases := []struct {
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
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			got := safeFormatForFilename(tc.s)
			require.Equal(t, tc.want, got)
		})
	}
}

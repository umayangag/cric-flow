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

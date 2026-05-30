package selection

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEncodeSession(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		in   int
		want int
	}{
		{0, 2},
		{-1, 2},
		{1, 1},
		{2, 2},
		{3, 3},
		{4, 3},
		{10, 3},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run("", func(t *testing.T) {
			got := encodeSession(tc.in)
			require.Equal(t, tc.want, got, "encodeSession(%d)", tc.in)
		})
	}
}

func TestEncodeViscosity(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		in   int
		want int
	}{
		{0, 0},
		{-1, 0},
		{-100, 0},
		{1, 1},
		{2, 1},
		{5, 1},
		{100, 1},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run("", func(t *testing.T) {
			got := encodeViscosity(tc.in)
			require.Equal(t, tc.want, got, "encodeViscosity(%d)", tc.in)
		})
	}
}

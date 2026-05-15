package selection

import "testing"

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
		got := encodeSession(tc.in)
		if got != tc.want {
			t.Errorf("encodeSession(%d) = %d, want %d", tc.in, got, tc.want)
		}
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
		got := encodeViscosity(tc.in)
		if got != tc.want {
			t.Errorf("encodeViscosity(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

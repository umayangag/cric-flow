package selection

import "testing"

func TestEncodeSession(t *testing.T) {
	t.Parallel()
	tests := []struct {
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
	for _, tt := range tests {
		got := encodeSession(tt.in)
		if got != tt.want {
			t.Errorf("encodeSession(%d) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestEncodeViscosity(t *testing.T) {
	t.Parallel()
	tests := []struct {
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
	for _, tt := range tests {
		got := encodeViscosity(tt.in)
		if got != tt.want {
			t.Errorf("encodeViscosity(%d) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

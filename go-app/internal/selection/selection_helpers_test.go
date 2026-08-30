package selection

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPrevSeasonName(t *testing.T) {
	testCases := []struct {
		in   string
		want string
	}{
		{"2024", "2023"},
		{"2019", "2018"},
		{"invalid", "invalid"},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.in, func(t *testing.T) {
			require.Equal(t, tc.want, prevSeasonName(tc.in))
		})
	}
}

func TestParseSeasonInt(t *testing.T) {
	testCases := []struct {
		in   string
		want int
	}{
		{"2024", 2024},
		{" 2019 ", 2019},
		{"x", 0},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.in, func(t *testing.T) {
			require.Equal(t, tc.want, parseSeasonInt(tc.in))
		})
	}
}

func TestNz64(t *testing.T) {
	require.Equal(t, int64(10), nz64(struct {
		Int64 int64
		Valid bool
	}{10, true}))
	require.Equal(t, int64(0), nz64(struct {
		Int64 int64
		Valid bool
	}{10, false}))
}

func TestF32(t *testing.T) {
	require.Equal(t, float32(3.14), f32(3.14))
}

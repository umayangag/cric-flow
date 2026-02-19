package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStepToCommand(t *testing.T) {
	t.Parallel()

	cases := []struct {
		step string
		want string
	}{
		{"train_batting", "make train-batting"},
		{"train_bowling", "make train-bowling"},
		{"train_fielding", "make train-fielding CUTOFF=2025-01-01T00:00:00Z"},
		{"train_extras", "make train-extras CUTOFF=2025-01-01T00:00:00Z"},
		{"train_win", "make train-win CUTOFF=2025-01-01T00:00:00Z"},
		{"auto_tune", "make ml-auto-tune MODEL=all ALL_FORMATS=1"},
		{"unknown_step", ""},
		{"", ""},
		{"export", ""},
		{"precompute", ""},
	}

	for _, tc := range cases {
		t.Run(tc.step, func(t *testing.T) {
			got := stepToCommand(tc.step)
			assert.Equal(t, tc.want, got)
		})
	}
}

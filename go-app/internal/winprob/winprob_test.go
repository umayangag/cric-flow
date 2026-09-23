package winprob_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/umayangag/cric-flow/go-app/internal/winprob"
)

func TestTeam1Wins_BreaksTheTieAtOneHalfTowardTeam1(t *testing.T) {
	testCases := []struct {
		name        string
		probability float64
		want        bool
	}{
		{name: "just below one half favours team2", probability: 0.4999, want: false},
		{name: "exactly one half favours team1", probability: 0.5, want: true},
		{name: "just above one half favours team1", probability: 0.5001, want: true},
		{name: "a decisive team2 probability favours team2", probability: 0.1, want: false},
		{name: "a decisive team1 probability favours team1", probability: 0.9, want: true},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, winprob.Team1Wins(tc.probability))
		})
	}
}

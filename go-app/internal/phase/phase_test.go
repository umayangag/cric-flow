package phase_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/umayangag/cric-flow/go-app/internal/phase"
)

func TestPhaseForCode(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		formatCode    string
		ballSeq       int
		inningsLength int
		expected      string
	}{
		{
			name:          "T20 powerplay boundary",
			formatCode:    "T20",
			ballSeq:       36,
			inningsLength: 120,
			expected:      phase.PhasePowerplay,
		},
		{
			name:          "T20 middle start",
			formatCode:    "T20",
			ballSeq:       37,
			inningsLength: 120,
			expected:      phase.PhaseMiddle,
		},
		{
			name:          "T20 death start",
			formatCode:    "T20",
			ballSeq:       91,
			inningsLength: 120,
			expected:      phase.PhaseDeath,
		},
		{
			name:          "T20 short innings no death",
			formatCode:    "T20",
			ballSeq:       80,
			inningsLength: 80,
			expected:      phase.PhaseMiddle,
		},
		{
			name:          "T20I equivalent",
			formatCode:    "T20I",
			ballSeq:       20,
			inningsLength: 120,
			expected:      phase.PhasePowerplay,
		},
		{
			name:          "ODI powerplay boundary",
			formatCode:    "ODI",
			ballSeq:       60,
			inningsLength: 300,
			expected:      phase.PhasePowerplay,
		},
		{
			name:          "ODI middle start",
			formatCode:    "ODI",
			ballSeq:       61,
			inningsLength: 300,
			expected:      phase.PhaseMiddle,
		},
		{
			name:          "ODI death start",
			formatCode:    "ODI",
			ballSeq:       241,
			inningsLength: 300,
			expected:      phase.PhaseDeath,
		},
		{
			name:          "ODI short innings no death",
			formatCode:    "ODI",
			ballSeq:       200,
			inningsLength: 200,
			expected:      phase.PhaseMiddle,
		},
		{
			name:          "Test any ball all",
			formatCode:    "TEST",
			ballSeq:       1,
			inningsLength: 0,
			expected:      phase.PhaseAll,
		},
		{
			name:          "Test any ball known length",
			formatCode:    "TEST",
			ballSeq:       250,
			inningsLength: 540,
			expected:      phase.PhaseAll,
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Act
			got := phase.PhaseForCode(tc.formatCode, tc.ballSeq, tc.inningsLength)

			// Assert
			assert.Equal(t, tc.expected, got)
		})
	}
}

func TestPhaseFor(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		formatID      int
		ballSeq       int
		inningsLength int
		expected      string
	}{
		{
			name:          "ID T20",
			formatID:      3,
			ballSeq:       10,
			inningsLength: 120,
			expected:      phase.PhasePowerplay,
		},
		{
			name:          "ID T20I",
			formatID:      4,
			ballSeq:       95,
			inningsLength: 120,
			expected:      phase.PhaseDeath,
		},
		{
			name:          "ID ODI",
			formatID:      2,
			ballSeq:       70,
			inningsLength: 300,
			expected:      phase.PhaseMiddle,
		},
		{
			name:          "ID Test",
			formatID:      1,
			ballSeq:       300,
			inningsLength: 0,
			expected:      phase.PhaseAll,
		},
		{
			name:          "ID unknown",
			formatID:      99,
			ballSeq:       10,
			inningsLength: 60,
			expected:      phase.PhaseAll,
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Act
			got := phase.PhaseFor(tc.formatID, tc.ballSeq, tc.inningsLength)

			// Assert
			assert.Equal(t, tc.expected, got)
		})
	}
}

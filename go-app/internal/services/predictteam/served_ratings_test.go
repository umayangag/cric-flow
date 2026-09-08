package predictteam

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServedRatings_Adopt(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		already   ServedRatings
		answer    ServedRatings
		want      ServedRatings
		wantError string
	}{
		{
			name:    "the first answer stamps the prediction",
			already: ServedRatings{},
			answer:  servedFromRunA,
			want:    servedFromRunA,
		},
		{
			name:    "a later answer from the same state agrees",
			already: servedFromRunA,
			answer:  servedFromRunA,
			want:    servedFromRunA,
		},
		{
			name:      "a later answer from another run is refused and the stamp kept",
			already:   servedFromRunA,
			answer:    servedFromRunB,
			want:      servedFromRunA,
			wantError: "the served run changed",
		},
		{
			name:      "the same run with a different date is still a change",
			already:   servedFromRunA,
			answer:    ServedRatings{RunID: servedFromRunA.RunID, RatingsThrough: "2026-09-03"},
			want:      servedFromRunA,
			wantError: "ratings through 2026-09-03",
		},
		{
			name:      "an answer with no run is refused rather than read as unknown",
			already:   ServedRatings{},
			answer:    ServedRatings{RatingsThrough: "2026-09-02"},
			want:      ServedRatings{},
			wantError: "without naming the run and the date",
		},
		{
			name:      "an answer with no date is refused rather than read as unknown",
			already:   ServedRatings{},
			answer:    ServedRatings{RunID: servedFromRunA.RunID},
			want:      ServedRatings{},
			wantError: "without naming the run and the date",
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			stamp := tc.already

			err := stamp.Adopt(tc.answer)

			assert.Equal(t, tc.want, stamp)
			if tc.wantError == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantError)
		})
	}
}

func TestServedRunChangedError_NamesBothStates(t *testing.T) {
	t.Parallel()

	err := &ServedRunChangedError{Was: servedFromRunA, Now: servedFromRunB}

	assert.Contains(t, err.Error(), servedFromRunA.RunID)
	assert.Contains(t, err.Error(), servedFromRunA.RatingsThrough)
	assert.Contains(t, err.Error(), servedFromRunB.RunID)
	assert.Contains(t, err.Error(), servedFromRunB.RatingsThrough)
}

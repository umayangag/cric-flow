package predictions_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/predictions"
)

// Two answers are two rows. The id is minted before the insert — it is inside the payload
// the row holds — so nothing downstream can catch a collision by handing back a sequence.
func TestNewID_MintsADistinctIdPerAnswer(t *testing.T) {
	t.Parallel()

	const answers = 1000
	seen := make(map[string]struct{}, answers)
	for i := 0; i < answers; i++ {
		id := predictions.NewID()
		require.Len(t, id, 36, "a UUID as the store's id column expects it")
		_, repeated := seen[id]
		assert.False(t, repeated, "two predictions must never be filed under one id")
		seen[id] = struct{}{}
	}
}

package cricsheet_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/cricsheet"
)

func TestStableMatchID_DeterministicAndSensitive(t *testing.T) {
	id1 := cricsheet.StableMatchID("2025-11-07", "India", "Australia")
	id2 := cricsheet.StableMatchID("2025-11-07", "India", "Australia")
	assert.Equal(t, id1, id2, "deterministic")

	id3 := cricsheet.StableMatchID("2025-11-07", "Australia", "India")
	assert.NotEqual(t, id1, id3, "order matters")

	id4 := cricsheet.StableMatchID("2025-11-08", "India", "Australia")
	assert.NotEqual(t, id1, id4, "date matters")
}

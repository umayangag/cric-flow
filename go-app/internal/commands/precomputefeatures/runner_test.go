package precomputefeatures

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewRunner(t *testing.T) {
	r := NewRunner()
	require.NotNil(t, r)
}

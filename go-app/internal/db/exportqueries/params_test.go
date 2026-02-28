package exportqueries

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetFeatureExtractionParams(t *testing.T) {
	t.Parallel()
	alpha, lastN, windowN, alphaShort, alphaLong, momentumLastN := GetFeatureExtractionParams()
	require.Greater(t, alpha, 0.0, "alpha should be positive")
	require.LessOrEqual(t, alpha, 1.0, "alpha should be <= 1")
	require.Greater(t, lastN, 0, "lastN should be positive")
	require.GreaterOrEqual(t, windowN, 0, "windowN should be non-negative")
	require.Greater(t, alphaShort, 0.0, "alphaShort should be positive")
	require.LessOrEqual(t, alphaShort, 1.0, "alphaShort should be <= 1")
	require.Greater(t, alphaLong, 0.0, "alphaLong should be positive")
	require.LessOrEqual(t, alphaLong, 1.0, "alphaLong should be <= 1")
	require.Greater(t, momentumLastN, 0, "momentumLastN should be positive")
}

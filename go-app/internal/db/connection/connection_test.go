package connection

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildDSN_ComposesExpectedURL(t *testing.T) {
	t.Parallel()

	// Act
	got := BuildDSN("user", "x", "host", "5432", "dbname", "disable")

	// Assert
	wantSuffix := "://user:x@host:5432/dbname?sslmode=disable"
	assert.True(t, strings.HasSuffix(got, wantSuffix), "BuildDSN() = %q, want suffix %q", got, wantSuffix)
}

func TestGetenv_ReturnsEnvOrDefault(t *testing.T) {
	t.Setenv("SOME_KEY", "value")

	assert.Equal(t, "value", getenv("SOME_KEY", "default"))
	assert.Equal(t, "fallback", getenv("MISSING_KEY", "fallback"))
}

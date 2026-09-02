package pipeline

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMLServiceBaseURL(t *testing.T) {
	testCases := []struct {
		name    string
		envVal  string
		wantHas string // substring the result must contain
	}{
		{
			name:    "env_override",
			envVal:  "http://ml:9000/",
			wantHas: "http://ml:9000",
		},
		{
			name:    "env_override_no_trailing_slash",
			envVal:  "http://ml:9000",
			wantHas: "http://ml:9000",
		},
		{
			name:    "empty_env_uses_fallback",
			envVal:  "",
			wantHas: "http", // fallback always contains http
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ML_SERVICE_URL", tc.envVal)
			got := MLServiceBaseURL()
			assert.Contains(t, got, tc.wantHas)
			// Must never end with trailing slash.
			assert.NotRegexp(t, `/$`, got)
		})
	}
}

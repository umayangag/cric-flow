package evaluate

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateNonEmpty(t *testing.T) {
	tests := []struct {
		name  string
		value string
		err   bool
	}{
		{"empty fails", "", true},
		{"non-empty ok", "abc", false},
		{"single char ok", "x", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateNonEmpty(tt.value, "flag")
			if tt.err {
				require.Error(t, err)
				require.Contains(t, err.Error(), "must not be empty")
				return
			}
			require.NoError(t, err)
		})
	}
}
